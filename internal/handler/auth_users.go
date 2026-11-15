package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"pingtai/internal/service/auth"
)

func (h *Handler) AddUser(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "请求参数错误"})
		return
	}
	username := strings.TrimSpace(stringValue(payload["username"]))
	password := stringValue(payload["password"])
	userType, hasUserType := payload["usertype"]
	role := intValue(userType, -1)
	if username == "" || password == "" || !hasUserType || role < 0 || role > 1 {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "缺少参数"})
		return
	}
	if _, err := h.Users.FindByUsername(r.Context(), username); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "用户已存在"})
		return
	} else if err != sql.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "查询用户失败"})
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"errcode": 500, "errmsg": "密码加密失败"})
		return
	}
	if _, err := h.Users.Create(r.Context(), username, hash, role); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "添加用户失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"errcode": 200, "errmsg": "添加成功"})
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "请求参数错误"})
		return
	}
	id := int64(intValue(payload["id"], 0))
	if id <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "缺少用户ID"})
		return
	}
	user, err := h.Users.FindByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "用户不存在"})
		return
	}
	identity, _ := currentIdentity(r)
	if user.Username == identity.Username {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "不能删除自己"})
		return
	}
	if err := h.Users.SoftDelete(r.Context(), id); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "删除失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"errcode": 200, "errmsg": "删除成功"})
}

func (h *Handler) GetUserList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("pagenum"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pagesize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}
	users, total, err := h.Users.List(r.Context(), page, pageSize)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": 500, "errmsg": "获取用户列表失败"})
		return
	}
	items := make([]map[string]any, 0, len(users))
	for _, user := range users {
		items = append(items, map[string]any{
			"id": user.ID, "username": user.Username,
			"usertype": user.UserType, "status": user.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": items, "total": total, "errcode": "200", "errmsg": "success",
		"pagenum": strconv.Itoa(page), "pagesize": strconv.Itoa(pageSize),
	})
}

func (h *Handler) EditUser(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "请求参数错误"})
		return
	}
	id := int64(intValue(payload["id"], 0))
	password := stringValue(payload["password"])
	if id <= 0 || password == "" {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "缺少参数"})
		return
	}
	target, err := h.Users.FindByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "用户不存在"})
		return
	}
	identity, _ := currentIdentity(r)
	current, currentErr := h.Users.FindByUsername(r.Context(), identity.Username)
	if target.Username != identity.Username && (currentErr != nil || current.UserType != 1) {
		writeJSON(w, http.StatusForbidden, map[string]any{"errcode": 401, "errmsg": "权限受限"})
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "密码加密失败"})
		return
	}
	if err := h.Users.UpdatePassword(r.Context(), id, hash); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": "501", "errmsg": "修改失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"errcode": 200, "errmsg": "修改成功"})
}
