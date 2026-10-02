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
	Request  []string
	Response []pgproto3.BackendMessage
}

func encodeExpects(msgs []pgproto3.BackendMessage) [][]byte {
	out := make([][]byte, len(msgs))
	for i, m := range msgs {
		out[i] = mustEncode(m)
	}
	return out
}

// equal compares the expected message wire encoding byte-wise.
func equal(exp pgproto3.BackendMessage, got []byte) bool {
	return bytes.Equal(mustEncode(exp), got)
}

func runTestFlow(t *testing.T, c *Conn, tt []MessageGroup) {
	t.Helper()
	for _, grp := range tt {
		msgs, err := c.RunQueries(grp.Request)
		if err != nil {
			t.Fatalf("%s: %v; messages: %x", grp.Request, err, msgs)
		}
		if len(msgs) != len(grp.Response) {
			t.Fatalf("%s: expected %x, got %x", grp.Request, encodeExpects(grp.Response), msgs)
		}
		for i, e := range grp.Response {
			if !equal(e, msgs[i]) {
				t.Fatalf("%s: message %d: expected %x, got %x", grp.Request, i, mustEncode(e), msgs[i])
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
			Request: []string{
				"LOAD 'transaction_transients_trace';",
				"SELECT 1;",
				"SET enable_seqscan TO off;",
				"BEGIN;",
				"COMMIT;",
				"CREATE TEMP TABLE wr_t(a int);",
				"CREATE TEMP TABLE wr_t2(a int);",
				"DROP TABLE wr_t2;",
				"BEGIN; DROP TABLE wr_t; ROLLBACK;",
				"SELECT current_setting('ttt.owns_session_objs');",
				"DROP TABLE wr_t;",
			},
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("LOAD")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("?column?"), DataTypeOID: 23, DataTypeSize: 4, TypeModifier: -1}}},
				&pgproto3.DataRow{Values: [][]byte{[]byte("1")}},
				&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("SET")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.CommandComplete{CommandTag: []byte("COMMIT")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: gucName, Value: "on"},
				&pgproto3.CommandComplete{CommandTag: []byte("CREATE TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("CREATE TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.CommandComplete{CommandTag: []byte("ROLLBACK")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("current_setting"), DataTypeOID: 25, DataTypeSize: -1, TypeModifier: -1}}},
				&pgproto3.DataRow{Values: [][]byte{[]byte("on")}},
				&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: gucName, Value: "off"},
				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
	}

	runTestFlow(t, conn, tt)
}

func TestXprotoSavepoint(t *testing.T) {
	conn := connectOrSkip(t)

	tt := []MessageGroup{
		{
			Request: []string{
				"LOAD 'transaction_transients_trace';",
				"CREATE TEMP TABLE sp(a int);",
				"BEGIN;",
				"SAVEPOINT s;",
				"DROP TABLE sp;",
				"ROLLBACK TO SAVEPOINT s;",
				"COMMIT;",
				"SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.sp');",
				"DROP TABLE sp;",
			},
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("LOAD")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: gucName, Value: "on"},
				&pgproto3.CommandComplete{CommandTag: []byte("CREATE TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.CommandComplete{CommandTag: []byte("SAVEPOINT")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.CommandComplete{CommandTag: []byte("ROLLBACK")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.CommandComplete{CommandTag: []byte("COMMIT")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{
					{Name: []byte("current_setting"), DataTypeOID: 25, DataTypeSize: -1, TypeModifier: -1},
					{Name: []byte("to_regclass"), DataTypeOID: 2205, DataTypeSize: 4, TypeModifier: -1},
				}},
				&pgproto3.DataRow{Values: [][]byte{[]byte("on"), []byte("sp")}},
				&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: gucName, Value: "off"},
				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
	}

	runTestFlow(t, conn, tt)
}
