// Raw wire protocol client (startup handshake, SCRAM auth, simple queries).
package xproto

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

type RawMessage struct {
	Type byte   // message type byte, e.g. 'S', 'C', 'Z', 'E'
	Body []byte // message body (without the type byte and length)
}

// CString reads a NUL-terminated string.
func CString(buf []byte) (string, []byte) {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i]), buf[i+1:]
		}
	}
	panic("unterminated string in backend message")
}

func (m RawMessage) ParameterStatus() (string, string) {
	name, rest := CString(m.Body)
	value, _ := CString(rest)
	return name, value
}

func (m RawMessage) ErrorFields() map[byte]string {
	fields := map[byte]string{}
	for len(m.Body) > 0 {
		code := m.Body[0]
		if code == 0 {
			break
		}
		var s string
		s, m.Body = CString(m.Body[1:])
		fields[code] = s
	}
	return fields
}

type Conn struct {
	conn  net.Conn
	trace bool
}

func traceEnabled() bool {
	return os.Getenv("TTT_WIREREPORT_DEBUG") != ""
}

// the server) authentication, then drains messages until ReadyForQuery.
func Connect(host string, port int, user, password, database string) (*Conn, []RawMessage, error) {
	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, nil, err
	}
	c := &Conn{conn: conn, trace: traceEnabled()}

	var params []struct{ k, v string }
	params = append(params,
		struct{ k, v string }{"user", user},
		struct{ k, v string }{"database", database},
	)
	var startup []byte
	for _, p := range params {
		startup = append(startup, []byte(p.k)...)
		startup = append(startup, 0)
		startup = append(startup, []byte(p.v)...)
		startup = append(startup, 0)
	}
	startup = append(startup, 0)
	msg := make([]byte, 8+len(startup))
	binary.BigEndian.PutUint32(msg[0:4], uint32(8+len(startup)))
	binary.BigEndian.PutUint32(msg[4:8], 196608) // protocol 3.0
	copy(msg[8:], startup)
	if _, err := conn.Write(msg); err != nil {
		conn.Close()
		return nil, nil, err
	}
	c.logf("-> StartupMessage (%d bytes)", len(msg))

	handshake := []RawMessage{}
	for {
		m, err := c.ReadMessage()
		if err != nil {
			conn.Close()
			return nil, nil, err
		}
		switch m.Type {
		case 'R': // AuthenticationRequest
			if err := c.handleAuth(m, user, password); err != nil {
				conn.Close()
				return nil, handshake, err
			}
		case 'E': // ErrorResponse during handshake
			conn.Close()
			return nil, handshake, fmt.Errorf("startup failed: %s", m.ErrorFields()['M'])
		case 'Z': // ReadyForQuery: connection established and idle
			return c, handshake, nil
		default:
			// 'S', 'K', 'N', ... just record
			handshake = append(handshake, m)
		}
	}
}

func (c *Conn) Close() error { return c.conn.Close() }

func (c *Conn) logf(format string, args ...any) {
	if c.trace {
		fmt.Fprintf(os.Stderr, "[xproto] "+format+"\n", args...)
	}
}

func (c *Conn) ReadMessage() (RawMessage, error) {
	var header [5]byte
	if _, err := io.ReadFull(c.conn, header[:]); err != nil {
		return RawMessage{}, err
	}
	length := binary.BigEndian.Uint32(header[1:]) - 4
	if length > 1<<24 {
		return RawMessage{}, errors.New("backend message too large")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.conn, body); err != nil {
		return RawMessage{}, err
	}
	c.logf("<- %s (%d bytes)", string(header[0]), length)
	return RawMessage{Type: header[0], Body: body}, nil
}

// backend messages until (and including) the final ReadyForQuery.
func (c *Conn) SimpleQuery(query string) ([]RawMessage, error) {
	payload := append([]byte(query), 0)
	msg := make([]byte, 5+len(payload))
	msg[0] = 'Q'
	binary.BigEndian.PutUint32(msg[1:5], uint32(4+len(payload)))
	copy(msg[5:], payload)
	if _, err := c.conn.Write(msg); err != nil {
		return nil, err
	}

	var out []RawMessage
	for {
		m, err := c.ReadMessage()
		if err != nil {
			return out, err
		}
		out = append(out, m)
		switch m.Type {
		case 'Z': // ReadyForQuery
			return out, nil
		case 'E': // ErrorResponse: keep reading until ReadyForQuery
		}
	}
}

func (c *Conn) sendPassword(body []byte) error {
	msg := make([]byte, 5+len(body))
	msg[0] = 'p'
	binary.BigEndian.PutUint32(msg[1:5], uint32(4+len(body)))
	copy(msg[5:], body)
	c.logf("-> Password 'p' (%d bytes body: %q)", len(body), body)
	_, err := c.conn.Write(msg)
	return err
}

