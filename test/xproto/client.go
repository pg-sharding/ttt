// Wire protocol client over pgproto3.Frontend.
// The server must accept trust authentication (see pg_hba.conf).
package xproto

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgproto3"
)

type Conn struct {
	conn     net.Conn
	frontend *pgproto3.Frontend
}

func Connect(host string, port int, user, database string) (*Conn, []pgproto3.BackendMessage, error) {
	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, nil, err
	}
	fe := pgproto3.NewFrontend(conn, conn)
	c := &Conn{conn: conn, frontend: fe}
	if os.Getenv("TTT_XPROTO_TRACE") != "" {
		fe.Trace(os.Stderr, pgproto3.TracerOptions{})
	}

	fe.Send(&pgproto3.StartupMessage{
		ProtocolVersion: 196608,
		Parameters:      map[string]string{"user": user, "database": database},
	})
	if err := fe.Flush(); err != nil {
		conn.Close()
		return nil, nil, err
	}

	var handshake []pgproto3.BackendMessage
	for {
		msg, err := fe.Receive()
		if err != nil {
			conn.Close()
			return nil, handshake, err
		}
		switch m := msg.(type) {
		case *pgproto3.AuthenticationOk:
		case *pgproto3.ErrorResponse:
			conn.Close()
			return nil, handshake, fmt.Errorf("startup failed: %s", m.Message)
		case *pgproto3.ReadyForQuery:
			return c, handshake, nil
		default:
			if _, ok := msg.(pgproto3.AuthenticationResponseMessage); ok {
				conn.Close()
				return nil, handshake, errors.New("server requires authentication; tests need trust auth in pg_hba.conf")
			}
			handshake = append(handshake, msg)
		}
	}
}

func (c *Conn) Close() error { return c.conn.Close() }

// SimpleQuery returns the exact wire encoding of every backend message
// up to (and including) ReadyForQuery. Frames are captured immediately:
// pgproto3.Frontend reuses its internal message structs across Receive calls.
func (c *Conn) SimpleQuery(query string) ([][]byte, error) {
	c.frontend.Send(&pgproto3.Query{String: query})
	if err := c.frontend.Flush(); err != nil {
		return nil, err
	}
	var out [][]byte
	for {
		msg, err := c.frontend.Receive()
		if err != nil {
			return out, err
		}
		out = append(out, mustEncode(msg))
		if _, ok := msg.(*pgproto3.ReadyForQuery); ok {
			return out, nil
		}
	}
}

func mustEncode(m pgproto3.BackendMessage) []byte {
	b, err := m.Encode(nil)
	if err != nil {
		panic(err)
	}
	return b
}
