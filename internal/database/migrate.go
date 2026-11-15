package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func Migrate(ctx context.Context, db *sql.DB) error {
	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)

	for _, name := range files {
		content, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		for _, statement := range strings.Split(string(content), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
		}
	}
	return nil
}
