package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pingtai/internal/model"
	"pingtai/internal/repository"
	"pingtai/internal/service/openai"
)

func (h *Handler) providerRepository() *repository.ProviderRepository {
	return repository.NewProviderRepository(h.DB)
}

func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) {
	repo := h.providerRepository()
	items, err := repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取供应商列表失败")
		return
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		models, _ := repo.Models(r.Context(), item.ID, false)
		result = append(result, providerResponse(item, models))
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": result})
}

func providerResponse(item model.Provider, models []model.ProviderModel) map[string]any {
	available := make([]map[string]any, 0, len(models))
	for _, entry := range models {
		available = append(available, map[string]any{
			"model_id": entry.ModelID,
			"context_length": entry.ContextLength,
			"max_output_tokens": entry.MaxOutputTokens,
			"enabled": entry.Enabled,
		})
	}
	return map[string]any{
		"id": item.ID, "name": item.Name, "base_url": item.BaseURL,
		"has_api_key": item.APIKey != "", "protocol": item.Protocol,
		"active": item.Active, "default_model": item.DefaultModel,
		"max_output_tokens": item.MaxOutputTokens, "timeout_seconds": item.TimeoutSeconds,
		"system_prompt": item.SystemPrompt, "models": available,
	}
}

func (h *Handler) SaveProvider(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	item := model.Provider{
		ID: int64(intValue(payload["id"], 0)),
		Name: strings.TrimSpace(stringValue(payload["name"])),
		BaseURL: strings.TrimSpace(stringValue(payload["base_url"])),
		APIKey: strings.TrimSpace(stringValue(payload["api_key"])),
		Protocol: "openai",
		DefaultModel: strings.TrimSpace(stringValue(payload["default_model"])),
		MaxOutputTokens: intValue(payload["max_output_tokens"], 0),
		TimeoutSeconds: intValue(payload["timeout_seconds"], 180),
		SystemPrompt: stringValue(payload["system_prompt"]),
	}
	if item.Name == "" || item.BaseURL == "" || item.MaxOutputTokens < 0 || item.TimeoutSeconds < 1 || item.TimeoutSeconds > 3600 {
		writeError(w, http.StatusBadRequest, "供应商名称、Base URL、超时或输出 Token 参数无效")
		return
	}
	if _, err := openai.Endpoint(item.BaseURL, "chat/completions"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if item.ID > 0 {
		existing, err := h.providerRepository().Get(r.Context(), item.ID)
		if err != nil {
			writeError(w, http.StatusNotFound, "供应商不存在")
			return
		}
		if item.APIKey == "" {
			item.APIKey = existing.APIKey
		}
	}
	id, err := h.providerRepository().Save(r.Context(), item)
	if err != nil {
		writeError(w, http.StatusBadRequest, "保存供应商失败，名称可能已存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{"id": id}})
}

func (h *Handler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	id := int64(intValue(payload["id"], 0))
	if id <= 0 {
		writeError(w, http.StatusBadRequest, "供应商 ID 无效")
		return
	}
	if err := h.providerRepository().Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "删除供应商失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "deleted"})
}

func (h *Handler) ActivateProvider(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	id := int64(intValue(payload["id"], 0))
	item, err := h.providerRepository().Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "供应商不存在")
		return
	}
	models, err := h.providerRepository().Models(r.Context(), id, true)
	if err != nil || len(models) == 0 {
		writeError(w, http.StatusBadRequest, "请先获取模型并至少启用一个模型")
		return
	}
	defaultFound := false
	for _, entry := range models {
		if entry.ModelID == item.DefaultModel {
			defaultFound = true
		}
	}
	if !defaultFound {
		writeError(w, http.StatusBadRequest, "默认模型必须是已启用的模型")
		return
	}
	if err := h.providerRepository().Activate(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "启用供应商失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "restart_required"})
}

func (h *Handler) DiscoverProviderModels(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	id := int64(intValue(payload["id"], 0))
	item, err := h.providerRepository().Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "供应商不存在")
		return
	}
	cfg := *h.Config
	cfg.OpenAIBaseURL = item.BaseURL
	cfg.OpenAIAPIKey = item.APIKey
	cfg.OpenAITimeout = time.Duration(item.TimeoutSeconds) * time.Second
	client := openai.NewClient(cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	response, err := client.Models(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "连接上游模型列表失败")
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		writeError(w, http.StatusBadGateway, "上游模型列表读取失败或超过 4 MB")
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "上游 /models 返回 HTTP "+strconv.Itoa(response.StatusCode))
		return
	}
	models, err := parseProviderModels(body, id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if len(models) == 0 {
		writeError(w, http.StatusBadGateway, "上游未返回可识别的模型 ID")
		return
	}
	if err := h.providerRepository().ReplaceModels(r.Context(), id, models); err != nil {
		writeError(w, http.StatusInternalServerError, "保存模型列表失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": map[string]any{
		"models": providerModelResponses(models), "total": len(models),
	}})
}

func parseProviderModels(body []byte, providerID int64) ([]model.ProviderModel, error) {
	var payload struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("上游模型列表不是有效的 OpenAI 兼容 JSON")
	}
	items := make([]model.ProviderModel, 0, len(payload.Data))
	seen := make(map[string]bool)
	for _, raw := range payload.Data {
		var id string
		if err := json.Unmarshal(raw["id"], &id); err != nil {
			continue
		}
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		items = append(items, model.ProviderModel{
			ProviderID: providerID, ModelID: id, Enabled: true,
			ContextLength: firstInt64(raw, "context_length", "context_window", "max_model_len", "max_input_tokens", "context_length_tokens"),
			MaxOutputTokens: firstInt64(raw, "max_output_tokens", "max_completion_tokens", "max_tokens"),
		})
	}
	return items, nil
}

func firstInt64(raw map[string]json.RawMessage, keys ...string) *int64 {
	for _, key := range keys {
		value, exists := raw[key]
		if !exists || string(value) == "null" {
			continue
		}
		var number int64
		if json.Unmarshal(value, &number) == nil && number > 0 {
			return &number
		}
		var text string
		if json.Unmarshal(value, &text) == nil {
			if parsed, err := strconv.ParseInt(text, 10, 64); err == nil && parsed > 0 {
				return &parsed
			}
		}
	}
	return nil
}

func providerModelResponses(items []model.ProviderModel) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"model_id": item.ModelID,
			"context_length": item.ContextLength,
			"max_output_tokens": item.MaxOutputTokens,
			"enabled": item.Enabled,
		})
	}
	return result
}

func (h *Handler) UpdateProviderModel(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	providerID := int64(intValue(payload["provider_id"], 0))
	modelID := strings.TrimSpace(stringValue(payload["model_id"]))
	contextLength := optionalPositiveInt64(payload["context_length"])
	maxOutput := optionalPositiveInt64(payload["max_output_tokens"])
	enabled := true
	if value, exists := payload["enabled"]; exists {
		enabled = value == true
	}
	if providerID <= 0 || modelID == "" {
		writeError(w, http.StatusBadRequest, "供应商 ID 或模型 ID 无效")
		return
	}
	if err := h.providerRepository().UpdateModel(r.Context(), providerID, modelID, contextLength, maxOutput, enabled); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "updated"})
}

func optionalPositiveInt64(value any) *int64 {
	switch number := value.(type) {
	case float64:
		if number > 0 {
			parsed := int64(number)
			return &parsed
		}
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(number), 10, 64); err == nil && parsed > 0 {
			return &parsed
		}
	}
	return nil
}
