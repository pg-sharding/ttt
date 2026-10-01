# xproto test

Raw PostgreSQL wire-protocol test asserting when the
`transaction_transients_trace` extension reports `ttt.owns_session_objs`
via `ParameterStatus` over the simple query protocol. Run with `go test -v ./...`
against a live PostgreSQL; the test skips if the server is unreachable.
