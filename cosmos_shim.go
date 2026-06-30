package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	_ "github.com/btnguyen2k/gocosmos"
)

var cosmosIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type cosmosShim struct {
	db         *sql.DB
	database   string
	container  string
	partition  string
	createDocs bool
}

func newCosmosShimFromEnv(defaultDatabase, defaultContainer, partition string) (*cosmosShim, bool, error) {
	endpoint := os.Getenv("COSMOS_ENDPOINT")
	key := os.Getenv("COSMOS_KEY")
	if endpoint == "" && key == "" {
		return nil, false, nil
	}
	if endpoint == "" || key == "" {
		return nil, true, errors.New("COSMOS_ENDPOINT and COSMOS_KEY must both be set")
	}
	database := os.Getenv("COSMOS_DATABASE")
	if database == "" {
		database = defaultDatabase
	}
	container := os.Getenv("COSMOS_CONTAINER")
	if container == "" {
		container = defaultContainer
	}
	shim, err := newCosmosShim(endpoint, key, database, container, partition)
	return shim, true, err
}

func newCosmosShim(endpoint, key, database, container, partition string) (*cosmosShim, error) {
	if !cosmosIdentifierPattern.MatchString(database) {
		return nil, fmt.Errorf("invalid COSMOS_DATABASE %q", database)
	}
	if !cosmosIdentifierPattern.MatchString(container) {
		return nil, fmt.Errorf("invalid COSMOS_CONTAINER %q", container)
	}
	dsn := fmt.Sprintf("AccountEndpoint=%s;AccountKey=%s;DefaultDb=%s;AutoId=false", endpoint, key, database)
	db, err := sql.Open("gocosmos", dsn)
	if err != nil {
		return nil, err
	}
	shim := &cosmosShim{db: db, database: database, container: container, partition: partition}
	if err := shim.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return shim, nil
}

func (s *cosmosShim) ensureSchema() error {
	if _, err := s.db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", s.database)); err != nil {
		return err
	}
	_, err := s.db.Exec(fmt.Sprintf("CREATE COLLECTION IF NOT EXISTS %s WITH PK=%s", s.container, s.partition))
	return err
}

func (s *cosmosShim) ready() error {
	return s.db.Ping()
}

func (s *cosmosShim) exec(query string, args ...any) error {
	_, err := s.db.Exec(query, args...)
	return err
}

func (s *cosmosShim) queryMaps(query string, args ...any) ([]map[string]any, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	results := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		scanValues := make([]any, len(cols))
		for i := range values {
			scanValues[i] = &values[i]
		}
		if err := rows.Scan(scanValues...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[strings.ToLower(col)] = values[i]
		}
		results = append(results, row)
	}
	return results, rows.Err()
}
