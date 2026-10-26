package xproto

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgproto3"
)

const lastWriteLSNGUCName = "ttt.last_write_lsn"

func writeLSN(t *testing.T, frame []byte) (uint64, bool) {
	t.Helper()
	if frame[0] != 'S' {
		return 0, false
	}
	var msg pgproto3.ParameterStatus
	if err := msg.Decode(frame[5:]); err != nil {
		t.Fatal(err)
	}
	if msg.Name != lastWriteLSNGUCName {
		return 0, false
	}
	parts := strings.Split(msg.Value, "/")
	if len(parts) != 2 {
		t.Fatalf("invalid LSN feedback: %q", msg.Value)
	}
	hi, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil {
		t.Fatal(err)
	}
	lo, err := strconv.ParseUint(parts[1], 16, 32)
	if err != nil {
		t.Fatal(err)
	}
	return hi<<32 | lo, true
}

func TestLastWriteLSN(t *testing.T) {
	conn := connectOrSkip(t)

	tt := []MessageGroup{
		{
			Request: []string{
				"LOAD 'transaction_transients_trace';",
				"CREATE TABLE wire_lsn_test (i int);",
				"SELECT 1;",
				"INSERT INTO wire_lsn_test VALUES (1);",
				"BEGIN;",
				"INSERT INTO wire_lsn_test VALUES (2);",
				"COMMIT;",
				"BEGIN;",
				"INSERT INTO wire_lsn_test VALUES (3);",
				"ROLLBACK;",
				"SELECT 1;",
				"SET synchronous_commit TO off;",
				"INSERT INTO wire_lsn_test VALUES (4);",
				"CREATE FUNCTION wire_lsn_write() RETURNS void LANGUAGE SQL AS 'INSERT INTO wire_lsn_test VALUES (5)';",
				"SELECT wire_lsn_write();",
				"DROP FUNCTION wire_lsn_write();",
				"BEGIN; INSERT INTO wire_lsn_test VALUES (6); COMMIT; BEGIN; INSERT INTO wire_lsn_test VALUES (7); ROLLBACK;",
				"DROP TABLE wire_lsn_test;",
			},
			Response: []pgproto3.BackendMessage{
				&pgproto3.CommandComplete{CommandTag: []byte("LOAD")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("CREATE TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("?column?"), DataTypeOID: 23, DataTypeSize: 4, TypeModifier: -1}}},
				&pgproto3.DataRow{Values: [][]byte{[]byte("1")}},
				&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("COMMIT")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'T'},

				&pgproto3.CommandComplete{CommandTag: []byte("ROLLBACK")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("?column?"), DataTypeOID: 23, DataTypeSize: 4, TypeModifier: -1}}},
				&pgproto3.DataRow{Values: [][]byte{[]byte("1")}},
				&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("SET")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("CREATE FUNCTION")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("wire_lsn_write"), DataTypeOID: 2278, DataTypeSize: 4, TypeModifier: -1}}},
				&pgproto3.DataRow{Values: [][]byte{nil}},
				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("DROP FUNCTION")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")},
				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("COMMIT")},
				&pgproto3.CommandComplete{CommandTag: []byte("BEGIN")},
				&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")},
				&pgproto3.CommandComplete{CommandTag: []byte("ROLLBACK")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},

				&pgproto3.ParameterStatus{Name: lastWriteLSNGUCName, Value: "*"},
				&pgproto3.CommandComplete{CommandTag: []byte("DROP TABLE")},
				&pgproto3.ReadyForQuery{TxStatus: 'I'},
			},
		},
	}

	runTestFlow(t, conn, tt)
}
