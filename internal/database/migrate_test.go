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


func TestStableIdentityMigrationBackfillsExistingChatAndKnowledgeOwnership(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO ai_user (username,password,usertype,status,deltag) VALUES ('alice','hash',0,1,0)",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO ai_conversation (id,user_id,bot_id,created_at,updated_at) VALUES ('legacy-chat','alice','bot','now','now')",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES ('legacy-kb','test','',1,'alice','now','now')",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO ai_knowledge_base_policy (base_id,owner_username,access_mode) VALUES ('legacy-kb','alice','private')",
	); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("identity backfill migration failed: %v", err)
	}
	var localID, conversationOwner, knowledgeOwner string
	if err := db.QueryRowContext(ctx, "SELECT id FROM ai_identity WHERE provider='local' AND username='alice'").Scan(&localID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT user_id FROM ai_conversation WHERE id='legacy-chat'").Scan(&conversationOwner); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT owner_user_id FROM ai_knowledge_base_identity WHERE base_id='legacy-kb'").Scan(&knowledgeOwner); err != nil {
		t.Fatal(err)
	}
	if localID != "local-1" || conversationOwner != localID || knowledgeOwner != localID {
		t.Fatalf("legacy ownership was not migrated to stable ID: identity=%q conversation=%q knowledge=%q",
			localID, conversationOwner, knowledgeOwner)
	}
}
