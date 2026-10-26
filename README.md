# TTT

Trace temp objects owned in session

## Write feedback

`ttt.last_write_lsn` is a read-only GUC reporting the WAL insertion position
after a transaction that generated WAL commits. It includes the commit record
and is sent as a `ParameterStatus` message before `ReadyForQuery`, without a
client query. The position can include concurrent transactions' WAL.

Its initial value is `0/0`. Transactions that generate no WAL and rolled-back
transactions do not update it.
