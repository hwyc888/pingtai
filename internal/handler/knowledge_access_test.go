package handler

import (
	"context"
	"testing"

	"pingtai/internal/config"
	"pingtai/internal/database"
)

func TestKnowledgeAccessDecision(t *testing.T) {
	tests := []struct {
		name, owner, mode, username string
		member, admin bool
		wantUse, wantManage bool
	}{
		{"private owner", "alice", "private", "alice", false, false, true, true},
		{"private other user", "alice", "private", "bob", false, false, false, false},
		{"shared any user", "alice", "shared", "bob", false, false, true, false},
		{"assigned member", "alice", "assigned", "bob", true, false, true, false},
		{"assigned non-member", "alice", "assigned", "carol", false, false, false, false},
		{"admin manages private", "alice", "private", "root", false, true, true, true},
		{"owner comparison ignores case", "Alice", "private", "alice", false, false, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			use, manage := knowledgeAccessDecision(test.owner, test.mode, test.username, test.member, test.admin)
			if use != test.wantUse || manage != test.wantManage {
				t.Fatalf("got use=%v manage=%v; want use=%v manage=%v", use, manage, test.wantUse, test.wantManage)
			}
		})
	}
}

func TestKnowledgeAccessQueriesSQLitePolicy(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES ('private','Private','',1,'alice','now','now')",
		"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES ('shared','Shared','',1,'alice','now','now')",
		"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES ('assigned','Assigned','',1,'alice','now','now')",
		"INSERT INTO ai_knowledge_base_policy VALUES ('private','alice','private')",
		"INSERT INTO ai_knowledge_base_policy VALUES ('shared','alice','shared')",
		"INSERT INTO ai_knowledge_base_policy VALUES ('assigned','alice','assigned')",
		"INSERT INTO ai_knowledge_base_member VALUES ('assigned','bob')",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{AuthMode: "local"}
	h := &Handler{Config: &cfg, DB: db}
	tests := []struct {
		baseID, username string
		wantUse, wantManage bool
	}{
		{"private","alice",true,true},
		{"private","bob",false,false},
		{"shared","bob",true,false},
		{"assigned","bob",true,false},
		{"assigned","carol",false,false},
	}
	for _, test := range tests {
		use, manage, err := h.knowledgeAccess(ctx,test.username,test.baseID)
		if err != nil {
			t.Fatalf("%s/%s: %v",test.baseID,test.username,err)
		}
		if use!=test.wantUse || manage!=test.wantManage {
			t.Fatalf("%s/%s: got use=%v manage=%v; want use=%v manage=%v",
				test.baseID,test.username,use,manage,test.wantUse,test.wantManage)
		}
	}
}
