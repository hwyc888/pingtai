package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationsDoNotDropExistingTables(t *testing.T) {
	files, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected embedded migrations")
	}
	combined := strings.Builder{}
	for _, name := range files {
		content, err := migrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		upper := strings.ToUpper(string(content))
		if strings.Contains(upper, "DROP TABLE") || strings.Contains(upper, "TRUNCATE TABLE") {
			t.Fatalf("destructive SQL found in %s", name)
		}
		combined.Write(content)
	}
	if !strings.Contains(combined.String(), "CREATE TABLE IF NOT EXISTS ai_user") {
		t.Fatal("expected safe ai_user create-if-missing migration")
	}
}

func TestSQLiteMigrationsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data", "pingtai.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second migration should be idempotent: %v", err)
	}
	var tableCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('ai_user','ai_conversation','ai_chat','ai_message','ai_message_context','ai_provider','ai_provider_model')").Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 7 {
		t.Fatalf("created %d expected application tables", tableCount)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO ai_user (username,password,usertype,status,deltag) VALUES (?,?,?,?,0)", "admin", "hash", 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("migration removed or damaged existing data: %v", err)
	}
	var username string
	if err := db.QueryRowContext(ctx, "SELECT username FROM ai_user WHERE id=1").Scan(&username); err != nil || username != "admin" {
		t.Fatalf("existing SQLite data was not preserved: username=%q err=%v", username, err)
	}
}
