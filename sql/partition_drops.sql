LOAD 'transaction_transients_trace';

-- session-objects detection is namespace-wide: the GUC must be on
-- while any temp object exists, regardless of per-partition drops

SELECT current_setting('ttt.owns_session_objs');

-- an empty partitioned temp table alone: is it counted as an object?
CREATE TEMP TABLE pt(a int) PARTITION BY RANGE (a);
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.pt');

CREATE TEMP TABLE pt_p1 PARTITION OF pt FOR VALUES FROM (0) TO (10);
CREATE TEMP TABLE pt_p2 PARTITION OF pt FOR VALUES FROM (10) TO (20);
CREATE TEMP TABLE pt_p3 PARTITION OF pt FOR VALUES FROM (20) TO (30);
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.pt_p1'),
       to_regclass('pg_temp.pt_p2'), to_regclass('pg_temp.pt_p3');

-- a statement that must not change the value
SET client_min_messages TO DEFAULT;
SELECT current_setting('ttt.owns_session_objs');

-- drop partitions one by one: value must remain on while others remain
DROP TABLE pt_p1;
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.pt_p1'),
       to_regclass('pg_temp.pt_p2'), to_regclass('pg_temp.pt_p3');

DROP TABLE pt_p2;
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.pt_p2'),
       to_regclass('pg_temp.pt_p3');

DROP TABLE pt_p3;
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.pt_p3'),
       to_regclass('pg_temp.pt');

-- last object: dropping the partitioned parent must flip to off
DROP TABLE pt;
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.pt');

-- first object creation flips it back on
CREATE TEMP TABLE t2(a int);
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.t2');

-- create parent with all partitions, then drop the parent: everything
-- goes away in one statement and the value must flip to off
CREATE TEMP TABLE pb(a int) PARTITION BY RANGE (a);
CREATE TEMP TABLE pb_p1 PARTITION OF pb FOR VALUES FROM (1) TO (10);
CREATE TEMP TABLE tb2(a int);
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.pb_p1');
DROP TABLE pb, tb2, t2;
SELECT current_setting('ttt.owns_session_objs'), to_regclass('pg_temp.pb'),
       to_regclass('pg_temp.pb_p1'), to_regclass('pg_temp.t2');
