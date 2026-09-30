ARG PG_VERSION=18
FROM postgres:${PG_VERSION}

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    postgresql-server-dev-${PG_MAJOR} \
    && rm -rf /var/lib/apt/lists/*

COPY . /transaction_transients_trace
WORKDIR /transaction_transients_trace

RUN make USE_PGXS=1 && make USE_PGXS=1 install

ENTRYPOINT [ "/transaction_transients_trace/docker/regress/run_tests.sh" ]
