-- Test transaction_transients_trace module: the ttt.owns_session_objs GUC
-- should reflect whether the current session owns any temporary relations.

-- Initially the module is not loaded; the custom GUC is unknown and reads as
-- an empty/off value.  Load the module to install the hooks.
LOAD 'transaction_transients_trace';

-- No temp relations yet: GUC should be off.
SHOW ttt.owns_session_objs;

-- Create a temp table.  The GUC is recalculated and reported at the end
-- of the utility statement.
CREATE TEMP TABLE z2();
SHOW ttt.owns_session_objs;

-- Drop the temp table.
DROP TABLE z2;
SHOW ttt.owns_session_objs;

-- Now test creating a temp table from within a PL/pgSQL function.  The
-- nested CREATE TEMP TABLE still goes through ProcessUtility.
CREATE FUNCTION make_temp() RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
  CREATE TEMP TABLE z3();
END $$;

SELECT make_temp();
SHOW ttt.owns_session_objs;

DROP TABLE z3;
SHOW ttt.owns_session_objs;

-- Functions in pg_temp are session objects too.
CREATE FUNCTION pg_temp.f() RETURNS int LANGUAGE sql AS $$ SELECT 1 $$;
SELECT 1;
SHOW ttt.owns_session_objs;

DROP FUNCTION pg_temp.f();
SELECT 1;
SHOW ttt.owns_session_objs;

-- A rolled back transaction must not leave the GUC on.
BEGIN;
CREATE TEMP TABLE z5();
SHOW ttt.owns_session_objs;
ROLLBACK;
SHOW ttt.owns_session_objs;

-- Cleanup.
DROP FUNCTION make_temp();
