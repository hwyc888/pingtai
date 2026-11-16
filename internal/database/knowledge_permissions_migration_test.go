package database

import (
	"context"
	"testing"
)

func TestKnowledgePermissionMigrationKeepsExistingBasesPrivate(t *testing.T) {
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
		"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES ('legacy','旧知识库','',1,'alice','2026-01-01','2026-01-01')",
	); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	var owner, mode string
	if err := db.QueryRowContext(ctx,
		"SELECT owner_username,access_mode FROM ai_knowledge_base_policy WHERE base_id='legacy'",
	).Scan(&owner, &mode); err != nil {
		t.Fatal(err)
	}
	if owner != "alice" || mode != "private" {
		t.Fatalf("legacy knowledge base migrated as owner=%q mode=%q", owner, mode)
	}
}
