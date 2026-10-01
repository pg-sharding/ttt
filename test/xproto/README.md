# xproto test

Raw PostgreSQL wire-protocol test for the `transaction_transients_trace`
extension. It pins down **when** the extension sends its `ParameterStatus`
report for the custom GUC `ttt.owns_session_objs` over the **simple query
protocol**, in the spirit of pg-sharding/spqr's `test/xproto` tests — but
against a real PostgreSQL backend and with zero external Go dependencies
(the client implements the startup handshake, including SCRAM-SHA-256
authentication, and simple `Q`uery messages by hand).

## What it asserts

- `LOAD 'transaction_transients_trace';` and statements that do not change
  the tracked value (`SELECT 1`, `SET`, a no-op `BEGIN`/`COMMIT`) produce
  **no** `ParameterStatus` report.
- `CREATE TEMP TABLE wr_t(a int);` produces **exactly one** report
  `ttt.owns_session_objs = on`, ordered **before** the statement's
  `ReadyForQuery`.
- A second `CREATE TEMP TABLE` (value unchanged) produces **no duplicate**
  report.
- `BEGIN; DROP TABLE wr_t; ROLLBACK;` (sent as one simple-protocol query
  string) produces **no** report: `XACT_EVENT_ABORT` forgets the pending
  value and the dropped table is restored (so both tables still exist).
- `DROP TABLE wr_t2;` still produces **no** report: `wr_t` remains, so
  the value is unchanged.
- `DROP TABLE wr_t;` — dropping the last session-owned object — produces
  exactly one report `ttt.owns_session_objs = off` before `ReadyForQuery`.

## Running

The test connects to a running PostgreSQL that has the extension
installed. `LOAD` is performed by the test itself, so no
`session_preload_libraries` configuration is required.

Connection parameters (with defaults):

| Env         | Default     |
|-------------|-------------|
| `PGHOST`    | `127.0.0.1` |
| `PGPORT`    | `5437`      |
| `PGUSER`    | `postgres`  |
| `PGPASSWORD`| `1234`      |
| `PGDATABASE`| `postgres`  |

```sh
PGHOST=127.0.0.1 PGPORT=5437 PGUSER=postgres PGPASSWORD=1234 go test -v ./...
```

If the server is unreachable the test **skips** (so it is inert by
default); connect-error skipping will be replaced by proper CI wiring
later.
