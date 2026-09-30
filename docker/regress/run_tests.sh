#!/bin/bash
set -e

# Initialize PostgreSQL
su postgres -c "pg_ctl init -D /var/lib/postgresql/data"

# Start PostgreSQL
su postgres -c "pg_ctl start -D /var/lib/postgresql/data -o '-c listen_addresses=localhost'"

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
