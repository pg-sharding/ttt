LOAD 'transaction_transients_trace';
SELECT current_setting('ttt.last_write_lsn') = '0/0' AS initially_unset;

CREATE TABLE lsn_test (i int);
SELECT current_setting('ttt.last_write_lsn')::pg_lsn > '0/0' AS ddl_reported;
SELECT current_setting('ttt.last_write_lsn') AS before_write \gset
BEGIN;
INSERT INTO lsn_test VALUES (1);
SELECT current_setting('ttt.last_write_lsn') = :'before_write' AS unchanged_before_commit;
COMMIT;
SELECT current_setting('ttt.last_write_lsn')::pg_lsn > :'before_write'::pg_lsn AS commit_reported;

SELECT current_setting('ttt.last_write_lsn') AS before_abort \gset
BEGIN;
INSERT INTO lsn_test VALUES (2);
ROLLBACK;
SELECT current_setting('ttt.last_write_lsn') = :'before_abort' AS unchanged_after_abort;
SELECT 1;
SELECT current_setting('ttt.last_write_lsn') = :'before_abort' AS unchanged_after_read;
RESET ALL;
SELECT current_setting('ttt.last_write_lsn') = :'before_abort' AS survives_reset;
SET ttt.last_write_lsn = '1/1';
SELECT current_setting('ttt.last_write_lsn') = :'before_abort' AS cannot_be_overridden;
DROP TABLE lsn_test;
