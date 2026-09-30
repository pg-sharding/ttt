LOAD 'transaction_transients_trace';

-- ExecutorEnd runs in parallel workers; no reporting must happen there
BEGIN;
SET LOCAL min_parallel_table_scan_size = 0;
SET LOCAL parallel_setup_cost = 0;
SET LOCAL max_parallel_workers_per_gather = 2;
CREATE TABLE customer_orders AS
  SELECT generate_series(1, 20000) AS s, 1 AS order_total;
ALTER TABLE customer_orders SET (parallel_workers = 2);
ANALYZE customer_orders;

EXPLAIN (COSTS OFF)
  SELECT SUM(order_total) FROM customer_orders;
SELECT SUM(order_total) FROM customer_orders;

CREATE TEMP TABLE z4();
SELECT SUM(order_total) FROM customer_orders;
SELECT 1;
SHOW ttt.owns_session_objs;

ROLLBACK;
