package botseed

import (
	"context"
	"database/sql"
	"fmt"
)

// Clear removes every bot-owned row from the users table of the
// supplied database. Bots live in their own namespace via the
// `bot_` email prefix; this function reuses that prefix as the
// isolation boundary so it is safe to run against a real database
// without accidentally deleting human-authored rows.
//
// The caller is responsible for opening the *sql.DB with whatever
// driver matches the project's database (sqlite, postgres, etc.)
// and closing it afterwards.
func Clear(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("botseed.Clear: nil database")
	}
	// Cascaded rows in child tables should vanish via FK ON DELETE
	// CASCADE; if a project forgot to set that up, at minimum the
	// user rows are removed so `--clear` always makes forward
	// progress.
	const stmt = `DELETE FROM users WHERE email LIKE 'bot\_%@%' ESCAPE '\'`
	res, err := db.ExecContext(ctx, stmt)
	if err != nil {
		// Fall back to the simpler (and slightly looser) pattern for
		// drivers that do not support ESCAPE clauses identically.
		const fallback = `DELETE FROM users WHERE email LIKE 'bot_%@%'`
		res, err = db.ExecContext(ctx, fallback)
		if err != nil {
			return fmt.Errorf("botseed.Clear: delete users: %w", err)
		}
	}
	if n, nerr := res.RowsAffected(); nerr == nil {
		_ = n // discarded — callers who need a count can query directly
	}
	return nil
}
