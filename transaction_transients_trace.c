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

#include "access/table.h"
#include "catalog/pg_class.h"
#include "catalog/namespace.h"
#include "executor/executor.h"
#include "libpq/protocol.h"
#include "libpq/pqformat.h"
#include "tcop/dest.h"
#include "tcop/utility.h"
#include "tcop/tcopprot.h"
#include "utils/guc.h"
#include "utils/guc_tables.h"
#include "utils/fmgroids.h"

PG_MODULE_MAGIC_EXT(
					.name = "transaction_transients_trace",
					.version = PG_VERSION
);

#define TTT_GUC_NAME "ttt.session_owns_temp_rels"

/* GUC variables */
static bool ttt_session_owns_temp_rels = false;
static bool ttt_session_owns_temp_rels_valid = false;

/* Saved hook values */
static ExecutorEnd_hook_type prev_ExecutorEnd = NULL;
static ProcessUtility_hook_type prev_ProcessUtility = NULL;

/* Forward declarations of hook functions */
static void ttt_ExecutorEnd(QueryDesc *queryDesc);
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
	Relation relRelation;
	ScanKeyData skey;
	SysScanDesc scan;
	HeapTuple	tuple;

	/* Prepare to scan pg_index for entries having indrelid = this rel. */
	relRelation = table_open(RelationRelationId, AccessShareLock);
	ScanKeyInit(&skey,
				Anum_pg_class_relnamespace,
				BTEqualStrategyNumber, F_OIDEQ,
				ObjectIdGetDatum(nsp));

	scan = systable_beginscan(relRelation, ClassNameNspIndexId, true,
							  NULL, 1, &skey);

	if (HeapTupleIsValid(tuple = systable_getnext(scan)))
	{
		ttt_session_owns_temp_rels = true;
	}
	else
	{
		ttt_session_owns_temp_rels = false;
	}

	systable_endscan(scan);
	table_close(relRelation, AccessShareLock);
}

/*
 * ExecutorEnd hook: simple proxy to the standard implementation.
 */
static void
ttt_ExecutorEnd(QueryDesc *queryDesc)
{
	if (!ttt_session_owns_temp_rels_valid)
	{
		Oid tempNamespace;
		Oid tempTOASTNamespace;
		GetTempNamespaceState(&tempNamespace, &tempTOASTNamespace);
		tttRecalculate(tempNamespace);
		ttt_session_owns_temp_rels_valid = true;
 		ReportGUCOption();
	}

	if (prev_ExecutorEnd)
		prev_ExecutorEnd(queryDesc);
	else
		standard_ExecutorEnd(queryDesc);
}

/*
 * ProcessUtility hook: simple proxy to the standard implementation.
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
	/* Store invalidtion request */
	ttt_session_owns_temp_rels_valid = false;

	if (prev_ProcessUtility)
		prev_ProcessUtility(pstmt, queryString, readOnlyTree,
							context, params, queryEnv, dest, qc);
	else
		standard_ProcessUtility(pstmt, queryString, readOnlyTree,
								context, params, queryEnv, dest, qc);
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

	/* Install hooks. */
	prev_ExecutorEnd = ExecutorEnd_hook;
	ExecutorEnd_hook = ttt_ExecutorEnd;

	prev_ProcessUtility = ProcessUtility_hook;
	ProcessUtility_hook = ttt_ProcessUtility;
}
