package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"pingtai/internal/service/coze"
)

func (h *Handler) WorkspacesList(w http.ResponseWriter, r *http.Request) {
	if h.Config.ChatProvider == "openai" {
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 0, "msg": "success", "data": map[string]any{
				"total_count": 1, "workspaces": []map[string]any{{"id": "openai-workspace", "name": "OpenAI 兼容模型"}},
			},
		})
		return
	}
	if h.Config.ChatProvider == "ctyun" && !h.Coze.Configured() {
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 0, "msg": "success",
			"data": map[string]any{
				"total_count": 1,
				"workspaces": []map[string]any{{
					"id": "ctyun-workspace", "name": "天翼云模型工作空间",
				}},
			},
		})
		return
	}
	resp, err := h.Coze.DoJSON(r.Context(), http.MethodGet, "/v1/workspaces", nil, nil, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": err.Error(), "error": true})
		return
	}
	body, status, err := coze.ReadResponse(resp)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 502, "msg": err.Error(), "error": true})
		return
	}
	if status >= 400 {
		h.writeCozeResponse(w, &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body))})
		return
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Workspaces []map[string]any `json:"workspaces"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 502, "msg": "Coze 工作空间响应格式错误"})
		return
	}
	filtered := make([]map[string]any, 0)
	for _, item := range result.Data.Workspaces {
		if h.Config.CozeWorkspaceID == "" || stringValue(item["id"]) == h.Config.CozeWorkspaceID {
			filtered = append(filtered, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": result.Code, "msg": result.Msg,
		"data": map[string]any{"total_count": len(filtered), "workspaces": filtered},
	})
}
