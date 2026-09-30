-- Test transaction_transients_trace module: the ttt.owns_session_objs GUC
-- should reflect whether the current session owns any temporary relations.

-- Initially the module is not loaded; the custom GUC is unknown and reads as
-- an empty/off value.  Load the module to install the hooks.
LOAD 'transaction_transients_trace';

-- No temp relations yet: GUC should be off.
SHOW ttt.owns_session_objs;

-- Create a temp table.  The ProcessUtility hook recalculates and reports
-- the GUC right away.
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

-- Cleanup.
DROP FUNCTION make_temp();
