#!/bin/bash
set -e

export PATH=/usr/local/pgsql/bin:$PATH

PGDATA=/tmp/pgdata

# Initialize PostgreSQL
initdb -D $PGDATA

# Start PostgreSQL
pg_ctl start -D $PGDATA -o '-c listen_addresses=localhost'

# Wait for PostgreSQL to be ready
sleep 2

# Run regression tests
cd /transaction_transients_trace
if ! make USE_PGXS=1 installcheck PGUSER=postgres TAP_TESTS=; then
    if [ -f /transaction_transients_trace/regression.diffs ]; then
        echo "=== regression.diffs ==="
        cat /transaction_transients_trace/regression.diffs
    fi
    exit 1
fi
