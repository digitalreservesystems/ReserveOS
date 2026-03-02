package storaged

import "database/sql"

func migrate(db *sql.DB) error {
	stmts := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS counters (name TEXT PRIMARY KEY, value INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS address_slots (
			slot_id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id TEXT NULL,
			purpose TEXT NOT NULL,
			derivation_index INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NULL,
			status TEXT NOT NULL,
			address_hash BLOB NULL,
			seen_txid TEXT NULL,
			seen_at INTEGER NULL,
			confirmed_height INTEGER NULL,
			confirmed_txid TEXT NULL,
			confirmed_at INTEGER NULL,
			UNIQUE(purpose, derivation_index)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_slots_hash ON address_slots(address_hash);`,
		`CREATE INDEX IF NOT EXISTS idx_slots_status ON address_slots(status);`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil { return err }
	}
	return nil
}
