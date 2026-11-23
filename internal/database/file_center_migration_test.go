package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPlatformFileMigrationCreatesSafeStorageMetadata(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "pingtai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{
		"id", "storage_key", "original_name", "content_type", "extension",
		"size_bytes", "sha256", "extracted_text", "extraction_status",
		"owner_user_id", "owner_username", "owner_provider", "access_mode",
	} {
		var count int
		if err := db.QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM pragma_table_info('platform_file') WHERE name=?", column,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected platform_file.%s column", column)
		}
	}
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO platform_file (id,storage_key,original_name,sha256,owner_user_id,owner_username,created_at,updated_at) VALUES ('one','one.txt','one.txt','hash','user-1','alice','now','now')",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(),
		"INSERT INTO platform_file (id,storage_key,original_name,sha256,owner_user_id,owner_username,created_at,updated_at) VALUES ('two','two.txt','two.txt','hash','user-1','alice','now','now')",
	); err == nil {
		t.Fatal("duplicate content for one owner should be rejected by the unique index")
	}
}
