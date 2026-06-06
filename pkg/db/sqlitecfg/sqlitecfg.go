// Package sqlitecfg builds the SQLite DSN every OpenExec database open must
// use.
//
// History: connections used mattn-style DSN parameters
// ("?_foreign_keys=on&_journal_mode=WAL"), but the driver registered as
// "sqlite" is modernc.org/sqlite, which silently ignores that syntax.
// Verified empirically: those connections actually ran with
// journal_mode=delete, foreign_keys=0, busy_timeout=0 — no WAL concurrency,
// no FK enforcement, and instant "database is locked" errors under
// concurrent writers (e.g. the daemon and a light-mode mcp-serve client
// sharing .openexec/openexec.db).
//
// modernc applies "_pragma=..." DSN parameters on EVERY pooled connection,
// which is required: busy_timeout and foreign_keys are per-connection
// settings, so a one-time Exec after sql.Open would not cover connections
// the pool opens later.
package sqlitecfg

// DSN returns the connection string for an OpenExec SQLite database:
// WAL journaling (multi-process readers + writer), foreign key enforcement,
// and a 5s busy timeout so concurrent writers wait instead of failing.
func DSN(dbPath string) string {
	return dbPath + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
}

// ReadOnlyDSN returns a read-only connection string with the same busy
// timeout (readers can still hit the write lock during checkpoints).
func ReadOnlyDSN(dbPath string) string {
	return dbPath + "?mode=ro&_pragma=busy_timeout(5000)"
}
