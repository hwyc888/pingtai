package handler

import (
	"context"
	"database/sql"
	"strings"
)

type knowledgeDBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func knowledgeAccessDecision(owner, mode, username string, member, admin bool) (bool, bool) {
	isOwner := strings.EqualFold(strings.TrimSpace(owner), strings.TrimSpace(username))
	canManage := admin || isOwner
	canUse := canManage || mode == "shared" || (mode == "assigned" && member)
	return canUse, canManage
}

func (h *Handler) knowledgeAccess(ctx context.Context, username, baseID string) (bool, bool, error) {
	var owner, mode string
	var member int
	err := h.DB.QueryRowContext(ctx,
		"SELECT COALESCE(p.owner_username,b.created_by),COALESCE(p.access_mode,'private'),"+
			"CASE WHEN m.username IS NULL THEN 0 ELSE 1 END "+
			"FROM ai_knowledge_base b "+
			"LEFT JOIN ai_knowledge_base_policy p ON p.base_id=b.id "+
			"LEFT JOIN ai_knowledge_base_member m ON m.base_id=b.id AND m.username=? COLLATE NOCASE "+
			"WHERE b.id=?",
		username, baseID,
	).Scan(&owner, &mode, &member)
	if err != nil {
		return false, false, err
	}
	return knowledgeAccessDecision(owner, mode, username, member == 1, h.isKnowledgeAdmin(ctx, username)), nil
}

func parseKnowledgeMembers(value any) []string {
	var values []string
	switch item := value.(type) {
	case string:
		values = strings.FieldsFunc(item, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '，'
		})
	case []any:
		for _, entry := range item {
			if username, ok := entry.(string); ok {
				values = append(values, username)
			}
		}
	case []string:
		values = item
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		username := strings.TrimSpace(value)
		if username == "" || len([]rune(username)) > 100 {
			continue
		}
		key := strings.ToLower(username)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, username)
	}
	return result
}

func saveKnowledgeMembers(ctx context.Context, tx knowledgeDBTX, baseID string, members []string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_knowledge_base_member WHERE base_id=?", baseID); err != nil {
		return err
	}
	for _, username := range members {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO ai_knowledge_base_member (base_id,username) VALUES (?,?)",
			baseID, username,
		); err != nil {
			return err
		}
	}
	return nil
}

func knowledgeMemberNamesTx(ctx context.Context, tx *sql.Tx, baseID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT username FROM ai_knowledge_base_member WHERE base_id=? ORDER BY username", baseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]string, 0)
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, err
		}
		members = append(members, username)
	}
	return members, rows.Err()
}
