package core

import (
	"fmt"
	"net/url"
	"regexp"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pocketbase/dbx"
)

// Sample Connection String: "postgres://<username>:<password>@127.0.0.1:<port>"
func PostgresDBConnectFunc(connectionString string) DBConnectFunc {
	url, err := url.Parse(connectionString)
	if err != nil {
		panic(fmt.Errorf("invalid connection string: %s", err))
	}
	if url.Scheme != "postgres" && url.Scheme != "postgresql" {
		panic(fmt.Errorf("invalid connection string scheme: [%s], must be [postgres] or [postgresql]", url.Scheme))
	}

	return func(dbName string) (*dbx.DB, error) {
		fmt.Println("Connecting to DB:", dbName)
		dbURL, err := postgresDatabaseURL(url.String(), dbName)
		if err != nil {
			return nil, err
		}
		db, err := dbx.MustOpen("pgx", dbURL)
		if err != nil && regexp.MustCompile(`database ".+" does not exist`).MatchString(err.Error()) {
			fmt.Println("Database not found, creating:", dbName)
			if err := createDatabase(connectionString, dbName); err != nil {
				return nil, fmt.Errorf("Failed to create database [%s]: %s, please create it manually", dbName, err)
			}
			fmt.Println("Database created, reconnecting:", dbName)
			db, err = dbx.MustOpen("pgx", dbURL)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Postgres: %s", err)
		}

		return db, nil
	}
}

// postgresDatabaseURL returns connectionString targeting database.
func postgresDatabaseURL(connectionString, database string) (string, error) {
	u, err := url.Parse(connectionString)
	if err != nil {
		return "", fmt.Errorf("invalid connection string: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("invalid connection string scheme: %q", u.Scheme)
	}

	u.Path = database
	return u.String(), nil
}

func createDatabase(connectionString string, dbName string) error {
	initDB, err := dbx.MustOpen("pgx", connectionString)
	if err != nil {
		return err
	}
	_, err = initDB.NewQuery(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)).Execute()
	if err != nil {
		return err
	}
	return nil
}
