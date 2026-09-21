package core

import (
	"context"
	"fmt"
	"os/exec"
)

const (
	postgresDataDumpFilename = "data.dump"
	postgresAuxDumpFilename  = "auxiliary.dump"
)

// dumpPostgresDatabase writes a portable PostgreSQL custom-format dump.
//
// pg_dump provides a transactionally consistent snapshot for each database.
// The data and auxiliary databases are independent, so they are dumped
// separately and restored separately during a controlled recovery.
func dumpPostgresDatabase(ctx context.Context, app App, database, destination string) error {
	connectionURL, err := postgresDatabaseURL(app.PostgresURL(), database)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(
		ctx,
		"pg_dump",
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--file="+destination,
		connectionURL,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pg_dump %q failed: %w: %s", database, err, output)
	}

	return nil
}
