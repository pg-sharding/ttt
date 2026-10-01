package xproto

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

const gucName = "ttt.owns_session_objs"

type MessageGroup struct {
	Request  string
	Response []Expectation
}

type Expectation struct {
	kind   byte
	s1, s2 string
	b      byte
	vals   []string
}

func CommandComplete(tag string) Expectation { return Expectation{kind: 'C', s1: tag} }
func ParameterStatus(name, value string) Expectation {
	return Expectation{kind: 'S', s1: name, s2: value}
}
func ReadyForQuery(txStatus byte) Expectation { return Expectation{kind: 'Z', b: txStatus} }
func RowDescription() Expectation             { return Expectation{kind: 'T'} }
func DataRow(values ...string) Expectation    { return Expectation{kind: 'D', vals: values} }

func (e Expectation) String() string {
	switch e.kind {
	case 'C':
		return fmt.Sprintf("CommandComplete(%s)", e.s1)
	case 'S':
		return fmt.Sprintf("ParameterStatus(%s=%s)", e.s1, e.s2)
	case 'Z':
		return fmt.Sprintf("ReadyForQuery(%c)", e.b)
	case 'D':
		return fmt.Sprintf("DataRow(%s)", strings.Join(e.vals, ","))
	default:
		return string(e.kind)
	}
}

func (m RawMessage) String() string {
	switch m.Type {
	case 'C':
		tag, _ := CString(m.Body)
		return fmt.Sprintf("CommandComplete(%s)", tag)
	case 'S':
		name, value := m.ParameterStatus()
		return fmt.Sprintf("ParameterStatus(%s=%s)", name, value)
	case 'Z':
		return fmt.Sprintf("ReadyForQuery(%c)", m.Body[0])
	case 'D':
		n := int(binary.BigEndian.Uint16(m.Body[:2]))
		rest := m.Body[2:]
		var vals []string
		for i := 0; i < n && len(rest) >= 4; i++ {
			l := int32(binary.BigEndian.Uint32(rest[:4]))
			rest = rest[4:]
			if l < 0 {
				vals = append(vals, "NULL")
				continue
			}
			vals = append(vals, string(rest[:l]))
			rest = rest[l:]
		}
		return fmt.Sprintf("DataRow(%s)", strings.Join(vals, ","))
	default:
		return string(m.Type)
	}
}

func (e Expectation) matches(m RawMessage) bool {
	if e.kind != m.Type {
		return false
	}
	switch e.kind {
	case 'C':
		tag, _ := CString(m.Body)
		return tag == e.s1
	case 'S':
		name, value := m.ParameterStatus()
		return name == e.s1 && value == e.s2
	case 'Z':
		return m.Body[0] == e.b
	case 'D':
		return m.String() == e.String()
	}
	return true
}

func runTestFlow(t *testing.T, c *Conn, tt []MessageGroup) {
	t.Helper()
	for _, grp := range tt {
		msgs, err := c.SimpleQuery(grp.Request)
		if err != nil {
			t.Fatalf("%s: %v; messages: %v", grp.Request, err, msgs)
		}
		if len(msgs) != len(grp.Response) {
			t.Fatalf("%s: expected %v, got %v", grp.Request, grp.Response, msgs)
		}
		for i, e := range grp.Response {
			if !e.matches(msgs[i]) {
				t.Fatalf("%s: expected %v, got %v", grp.Request, grp.Response, msgs)
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
			Response: []Expectation{
				CommandComplete("LOAD"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "SELECT 1;",
			Response: []Expectation{
				RowDescription(),
				DataRow("1"),
				CommandComplete("SELECT 1"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "SET enable_seqscan TO off;",
			Response: []Expectation{
				CommandComplete("SET"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "BEGIN;",
			Response: []Expectation{
				CommandComplete("BEGIN"),
				ReadyForQuery('T'),
			},
		},
		{
			Request: "COMMIT;",
			Response: []Expectation{
				CommandComplete("COMMIT"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "CREATE TEMP TABLE wr_t(a int);",
			Response: []Expectation{
				ParameterStatus(gucName, "on"),
				CommandComplete("CREATE TABLE"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "CREATE TEMP TABLE wr_t2(a int);",
			Response: []Expectation{
				CommandComplete("CREATE TABLE"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "DROP TABLE wr_t2;",
			Response: []Expectation{
				CommandComplete("DROP TABLE"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "BEGIN; DROP TABLE wr_t; ROLLBACK;",
			Response: []Expectation{
				CommandComplete("BEGIN"),
				CommandComplete("DROP TABLE"),
				CommandComplete("ROLLBACK"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "SELECT current_setting('ttt.owns_session_objs');",
			Response: []Expectation{
				RowDescription(),
				DataRow("on"),
				CommandComplete("SELECT 1"),
				ReadyForQuery('I'),
			},
		},
		{
			Request: "DROP TABLE wr_t;",
			Response: []Expectation{
				ParameterStatus(gucName, "off"),
				CommandComplete("DROP TABLE"),
				ReadyForQuery('I'),
			},
		},
	}

	runTestFlow(t, conn, tt)
}
