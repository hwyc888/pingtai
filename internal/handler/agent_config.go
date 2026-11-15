package handler

import (
	"net/http"
	"net/url"
	"strings"
)

func (h *Handler) GetAgentConfig(w http.ResponseWriter, r *http.Request) {
	botID := strings.TrimSpace(r.URL.Query().Get("bot_id"))
	if botID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少智能体ID参数"})
		return
	}
	query := url.Values{"bot_id": []string{botID}}
	h.callCozeJSON(w, r, http.MethodGet, "/v1/bot/get_online_info", query, nil, nil)
}

func (h *Handler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) {
		return
	}
	botID := strings.TrimSpace(r.FormValue("bot_id"))
	if botID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少智能体ID参数"})
		return
	}
	payload := map[string]any{
		"bot_id":      botID,
		"name":        r.FormValue("name"),
		"description": r.FormValue("description"),
	}
	prologue := r.FormValue("prologue")
	questions := r.Form["suggested_questions"]
	if prologue != "" || (len(questions) > 0 && questions[0] != "") {
		onboarding := make(map[string]any)
		if prologue != "" {
			onboarding["prologue"] = prologue
		}
		if len(questions) > 0 && questions[0] != "" {
			onboarding["suggested_questions"] = questions
		}
		payload["onboarding_info"] = onboarding
	}
	datasetIDs := r.Form["dataset_ids"]
	if len(datasetIDs) > 0 && datasetIDs[0] != "" {
		payload["knowledge"] = map[string]any{"dataset_ids": datasetIDs}
	}
	if prompt := r.FormValue("prompt"); prompt != "" {
		payload["prompt_info"] = map[string]any{"prompt": prompt}
	}
	file, header, err := r.FormFile("icon")
	if err == nil {
		defer file.Close()
		fileID, uploadErr := h.uploadCozeFile(r.Context(), file, header)
		if uploadErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "上传图标失败: " + uploadErr.Error()})
			return
		}
		payload["icon_file_id"] = fileID
	}
	h.callCozeJSON(w, r, http.MethodPost, "/v1/bot/update", nil, payload, nil)
}

func (h *Handler) PublishAgent(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	botID := strings.TrimSpace(stringValue(payload["bot_id"]))
	if botID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少智能体ID参数"})
		return
	}
	h.callCozeJSON(w, r, http.MethodPost, "/v1/bot/publish", nil, map[string]any{
		"bot_id": botID, "connector_ids": []string{"1024", "999"},
	}, nil)
}

func (h *Handler) GetAgentsList(w http.ResponseWriter, r *http.Request) {
	spaceID := strings.TrimSpace(r.URL.Query().Get("space_id"))
	if spaceID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少空间ID参数"})
		return
	}
	if h.Config.ChatProvider == "openai" {
		bots := make([]map[string]any, 0, 1)
		if model := h.Config.DefaultModel(); model != "" {
			bots = append(bots, map[string]any{
				"bot_id": modelBotID(model), "name": model,
				"description": "平台默认模型，支持多轮会话上下文",
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 0, "msg": "success", "data": map[string]any{
				"space_bots": bots, "total": len(bots),
			},
		})
		return
	}
	if h.Config.ChatProvider == "ctyun" && (!h.Coze.Configured() || spaceID == "ctyun-workspace") {
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 0, "msg": "success",
			"data": map[string]any{
				"space_bots": []map[string]any{{
					"bot_id": "ctyun-default", "name": "天翼云模型",
					"description": "由 CTYun 模型接口提供回答",
				}},
				"total": 1,
			},
		})
		return
	}
	query := url.Values{}
	query.Set("space_id", spaceID)
	query.Set("page_size", defaultString(r.URL.Query().Get("page_size"), "10"))
	query.Set("page_index", defaultString(r.URL.Query().Get("page_num"), "1"))
	h.callCozeJSON(w, r, http.MethodGet, "/v1/space/published_bots_list", query, nil, nil)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
