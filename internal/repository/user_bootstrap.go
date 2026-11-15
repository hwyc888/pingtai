package repository

import (
	"context"
	"errors"
	"time"
)

var ErrBootstrapComplete = errors.New("administrator account has already been created")

func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_user").Scan(&count)
	return count, err
}

// CreateFirstAdmin uses one conditional INSERT so concurrent first visits cannot
// claim the first-admin role with different usernames.
func (r *UserRepository) CreateFirstAdmin(ctx context.Context, username, passwordHash string) (int64, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO ai_user (username,password,usertype,status,deltag,created_at,updated_at)
		SELECT ?, ?, 1, 1, 0, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM ai_user)
	`, username, passwordHash, now, now)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		return 0, ErrBootstrapComplete
	}
	return result.LastInsertId()
}
