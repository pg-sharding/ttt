LOAD 'transaction_transients_trace';

-- Issue #7: rollback of an aborted transaction must not open catalogs
CREATE TEMP TABLE t(a int);
BEGIN;
DROP TABLE t;
SELECT 1/0;
ROLLBACK;

SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.t');

-- Clean rollback restores the temp table: the GUC must still say on
CREATE TEMP TABLE rt(a int);
BEGIN;
DROP TABLE rt;
ROLLBACK;

SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.rt');
DROP TABLE rt;
