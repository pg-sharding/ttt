package xproto

import (
	"bytes"
	"encoding/binary"
	"os"
	"strconv"
	"testing"
)

const gucName = "ttt.owns_session_objs"

type MessageGroup struct {
	Request  string
	Response []Message
}

// Message is a pgproto3-style backend message: Equal compares the expected
// wire encoding byte-wise against the received raw message.
type Message interface {
	Encode(dst []byte) []byte
	Equal(raw RawMessage) bool
}

// rawWire rebuilds the full wire encoding of a received message.
func rawWire(m RawMessage) []byte {
	buf := make([]byte, 5+len(m.Body))
	buf[0] = m.Type
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(m.Body)+4))
	copy(buf[5:], m.Body)
	return buf
}

func encodeExpects(msgs []Message) [][]byte {
	out := make([][]byte, len(msgs))
	for i, m := range msgs {
		out[i] = m.Encode(nil)
	}
	return out
}

func rawWires(msgs []RawMessage) [][]byte {
	out := make([][]byte, len(msgs))
	for i, m := range msgs {
		out[i] = rawWire(m)
	}
	return out
}

type CommandComplete struct {
	CommandTag []byte
}

func (m *CommandComplete) Encode(dst []byte) []byte {
	var body []byte
	body = append(body, m.CommandTag...)
	body = append(body, 0)
	return appendMessage(dst, 'C', body)
}

func (m *CommandComplete) Equal(raw RawMessage) bool {
	return bytes.Equal(m.Encode(nil), rawWire(raw))
}

type ParameterStatus struct {
	Name  []byte
	Value []byte
}

func (m *ParameterStatus) Encode(dst []byte) []byte {
	var body []byte
	body = append(body, m.Name...)
	body = append(body, 0)
	body = append(body, m.Value...)
	body = append(body, 0)
	return appendMessage(dst, 'S', body)
}

func (m *ParameterStatus) Equal(raw RawMessage) bool {
	return bytes.Equal(m.Encode(nil), rawWire(raw))
}

type ReadyForQuery struct {
	TxStatus byte
}

func (m *ReadyForQuery) Encode(dst []byte) []byte {
	return appendMessage(dst, 'Z', []byte{m.TxStatus})
}

func (m *ReadyForQuery) Equal(raw RawMessage) bool {
	return bytes.Equal(m.Encode(nil), rawWire(raw))
}

type FieldDescription struct {
	Name                 []byte
	TableOID             uint32
	TableAttributeNumber uint16
	DataTypeOID          uint32
	DataTypeSize         int16
	TypeModifier         int32
	Format               int16
}

type RowDescription struct {
	Fields []FieldDescription
}

func (m *RowDescription) Encode(dst []byte) []byte {
	var body []byte
	body = binary.BigEndian.AppendUint16(body, uint16(len(m.Fields)))
	for _, f := range m.Fields {
		body = append(body, f.Name...)
		body = append(body, 0)
		body = binary.BigEndian.AppendUint32(body, f.TableOID)
		body = binary.BigEndian.AppendUint16(body, f.TableAttributeNumber)
		body = binary.BigEndian.AppendUint32(body, f.DataTypeOID)
		body = binary.BigEndian.AppendUint16(body, uint16(f.DataTypeSize))
		body = binary.BigEndian.AppendUint32(body, uint32(f.TypeModifier))
		body = binary.BigEndian.AppendUint16(body, uint16(f.Format))
	}
	return appendMessage(dst, 'T', body)
}

func (m *RowDescription) Equal(raw RawMessage) bool {
	return bytes.Equal(m.Encode(nil), rawWire(raw))
}

type DataRow struct {
	Values [][]byte
}

func (m *DataRow) Encode(dst []byte) []byte {
	var body []byte
	body = binary.BigEndian.AppendUint16(body, uint16(len(m.Values)))
	for _, v := range m.Values {
		if v == nil {
			body = binary.BigEndian.AppendUint32(body, 0xffffffff)
			continue
		}
		body = binary.BigEndian.AppendUint32(body, uint32(len(v)))
		body = append(body, v...)
	}
	return appendMessage(dst, 'D', body)
}

func (m *DataRow) Equal(raw RawMessage) bool {
	return bytes.Equal(m.Encode(nil), rawWire(raw))
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
			if !e.Equal(msgs[i]) {
				t.Fatalf("%s: message %d: expected %x, got %x", grp.Request, i, e.Encode(nil), rawWire(msgs[i]))
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
			Response: []Message{
				&CommandComplete{CommandTag: []byte("LOAD")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "SELECT 1;",
			Response: []Message{
				&RowDescription{Fields: []FieldDescription{{Name: []byte("?column?"), DataTypeOID: 23, DataTypeSize: 4, TypeModifier: -1}}},
				&DataRow{Values: [][]byte{[]byte("1")}},
				&CommandComplete{CommandTag: []byte("SELECT 1")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "SET enable_seqscan TO off;",
			Response: []Message{
				&CommandComplete{CommandTag: []byte("SET")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "BEGIN;",
			Response: []Message{
				&CommandComplete{CommandTag: []byte("BEGIN")},
				&ReadyForQuery{TxStatus: 'T'},
			},
		},
		{
			Request: "COMMIT;",
			Response: []Message{
				&CommandComplete{CommandTag: []byte("COMMIT")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "CREATE TEMP TABLE wr_t(a int);",
			Response: []Message{
				&ParameterStatus{Name: []byte(gucName), Value: []byte("on")},
				&CommandComplete{CommandTag: []byte("CREATE TABLE")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "CREATE TEMP TABLE wr_t2(a int);",
			Response: []Message{
				&CommandComplete{CommandTag: []byte("CREATE TABLE")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "DROP TABLE wr_t2;",
			Response: []Message{
				&CommandComplete{CommandTag: []byte("DROP TABLE")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "BEGIN; DROP TABLE wr_t; ROLLBACK;",
			Response: []Message{
				&CommandComplete{CommandTag: []byte("BEGIN")},
				&CommandComplete{CommandTag: []byte("DROP TABLE")},
				&CommandComplete{CommandTag: []byte("ROLLBACK")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "SELECT current_setting('ttt.owns_session_objs');",
			Response: []Message{
				&RowDescription{Fields: []FieldDescription{{Name: []byte("current_setting"), DataTypeOID: 25, DataTypeSize: -1, TypeModifier: -1}}},
				&DataRow{Values: [][]byte{[]byte("on")}},
				&CommandComplete{CommandTag: []byte("SELECT 1")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
		{
			Request: "DROP TABLE wr_t;",
			Response: []Message{
				&ParameterStatus{Name: []byte(gucName), Value: []byte("off")},
				&CommandComplete{CommandTag: []byte("DROP TABLE")},
				&ReadyForQuery{TxStatus: 'I'},
			},
		},
	}

	runTestFlow(t, conn, tt)
}
