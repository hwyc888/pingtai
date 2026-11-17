package handler

import (
	"context"
	"database/sql"
	"strings"

	"pingtai/internal/middleware"
	"pingtai/internal/model"
)

type knowledgeDBTX interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func knowledgeAccessDecision(ownerUserID, mode, userID string, member, admin bool) (bool, bool) {
	isOwner := strings.TrimSpace(ownerUserID) != "" && strings.EqualFold(strings.TrimSpace(ownerUserID), strings.TrimSpace(userID))
	canManage := admin || isOwner
	canUse := canManage || mode == "shared" || (mode == "assigned" && member)
	return canUse, canManage
}

func (h *Handler) knowledgeAccess(ctx context.Context, identity middleware.Identity, baseID string) (bool, bool, error) {
	var ownerUserID, mode string
	var member int
	err := h.DB.QueryRowContext(ctx,
		"SELECT COALESCE(i.owner_user_id,''),COALESCE(i.access_mode,p.access_mode,'private'),"+
			"CASE WHEN m.user_id IS NULL THEN 0 ELSE 1 END "+
			"FROM ai_knowledge_base b "+
			"LEFT JOIN ai_knowledge_base_identity i ON i.base_id=b.id "+
			"LEFT JOIN ai_knowledge_base_policy p ON p.base_id=b.id "+
			"LEFT JOIN ai_knowledge_base_identity_member m ON m.base_id=b.id AND m.user_id=? "+
			"WHERE b.id=?",
		identity.UserID, baseID,
	).Scan(&ownerUserID, &mode, &member)
	if err != nil {
		return false, false, err
	}
	admin := h.isKnowledgeAdmin(ctx, identity)
	canUse, canManage := knowledgeAccessDecision(ownerUserID, mode, identity.UserID, member == 1, admin)
	return canUse, canManage, nil
}

func (h *Handler) resolveKnowledgeIdentity(ctx context.Context, username, provider string) (model.Identity, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return model.Identity{}, sql.ErrNoRows
	}
	if provider == "" {
		provider = "local"
	}
	if provider == "sso" {
		return h.Users.EnsureExternalIdentity(ctx, "sso", username, username, username)
	}
	user, err := h.Users.FindByUsername(ctx, username)
	if err != nil {
		return model.Identity{}, err
	}
	return h.Users.EnsureLocalIdentity(ctx, user)
}

func (h *Handler) resolveKnowledgeMemberIdentities(ctx context.Context, members []string) ([]model.Identity, error) {
	identities := make([]model.Identity, 0, len(members))
	for _, member := range members {
		provider := "local"
		username := strings.TrimSpace(member)
		if strings.HasPrefix(strings.ToLower(username), "sso:") {
			provider = "sso"
			username = strings.TrimSpace(username[4:])
		}
		identity, err := h.resolveKnowledgeIdentity(ctx, username, provider)
		if err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return identities, nil
}

func saveKnowledgeIdentityMembers(ctx context.Context, tx knowledgeDBTX, baseID string, members []model.Identity) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_knowledge_base_identity_member WHERE base_id=?", baseID); err != nil {
		return err
	}
	for _, member := range members {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO ai_knowledge_base_identity_member (base_id,user_id) VALUES (?,?)",
			baseID, member.ID,
		); err != nil {
			return err
		}
	}
	return nil
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
		if username == "" || len([]rune(username)) > 105 {
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
