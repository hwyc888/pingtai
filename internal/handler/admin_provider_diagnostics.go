package handler

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/model"
	"pingtai/internal/service/openai"
)

func providerOpenAIClient(base config.Config, item model.Provider) *openai.Client {
	base.OpenAIBaseURL = item.BaseURL
	base.OpenAIAPIKey = item.APIKey
	base.OpenAITimeout = time.Duration(item.TimeoutSeconds) * time.Second
	return openai.NewClient(base)
}

func (h *Handler) TestProviderConnection(w http.ResponseWriter, r *http.Request) {
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
	client := providerOpenAIClient(*h.Config, item)
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	started := time.Now()
	response, err := client.Models(ctx)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		writeError(w, http.StatusBadGateway, "连接上游失败，请检查 Base URL、网络和 API Key")
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
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"reachable": true,
			"status_code": response.StatusCode,
			"latency_ms": latency,
			"model_count": len(models),
		},
	})
}

func (h *Handler) TestProviderModel(w http.ResponseWriter, r *http.Request) {
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
	modelID := strings.TrimSpace(stringValue(payload["model_id"]))
	if modelID == "" {
		modelID = item.DefaultModel
	}
	models, err := h.providerRepository().Models(r.Context(), id, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取供应商模型失败")
		return
	}
	enabled := false
	for _, entry := range models {
		if entry.ModelID == modelID {
			enabled = true
			break
		}
	}
	if !enabled {
		writeError(w, http.StatusBadRequest, "请选择一个已启用的模型")
		return
	}

	client := providerOpenAIClient(*h.Config, item)
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	started := time.Now()
	body, status, err := client.Complete(ctx, map[string]any{
		"model": modelID,
		"messages": []map[string]any{{
			"role": "user", "content": "Reply with OK.",
		}},
		"max_tokens": 8,
		"stream": false,
	})
	latency := time.Since(started).Milliseconds()
	if err != nil {
		writeError(w, http.StatusBadGateway, "模型请求失败，请检查模型 ID 和上游接口配置")
		return
	}
	if status < 200 || status >= 300 {
		writeError(w, http.StatusBadGateway, "上游聊天接口返回 HTTP "+strconv.Itoa(status))
		return
	}
	answer, _, err := parseModelAnswer(body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "上游未返回有效的 Chat Completions 文本响应")
		return
	}
	preview := strings.TrimSpace(answer)
	if runes := []rune(preview); len(runes) > 80 {
		preview = string(runes[:80]) + "…"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"model_id": modelID,
			"status_code": status,
			"latency_ms": latency,
			"response_preview": preview,
		},
	})
}
