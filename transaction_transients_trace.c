/*-------------------------------------------------------------------------
 *
 *  transaction_transients_trace.c
 *
 *	  Simple tracing for session transient (temp) relations.
 *
 * Copyright (c) 2025, PostgreSQL Global Development Group
 *
 * IDENTIFICATION
 *	  contrib/transaction_transients_trace/transaction_transients_trace.c
 *
 *-------------------------------------------------------------------------
 */
#include "postgres.h"

#include "access/genam.h"
#include "access/table.h"
#include "access/xact.h"
#include "access/relscan.h"
#include "catalog/indexing.h"
#include "catalog/pg_depend.h"
#include "catalog/pg_namespace.h"
#include "catalog/namespace.h"
#if PG_VERSION_NUM >= 170000
#include "libpq/protocol.h"
#else
#define PqMsg_ParameterStatus ('S')
#endif
#include "libpq/pqformat.h"
#include "miscadmin.h"
#include "tcop/dest.h"
#include "tcop/utility.h"
#include "tcop/tcopprot.h"
#include "utils/snapmgr.h"
#include "utils/guc.h"
#include "utils/guc_tables.h"

#if PG_VERSION_NUM < 160000
#define MarkGUCPrefixReserved(className) EmitWarningsOnPlaceholders(className)
#endif
#include "utils/fmgroids.h"

#if PG_VERSION_NUM >= 180000
PG_MODULE_MAGIC_EXT(
					.name = "transaction_transients_trace",
					.version = PG_VERSION
);
#else
PG_MODULE_MAGIC;
#endif

#define TTT_GUC_NAME "ttt.owns_session_objs"

/* GUC variables */
static bool ttt_session_owns_temp_rels = false;

/* New value read during pre-commit, reported at commit */
static bool ttt_new_owns_temp_rels = false;

/* Pending recompute signal, set by the ProcessUtility hook */
static bool ttt_pending_update = false;

/* Saved hook values */
static ProcessUtility_hook_type prev_ProcessUtility = NULL;

/* Forward declarations of hook functions */
static void ttt_XactCallback(XactEvent event, void *arg);
static void ttt_ProcessUtility(PlannedStmt *pstmt,
							   const char *queryString,
							   bool readOnlyTree,
							   ProcessUtilityContext context,
							   ParamListInfo params,
							   QueryEnvironment *queryEnv,
							   DestReceiver *dest,
							   QueryCompletion *qc);

/*
 * ReportGUCOption: if appropriate, transmit option value to frontend
 *
 * We need not transmit the value if it's the same as what we last
 * transmitted.
 */
static void
ReportGUCOption(void)
{
	char	   *val = ttt_session_owns_temp_rels ? "on" : "off";
	StringInfoData msgbuf;

	/* Don't send anything if we're not connected to a frontend. */
	if (whereToSendOutput != DestRemote)
		return;

	pq_beginmessage(&msgbuf, PqMsg_ParameterStatus);
	pq_sendstring(&msgbuf, TTT_GUC_NAME);
	pq_sendstring(&msgbuf, val);
	pq_endmessage(&msgbuf);
}

static bool
tttRecalculate(Oid nsp)
{
	Relation depRelation;
	ScanKeyData skey[2];
	SysScanDesc scan;
	bool		owns;
	/* No temp namespace: the session owns no session objects */
	if (!OidIsValid(nsp))
		return false;

	/* Look for any object depending on the temp schema */
	ScanKeyInit(&skey[0],
				Anum_pg_depend_refclassid,
				BTEqualStrategyNumber, F_OIDEQ,
				ObjectIdGetDatum(NamespaceRelationId));
	ScanKeyInit(&skey[1],
				Anum_pg_depend_refobjid,
				BTEqualStrategyNumber, F_OIDEQ,
				ObjectIdGetDatum(nsp));

	depRelation = table_open(DependRelationId, AccessShareLock);
	scan = systable_beginscan(depRelation, DependReferenceIndexId, true,
							  NULL, 2, skey);

	owns = HeapTupleIsValid(systable_getnext(scan));

	systable_endscan(scan);
	table_close(depRelation, AccessShareLock);

	return owns;
}

/*
 * ProcessUtility hook: proxy to the standard implementation, then
 * recalculate and report the GUC state.
 */
static void
ttt_ProcessUtility(PlannedStmt *pstmt,
				  const char *queryString,
				  bool readOnlyTree,
				  ProcessUtilityContext context,
				  ParamListInfo params,
				  QueryEnvironment *queryEnv,
				  DestReceiver *dest,
				  QueryCompletion *qc)
{
	if (AmRegularBackendProcess())
		ttt_pending_update = true;

	if (prev_ProcessUtility)
		prev_ProcessUtility(pstmt, queryString, readOnlyTree,
							context, params, queryEnv, dest, qc);
	else
		standard_ProcessUtility(pstmt, queryString, readOnlyTree,
								context, params, queryEnv, dest, qc);
}

/*
 * Transaction callback: recalculate and report pending updates at commit.
 */
static void
ttt_XactCallback(XactEvent event, void *arg)
{
	Oid			tempNamespace;
	Oid			tempTOASTNamespace;

	switch (event)
	{
		case XACT_EVENT_PRE_COMMIT:
			if (!ttt_pending_update)
				break;

			/* The catalog snapshot may predate the utility statement */
			InvalidateCatalogSnapshot();
			GetTempNamespaceState(&tempNamespace, &tempTOASTNamespace);
			ttt_new_owns_temp_rels = tttRecalculate(tempNamespace);
			break;

		case XACT_EVENT_COMMIT:
			if (!ttt_pending_update)
				break;
			ttt_pending_update = false;

			if (ttt_new_owns_temp_rels != ttt_session_owns_temp_rels)
			{
				ttt_session_owns_temp_rels = ttt_new_owns_temp_rels;
				ReportGUCOption();
			}
			break;

		case XACT_EVENT_ABORT:
			/* Forget the value read during pre-commit */
			ttt_pending_update = false;
			break;

		default:
			break;
	}
}

/*
 * Module load callback
 */
void
_PG_init(void)
{
	/* Define custom GUC variables. */
	DefineCustomBoolVariable(TTT_GUC_NAME,
							 "Whether the current session owns temporary relations.",
							 NULL,
							 &ttt_session_owns_temp_rels,
							 false,
							 PGC_USERSET,
							 0,
							 NULL,
							 NULL,
							 NULL);

	MarkGUCPrefixReserved("ttt");

	RegisterXactCallback(ttt_XactCallback, NULL);

	/* Install hooks. */
	prev_ProcessUtility = ProcessUtility_hook;
	ProcessUtility_hook = ttt_ProcessUtility;
}
