package xproto

import (
	"bytes"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgproto3"
)

const gucName = "ttt.owns_session_objs"

type MessageGroup struct {
	Request  string
	Response []pgproto3.BackendMessage
}

// rawWire rebuilds the full wire encoding of a received message.
func rawWire(m RawMessage) []byte {
	return appendMessage(nil, m.Type, m.Body)
}

func encodeExpects(msgs []pgproto3.BackendMessage) [][]byte {
	out := make([][]byte, len(msgs))
	for i, m := range msgs {
		out[i] = mustEncode(m)
	}
	return out
}

func mustEncode(m pgproto3.BackendMessage) []byte {
	b, err := m.Encode(nil)
	if err != nil {
		panic(err)
	}
	return b
}

func rawWires(msgs []RawMessage) [][]byte {
	out := make([][]byte, len(msgs))
	for i, m := range msgs {
		out[i] = rawWire(m)
	}
	return out
}

// equal compares the expected message wire encoding byte-wise against
// the received raw message.
func equal(exp pgproto3.BackendMessage, raw RawMessage) bool {
	return bytes.Equal(mustEncode(exp), rawWire(raw))
}

func runTestFlow(t *testing.T, c *Conn, tt []MessageGroup) {
	t.Helper()
	for _, grp := range tt {
		msgs, err := c.SimpleQuery(grp.Request)
		if err != nil {
			t.Fatalf("%s: %v; messages: %x", grp.Request, err, rawWires(msgs))
		}
		if len(msgs) != len(grp.Response) {
			t.Fatalf("%s: expected %x, got %x", grp.Request, encodeExpects(grp.Response), rawWires(msgs))
		}
		for i, e := range grp.Response {
			if !equal(e, msgs[i]) {
				t.Fatalf("%s: message %d: expected %x, got %x", grp.Request, i, mustEncode(e), rawWire(msgs[i]))
			}
		}
	}
}

func connectOrSkip(t *testing.T) *Conn {
	t.Helper()
	explicit := false
	for _, key := range []string{"PGHOST", "PGPORT", "PGUSER", "PGDATABASE"} {
		if os.Getenv(key) != "" {
			explicit = true
			break
		}
	}
	host := envOr("PGHOST", "127.0.0.1")
	port, _ := strconv.Atoi(envOr("PGPORT", "5437"))

	conn, _, err := Connect(host, port, envOr("PGUSER", "postgres"),
		envOr("PGDATABASE", "postgres"))
	if err != nil {
		if explicit {
			t.Fatalf("PostgreSQL at %s:%d: %v", host, port, err)
		}
		t.Skipf("PostgreSQL at %s:%d unreachable: %v", host, port, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestXproto(t *testing.T) {
	conn := connectOrSkip(t)

	tt := []MessageGroup{
		{
			Request: "LOAD 'transaction_transients_trace';",
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("LOAD")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "SELECT 1;",
			Response: []pgproto3.BackendMessage{
				&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("?column?"), DataTypeOID: 23, DataTypeSize: 4, TypeModifier: -1}}},
				&pgproto3.DataRow{Values: [][]byte{[]byte("1")}},
				&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "SET enable_seqscan TO off;",
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("SET")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "BEGIN;",
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},
			},
		},
		{
			Request: "COMMIT;",
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("COMMIT")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "CREATE TEMP TABLE wr_t(a int);",
			Response: []pgproto3.BackendMessage{
				&pgproto3.ParameterStatus{Name: gucName, Value: "on"},
				&pgproto3.CommandComplete{CommandTag: []byte("CREATE TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "CREATE TEMP TABLE wr_t2(a int);",
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("CREATE TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "DROP TABLE wr_t2;",
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "BEGIN; DROP TABLE wr_t; ROLLBACK;",
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.CommandComplete{CommandTag: []byte("ROLLBACK")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "SELECT current_setting('ttt.owns_session_objs');",
			Response: []pgproto3.BackendMessage{
				&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("current_setting"), DataTypeOID: 25, DataTypeSize: -1, TypeModifier: -1}}},
				&pgproto3.DataRow{Values: [][]byte{[]byte("on")}},
				&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "DROP TABLE wr_t;",
			Response: []pgproto3.BackendMessage{
				&pgproto3.ParameterStatus{Name: gucName, Value: "off"},
				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
	}

	runTestFlow(t, conn, tt)
}
