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
#include "libpq/protocol.h"
#include "libpq/pqformat.h"
#include "miscadmin.h"
#include "tcop/dest.h"
#include "tcop/utility.h"
#include "tcop/tcopprot.h"
#include "utils/snapmgr.h"
#include "utils/guc.h"
#include "utils/guc_tables.h"
#include "utils/fmgroids.h"

PG_MODULE_MAGIC_EXT(
					.name = "transaction_transients_trace",
					.version = PG_VERSION
);

#define TTT_GUC_NAME "ttt.owns_session_objs"

/* GUC variables */
static bool ttt_session_owns_temp_rels = false;

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

static void
tttRecalculate(Oid nsp)
{
	Relation depRelation;
	ScanKeyData skey[2];
	SysScanDesc scan;
	HeapTuple tuple;

	/* No temp namespace: the session owns no session objects */
	if (!OidIsValid(nsp))
	{
		ttt_session_owns_temp_rels = false;
		return;
	}

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

	if (HeapTupleIsValid(tuple = systable_getnext(scan)))
	{
		ttt_session_owns_temp_rels = true;
	}
	else
	{
		ttt_session_owns_temp_rels = false;
	}

	systable_endscan(scan);
	table_close(depRelation, AccessShareLock);
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

	if (event != XACT_EVENT_PRE_COMMIT || !ttt_pending_update)
		return;

	ttt_pending_update = false;

	/* The catalog snapshot may predate the utility statement */
	InvalidateCatalogSnapshot();
	GetTempNamespaceState(&tempNamespace, &tempTOASTNamespace);
	tttRecalculate(tempNamespace);
	ReportGUCOption();
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
