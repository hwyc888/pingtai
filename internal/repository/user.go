package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"pingtai/internal/model"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (model.User, error) {
	var user model.User
	var bindTime, createdAt, updatedAt sql.NullTime
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Password,
		&user.UserType,
		&user.Status,
		&user.DelTag,
		&user.OpenID,
		&bindTime,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.User{}, err
	}
	if bindTime.Valid {
		user.BindTime = &bindTime.Time
	}
	if createdAt.Valid {
		user.CreatedAt = &createdAt.Time
	}
	if updatedAt.Valid {
		user.UpdatedAt = &updatedAt.Time
	}
	return user, nil
}

const userColumns = "id, COALESCE(username,''), COALESCE(password,''), COALESCE(usertype,0), " +
	"COALESCE(status,0), COALESCE(deltag,0), COALESCE(openid,''), bind_time, created_at, updated_at"

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (model.User, error) {
	query := "SELECT " + userColumns + " FROM ai_user WHERE username=? LIMIT 1"
	return scanUser(r.db.QueryRowContext(ctx, query, username))
}

func (r *UserRepository) FindByID(ctx context.Context, id int64) (model.User, error) {
	query := "SELECT " + userColumns + " FROM ai_user WHERE id=? LIMIT 1"
	return scanUser(r.db.QueryRowContext(ctx, query, id))
}

func (r *UserRepository) Create(ctx context.Context, username, password string, userType int) (int64, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, "INSERT INTO ai_user "+
		"(username,password,usertype,status,deltag,created_at,updated_at) "+
		"VALUES (?,?,?,?,0,?,?)", username, password, userType, 1, now, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id int64, password string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE ai_user SET password=?, updated_at=? WHERE id=?", password, time.Now(), id)
	return err
}

func (r *UserRepository) SoftDelete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE ai_user SET deltag=1, updated_at=? WHERE id=?", time.Now(), id)
	return err
}

func (r *UserRepository) List(ctx context.Context, page, pageSize int) ([]model.User, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_user WHERE COALESCE(deltag,0)=0").Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	query := "SELECT " + userColumns + " FROM ai_user WHERE COALESCE(deltag,0)=0 " +
		"ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := r.db.QueryContext(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := make([]model.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}
	return users, total, rows.Err()
}

func (r *UserRepository) CreateOrUpdateAdmin(ctx context.Context, username, password string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM ai_user WHERE username=? LIMIT 1 FOR UPDATE", username).Scan(&id)
	now := time.Now()
	switch {
	case err == nil:
		_, err = tx.ExecContext(ctx, "UPDATE ai_user SET password=?,usertype=1,status=1,deltag=0,updated_at=? WHERE id=?", password, now, id)
	case errors.Is(err, sql.ErrNoRows):
		var admins int
		if countErr := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_user WHERE usertype=1 AND COALESCE(deltag,0)=0").Scan(&admins); countErr != nil {
			return countErr
		}
		if admins > 0 {
			return errors.New("another administrator already exists; refusing to create a second bootstrap administrator")
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO ai_user "+
			"(username,password,usertype,status,deltag,created_at,updated_at) "+
			"VALUES (?,?,1,1,0,?,?)", username, password, now, now)
	default:
		return err
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
