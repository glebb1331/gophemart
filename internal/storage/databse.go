package storage

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql

var embedMigrations embed.FS

type DatabaseStorage struct {
	db *sql.DB
}

func NewDatabaseStorage(dsn string) (*DatabaseStorage, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return nil, err
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return nil, err
	}

	return &DatabaseStorage{db: db}, nil
}
