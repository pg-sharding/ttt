LOAD 'transaction_transients_trace';

-- ExecutorEnd runs in parallel workers; no reporting must happen there
SET min_parallel_table_scan_size = 0;
SET parallel_setup_cost = 0;
SET max_parallel_workers_per_gather = 2;
CREATE TABLE customer_orders AS
  SELECT generate_series(1, 20000) AS s, 1 AS order_total;
ALTER TABLE customer_orders SET (parallel_workers = 2);
ANALYZE customer_orders;

EXPLAIN (COSTS OFF)
  SELECT SUM(order_total) FROM customer_orders;
SELECT SUM(order_total) FROM customer_orders;
SHOW ttt.owns_session_objs;

-- Same, while the session owns a temp relation.
CREATE TEMP TABLE z4();
SELECT SUM(order_total) FROM customer_orders;
SHOW ttt.owns_session_objs;

DROP TABLE z4;
DROP TABLE customer_orders;