func (c *Conn) handleAuth(m RawMessage, user, password string) error {
	if len(m.Body) < 4 {
		return errors.New("malformed authentication message")
	}
	code := binary.BigEndian.Uint32(m.Body[:4])
	data := m.Body[4:]
	switch code {
	case 0: // AuthenticationOk
		return nil
	case 3: // cleartext password
		return c.sendPassword(append([]byte(password), 0))
	case 5: // md5
		if len(data) < 4 {
			return errors.New("malformed md5 auth message")
		}
		inner := md5.Sum([]byte(password + user))
		outer := md5.Sum(append([]byte(fmt.Sprintf("%x", inner)), data[:4]...))
		return c.sendPassword(append([]byte("md5"+fmt.Sprintf("%x", outer)), 0))
	case 10: // SASL: pick SCRAM-SHA-256
		mechs := map[string]bool{}
		for len(data) > 0 {
			var name string
			name, data = CString(data)
			if name == "" {
				break
			}
			mechs[name] = true
		}
		if !mechs["SCRAM-SHA-256"] {
			return fmt.Errorf("no supported SASL mechanism among %v", mechs)
		}
		return c.scramAuth(password)
	case 11, 12:
		return errors.New("unexpected SASL message outside of a SCRAM exchange")
	default:
		return fmt.Errorf("unsupported authentication request code %d", code)
	}
}

// 'p' password messages. It is called after receiving the SASL request.
func (c *Conn) scramAuth(password string) error {
	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	clientNonce := base64.StdEncoding.EncodeToString(nonceBytes)
	clientFirstBare := "n=,r=" + clientNonce
	clientFirst := "n,," + clientFirstBare

	initial := append([]byte("SCRAM-SHA-256"), 0)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(clientFirst)))
	initial = append(initial, lenBuf[:]...)
	initial = append(initial, []byte(clientFirst)...)
	if err := c.sendPassword(initial); err != nil {
		return err
	}

	m, err := c.ReadMessage()
	if err != nil {
		return err
	}
	if m.Type != 'R' || binary.BigEndian.Uint32(m.Body[:4]) != 11 {
		return fmt.Errorf("expected SASLContinue, got type %q", string(m.Type))
	}
	serverFirstStr := strings.TrimRight(string(m.Body[4:]), "\x00")
	var serverNonceB64 string
	var salt []byte
	iters := 0
	for _, kv := range strings.Split(serverFirstStr, ",") {
		switch {
		case strings.HasPrefix(kv, "r="):
			serverNonceB64 = kv[2:]
		case strings.HasPrefix(kv, "s="):
			salt, err = base64.StdEncoding.DecodeString(kv[2:])
			if err != nil {
				return err
			}
		case strings.HasPrefix(kv, "i="):
			iters, err = strconv.Atoi(kv[2:])
			if err != nil {
				return err
			}
		}
	}
	if !strings.HasPrefix(serverNonceB64, clientNonce) {
		return errors.New("SCRAM server nonce does not extend client nonce")
	}
	clientFinalNoProof := "c=biws,r=" + serverNonceB64
	authMessage := clientFirstBare + "," + serverFirstStr + "," + clientFinalNoProof

	salted := pbkdf2SHA256([]byte(password), salt, iters, sha256.Size)
	clientKey := hmacSHA256(salted, []byte("Client Key"))
	storedKey := sha256.Sum256(clientKey)
	clientSignature := hmacSHA256(storedKey[:], []byte(authMessage))
	proof := make([]byte, len(clientKey))
	for i := range clientKey {
		proof[i] = clientKey[i] ^ clientSignature[i]
	}
	clientFinal := clientFinalNoProof + ",p=" + base64.StdEncoding.EncodeToString(proof)
	if err := c.sendPassword([]byte(clientFinal)); err != nil {
		return err
	}

	m, err = c.ReadMessage()
	if err != nil {
		return err
	}
	if m.Type != 'R' || binary.BigEndian.Uint32(m.Body[:4]) != 12 {
		return fmt.Errorf("expected SASLFinal, got type %q", string(m.Type))
	}
	serverKey := hmacSHA256(salted, []byte("Server Key"))
	serverSignature := hmacSHA256(serverKey, []byte(authMessage))
	want := "v=" + base64.StdEncoding.EncodeToString(serverSignature)
	if strings.TrimRight(string(m.Body[4:]), "\x00") != want {
		return errors.New("SCRAM server signature mismatch")
	}
	return nil
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	var out []byte
	blocks := (keyLen + sha256.Size - 1) / sha256.Size
	for block := 1; block <= blocks; block++ {
		saltBlock := append(append([]byte{}, salt...), byte(block>>24), byte(block>>16), byte(block>>8), byte(block))
		u := hmacSHA256(password, saltBlock)
		t := append([]byte{}, u...)
		for i := 1; i < iter; i++ {
			u = hmacSHA256(password, u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
