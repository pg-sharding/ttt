// Raw wire protocol client (startup handshake, simple queries).
// The server must accept trust authentication (see pg_hba.conf).
package xproto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
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

func Connect(host string, port int, user, database string) (*Conn, []RawMessage, error) {
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
			if err := handleAuth(m); err != nil {
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

func handleAuth(m RawMessage) error {
	if len(m.Body) < 4 {
		return errors.New("malformed authentication message")
	}
	code := binary.BigEndian.Uint32(m.Body[:4])
	if code != 0 {
		// trust auth: anything but AuthenticationOk won't do
		return fmt.Errorf("server requires authentication (code %d); tests need trust auth in pg_hba.conf", code)
	}
	return nil
}
