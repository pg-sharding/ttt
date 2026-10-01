LOAD 'transaction_transients_trace';

-- savepoint ROLLBACK TO undoes the DROP
CREATE TEMP TABLE sp(a int);
BEGIN;
SAVEPOINT s;
DROP TABLE sp;
ROLLBACK TO SAVEPOINT s;
COMMIT;
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.sp');

-- savepoint RELEASE keeps work done inside (the first temp table is
-- created in a subtransaction and survives the outer commit)
DROP TABLE sp;
BEGIN;
SAVEPOINT s;
CREATE TEMP TABLE sp2(a int);
RELEASE SAVEPOINT s;
COMMIT;
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.sp2');
DROP TABLE sp2;

-- PL/pgSQL exception block: subtransaction abort undoes the DROP
CREATE TEMP TABLE exc(a int);
CREATE FUNCTION exc_drop_then_fail() RETURNS void AS $$
BEGIN
  DROP TABLE exc;
  PERFORM 1/0;
EXCEPTION WHEN OTHERS THEN
  NULL;
END $$ LANGUAGE plpgsql;
SELECT exc_drop_then_fail();
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.exc');
DROP TABLE exc;

-- PL/pgSQL exception block: subtransaction abort undoes the CREATE
CREATE FUNCTION exc_create_then_fail() RETURNS void AS $$
BEGIN
  CREATE TEMP TABLE exc_rolled(a int);
  PERFORM 1/0;
EXCEPTION WHEN OTHERS THEN
  NULL;
END $$ LANGUAGE plpgsql;
SELECT exc_create_then_fail();
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.exc_rolled');

-- a utility failing mid-execution must not leave a stale report
CREATE TEMP TABLE ctas AS SELECT 1 / g AS a FROM generate_series(0, 0) AS g;
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.ctas');

-- a failing duplicate CREATE aborts its implicit transaction; the
-- committed tables remain, so the flag must stay on
CREATE TEMP TABLE ok1(a int); CREATE TEMP TABLE bad(a int); CREATE TEMP TABLE bad(a int);
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.ok1'), to_regclass('pg_temp.bad');
DROP TABLE ok1;
DROP TABLE bad;
SELECT current_setting('ttt.owns_session_objs');
