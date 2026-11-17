package handler

import (
	"context"
	"testing"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/middleware"
)

func TestKnowledgeAccessDecision(t *testing.T) {
	tests := []struct {
		name, owner, mode, userID string
		member, admin bool
		wantUse, wantManage bool
	}{
		{"private owner", "uid-alice", "private", "uid-alice", false, false, true, true},
		{"private other user", "uid-alice", "private", "uid-bob", false, false, false, false},
		{"shared any user", "uid-alice", "shared", "uid-bob", false, false, true, false},
		{"assigned member", "uid-alice", "assigned", "uid-bob", true, false, true, false},
		{"assigned non-member", "uid-alice", "assigned", "uid-carol", false, false, false, false},
		{"admin manages private", "uid-alice", "private", "uid-root", false, true, true, true},
		{"owner comparison ignores case", "UID-Alice", "private", "uid-alice", false, false, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			use, manage := knowledgeAccessDecision(test.owner, test.mode, test.userID, test.member, test.admin)
			if use != test.wantUse || manage != test.wantManage {
				t.Fatalf("got use=%v manage=%v; want use=%v manage=%v", use, manage, test.wantUse, test.wantManage)
			}
		})
	}
}

func TestKnowledgeAccessQueriesIdentityPolicy(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	now := "now"
	for _, item := range []struct{ id, username string }{
		{"uid-alice", "alice"}, {"uid-bob", "bob"}, {"uid-carol", "carol"},
	} {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO ai_identity (id,provider,subject,username,display_name,created_at,updated_at) VALUES (?,'local',?,?,?, ?,?)",
			item.id, "user:"+item.username, item.username, item.username, now, now,
		); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct{ id, name, mode string }{
		{"private", "Private", "private"},
		{"shared", "Shared", "shared"},
		{"assigned", "Assigned", "assigned"},
	} {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES (?,?, '',1,'alice','now','now')",
			item.id, item.name,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO ai_knowledge_base_policy (base_id,owner_username,access_mode) VALUES (?,?,?)",
			item.id, "alice", item.mode,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO ai_knowledge_base_identity (base_id,owner_user_id,access_mode) VALUES (?,?,?)",
			item.id, "uid-alice", item.mode,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO ai_knowledge_base_member VALUES ('assigned','bob')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO ai_knowledge_base_identity_member VALUES ('assigned','uid-bob')"); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{AuthMode: "local"}
	h := &Handler{Config: &cfg, DB: db}
	tests := []struct {
		baseID string
		identity middleware.Identity
		wantUse, wantManage bool
	}{
		{"private", middleware.Identity{Username:"alice",UserID:"uid-alice",Provider:"local"}, true, true},
		{"private", middleware.Identity{Username:"bob",UserID:"uid-bob",Provider:"local"}, false, false},
		{"shared", middleware.Identity{Username:"bob",UserID:"uid-bob",Provider:"local"}, true, false},
		{"assigned", middleware.Identity{Username:"bob",UserID:"uid-bob",Provider:"local"}, true, false},
		{"assigned", middleware.Identity{Username:"carol",UserID:"uid-carol",Provider:"local"}, false, false},
	}
	for _, test := range tests {
		use, manage, err := h.knowledgeAccess(ctx, test.identity, test.baseID)
		if err != nil {
			t.Fatalf("%s/%s: %v", test.baseID, test.identity.Username, err)
		}
		if use != test.wantUse || manage != test.wantManage {
			t.Fatalf("%s/%s: got use=%v manage=%v; want use=%v manage=%v",
				test.baseID, test.identity.Username, use, manage, test.wantUse, test.wantManage)
		}
	}
}
