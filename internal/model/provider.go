package model

import "time"

type Provider struct {
	ID              int64
	Name            string
	BaseURL         string
	APIKey          string
	Protocol        string
	Active          bool
	DefaultModel    string
	MaxOutputTokens int
	TimeoutSeconds  int
	SystemPrompt    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ProviderModel struct {
	ID              int64
	ProviderID      int64
	ModelID         string
	ContextLength   *int64
	MaxOutputTokens *int64
	Enabled         bool
	DiscoveredAt    *time.Time
}
