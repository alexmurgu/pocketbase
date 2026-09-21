package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// The original PostgreSQL fork stored _logs ids as UUIDs. PocketBase assigns
// string ids to logs, so existing installations need this one-time conversion.
func init() {
	core.SystemMigrations.Register(func(txApp core.App) error {
		var dataType string
		err := txApp.AuxDB().NewQuery(`
			SELECT data_type
			FROM information_schema.columns
			WHERE table_schema = current_schema()
				AND table_name = '_logs'
				AND column_name = 'id'
		`).Row(&dataType)
		if err != nil {
			return fmt.Errorf("inspect _logs.id type: %w", err)
		}

		if dataType != "uuid" {
			return nil
		}

		_, err = txApp.AuxDB().NewQuery(`
			ALTER TABLE {{_logs}} ALTER COLUMN [[id]] DROP DEFAULT;
			ALTER TABLE {{_logs}} ALTER COLUMN [[id]] TYPE TEXT USING [[id]]::TEXT;
		`).Execute()
		if err != nil {
			return fmt.Errorf("migrate _logs.id to text: %w", err)
		}

		return nil
	}, func(txApp core.App) error {
		// New log ids are not UUIDs, so this migration cannot be safely reversed.
		return nil
	})
}
