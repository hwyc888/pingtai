package repository

import (
	"context"
	"fmt"
	"time"

	"pingtai/internal/model"
	"pingtai/internal/util"
)

func (r *UserRepository) EnsureLocalIdentity(ctx context.Context, user model.User) (model.Identity, error) {
	if user.ID <= 0 || user.Username == "" {
		return model.Identity{}, fmt.Errorf("a persisted local user is required")
	}
	id := fmt.Sprintf("local-%d", user.ID)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO ai_identity (id,provider,subject,username,display_name,local_user_id,created_at,updated_at) "+
			"VALUES (?,'local',?,?,?,?,?,?)",
		id, fmt.Sprintf("user:%d", user.ID), user.Username, user.Username, user.ID, now, now,
	)
	if err != nil {
		return model.Identity{}, err
	}
	if _, err := r.db.ExecContext(ctx,
		"UPDATE ai_identity SET username=?,display_name=?,updated_at=? WHERE provider='local' AND local_user_id=?",
		user.Username, user.Username, now, user.ID,
	); err != nil {
		return model.Identity{}, err
	}
	return scanIdentity(r.db.QueryRowContext(ctx,
		"SELECT id,provider,subject,username,display_name,COALESCE(local_user_id,0) "+
			"FROM ai_identity WHERE provider='local' AND local_user_id=? LIMIT 1", user.ID,
	))
}

func (r *UserRepository) EnsureExternalIdentity(ctx context.Context, provider, subject, username, displayName string) (model.Identity, error) {
	if provider != "sso" || subject == "" || username == "" {
		return model.Identity{}, fmt.Errorf("a valid external identity is required")
	}
	if displayName == "" {
		displayName = username
	}
	id, err := util.NewID()
	if err != nil {
		return model.Identity{}, err
	}
	id = provider + "-" + id
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = r.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO ai_identity (id,provider,subject,username,display_name,created_at,updated_at) "+
			"VALUES (?,?,?,?,?,?,?)",
		id, provider, subject, username, displayName, now, now,
	)
	if err != nil {
		return model.Identity{}, err
	}
	if _, err := r.db.ExecContext(ctx,
		"UPDATE ai_identity SET username=?,display_name=?,updated_at=? WHERE provider=? AND subject=?",
		username, displayName, now, provider, subject,
	); err != nil {
		return model.Identity{}, err
	}
	return scanIdentity(r.db.QueryRowContext(ctx,
		"SELECT id,provider,subject,username,display_name,COALESCE(local_user_id,0) "+
			"FROM ai_identity WHERE provider=? AND subject=? LIMIT 1", provider, subject,
	))
}

func (r *UserRepository) FindIdentityByUsername(ctx context.Context, provider, username string) (model.Identity, error) {
	return scanIdentity(r.db.QueryRowContext(ctx,
		"SELECT id,provider,subject,username,display_name,COALESCE(local_user_id,0) "+
			"FROM ai_identity WHERE provider=? AND username=? COLLATE NOCASE LIMIT 1",
		provider, username,
	))
}

func scanIdentity(row interface{ Scan(...any) error }) (model.Identity, error) {
	var item model.Identity
	err := row.Scan(&item.ID, &item.Provider, &item.Subject, &item.Username, &item.DisplayName, &item.LocalUserID)
	if err != nil {
		return model.Identity{}, err
	}
	return item, nil
}

