package handler

import (
	"net/http"
	"net/url"
	"strings"
)

func (h *Handler) CreateKnowledge(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	formatType := intValue(payload["format_type"], 0)
	data := map[string]any{
		"space_id":    payload["space_id"],
		"format_type": formatType,
		"name":        payload["name"],
		"description": payload["description"],
		"file_id":     payload["file_id"],
	}
	h.callCozeJSON(w, r, http.MethodPost, "/v1/datasets", nil, data, nil)
}

func (h *Handler) KnowledgeList(w http.ResponseWriter, r *http.Request) {
	spaceID := strings.TrimSpace(r.URL.Query().Get("space_id"))
	if spaceID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少空间ID参数"})
		return
	}
	query := url.Values{}
	query.Set("space_id", spaceID)
	for _, key := range []string{"name", "format_type", "page_num", "page_size"} {
		if value := r.URL.Query().Get(key); value != "" {
			query.Set(key, value)
		}
	}
	if query.Get("page_num") == "" {
		query.Set("page_num", "1")
	}
	if query.Get("page_size") == "" {
		query.Set("page_size", "10")
	}
	h.callCozeJSON(w, r, http.MethodGet, "/v1/datasets", query, nil, nil)
}

func (h *Handler) DeleteKnowledge(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	datasetID := strings.TrimSpace(stringValue(payload["dataset_id"]))
	if datasetID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少知识库ID参数"})
		return
	}
	h.callCozeJSON(w, r, http.MethodDelete, "/v1/datasets/"+url.PathEscape(datasetID), nil, nil, nil)
}

func (h *Handler) EditKnowledge(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	datasetID := strings.TrimSpace(stringValue(payload["dataset_id"]))
	if datasetID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少知识库ID参数"})
		return
	}
	data := map[string]any{
		"name":        payload["name"],
		"description": payload["description"],
		"file_id":     payload["file_id"],
	}
	h.callCozeJSON(w, r, http.MethodPut, "/v1/datasets/"+url.PathEscape(datasetID), nil, data, nil)
}
