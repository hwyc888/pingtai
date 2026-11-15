package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"pingtai/internal/model"
)

type ProviderRepository struct {
	db *sql.DB
}

func NewProviderRepository(db *sql.DB) *ProviderRepository {
	return &ProviderRepository{db: db}
}

const providerColumns = "id,name,base_url,api_key,protocol,active,default_model,max_output_tokens,timeout_seconds,system_prompt,created_at,updated_at"

func scanProvider(row interface{ Scan(...any) error }) (model.Provider, error) {
	var item model.Provider
	var active int
	err := row.Scan(&item.ID, &item.Name, &item.BaseURL, &item.APIKey, &item.Protocol, &active,
		&item.DefaultModel, &item.MaxOutputTokens, &item.TimeoutSeconds, &item.SystemPrompt,
		&item.CreatedAt, &item.UpdatedAt)
	item.Active = active == 1
	return item, err
}

func (r *ProviderRepository) List(ctx context.Context) ([]model.Provider, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+providerColumns+" FROM ai_provider ORDER BY active DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.Provider, 0)
	for rows.Next() {
		item, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ProviderRepository) Get(ctx context.Context, id int64) (model.Provider, error) {
	return scanProvider(r.db.QueryRowContext(ctx, "SELECT "+providerColumns+" FROM ai_provider WHERE id=?", id))
}

func (r *ProviderRepository) Save(ctx context.Context, item model.Provider) (int64, error) {
	now := time.Now()
	item.Name = strings.TrimSpace(item.Name)
	item.BaseURL = strings.TrimSpace(item.BaseURL)
	item.Protocol = "openai"
	if item.TimeoutSeconds <= 0 {
		item.TimeoutSeconds = 180
	}
	if item.MaxOutputTokens < 0 {
		item.MaxOutputTokens = 0
	}
	if item.ID > 0 {
		result, err := r.db.ExecContext(ctx, "UPDATE ai_provider SET name=?,base_url=?,api_key=?,protocol=?,default_model=?,max_output_tokens=?,timeout_seconds=?,system_prompt=?,updated_at=? WHERE id=?",
			item.Name, item.BaseURL, item.APIKey, item.Protocol, item.DefaultModel, item.MaxOutputTokens, item.TimeoutSeconds, item.SystemPrompt, now, item.ID)
		if err != nil {
			return 0, err
		}
		if count, _ := result.RowsAffected(); count == 0 {
			return 0, sql.ErrNoRows
		}
		return item.ID, nil
	}
	result, err := r.db.ExecContext(ctx, "INSERT INTO ai_provider (name,base_url,api_key,protocol,active,default_model,max_output_tokens,timeout_seconds,system_prompt,created_at,updated_at) VALUES (?,?,?,'openai',0,?,?,?,?,?,?)",
		item.Name, item.BaseURL, item.APIKey, item.DefaultModel, item.MaxOutputTokens, item.TimeoutSeconds, item.SystemPrompt, now, now)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *ProviderRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM ai_provider WHERE id=?", id)
	return err
}

func (r *ProviderRepository) Activate(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM ai_provider WHERE id=?", id).Scan(&exists); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ai_provider SET active=0,updated_at=?", time.Now()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE ai_provider SET active=1,updated_at=? WHERE id=?", time.Now(), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ProviderRepository) Active(ctx context.Context) (model.Provider, []model.ProviderModel, error) {
	item, err := scanProvider(r.db.QueryRowContext(ctx, "SELECT "+providerColumns+" FROM ai_provider WHERE active=1 LIMIT 1"))
	if err != nil {
		return model.Provider{}, nil, err
	}
	models, err := r.Models(ctx, item.ID, true)
	return item, models, err
}

func (r *ProviderRepository) ReplaceModels(ctx context.Context, providerID int64, models []model.ProviderModel) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_provider_model WHERE provider_id=?", providerID); err != nil {
		return err
	}
	now := time.Now()
	for _, item := range models {
		if strings.TrimSpace(item.ModelID) == "" {
			continue
		}
		var contextLength, maxOutput any
		if item.ContextLength != nil {
			contextLength = *item.ContextLength
		}
		if item.MaxOutputTokens != nil {
			maxOutput = *item.MaxOutputTokens
		}
		enabled := 0
		if item.Enabled {
			enabled = 1
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO ai_provider_model (provider_id,model_id,context_length,max_output_tokens,enabled,discovered_at) VALUES (?,?,?,?,?,?)",
			providerID, item.ModelID, contextLength, maxOutput, enabled, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *ProviderRepository) Models(ctx context.Context, providerID int64, enabledOnly bool) ([]model.ProviderModel, error) {
	query := "SELECT id,provider_id,model_id,context_length,max_output_tokens,enabled,discovered_at FROM ai_provider_model WHERE provider_id=?"
	if enabledOnly {
		query += " AND enabled=1"
	}
	query += " ORDER BY model_id"
	rows, err := r.db.QueryContext(ctx, query, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.ProviderModel, 0)
	for rows.Next() {
		var item model.ProviderModel
		var contextLength, maxOutput sql.NullInt64
		var enabled int
		var discovered sql.NullTime
		if err := rows.Scan(&item.ID, &item.ProviderID, &item.ModelID, &contextLength, &maxOutput, &enabled, &discovered); err != nil {
			return nil, err
		}
		if contextLength.Valid {
			value := contextLength.Int64
			item.ContextLength = &value
		}
		if maxOutput.Valid {
			value := maxOutput.Int64
			item.MaxOutputTokens = &value
		}
		if discovered.Valid {
			item.DiscoveredAt = &discovered.Time
		}
		item.Enabled = enabled == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ProviderRepository) UpdateModel(ctx context.Context, providerID int64, modelID string, contextLength, maxOutput *int64, enabled bool) error {
	var contextValue, outputValue any
	if contextLength != nil {
		contextValue = *contextLength
	}
	if maxOutput != nil {
		outputValue = *maxOutput
	}
	enabledValue := 0
	if enabled {
		enabledValue = 1
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO ai_provider_model (provider_id,model_id,context_length,max_output_tokens,enabled,discovered_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(provider_id,model_id) DO UPDATE SET
			context_length=excluded.context_length,
			max_output_tokens=excluded.max_output_tokens,
			enabled=excluded.enabled,
			discovered_at=excluded.discovered_at
	`, providerID, modelID, contextValue, outputValue, enabledValue, time.Now())
	return err
}
