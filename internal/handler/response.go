package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"pingtai/internal/middleware"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"code": status, "msg": message})
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("request body is empty")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 10<<20)
	return json.NewDecoder(r.Body).Decode(target)
}

func (h *Handler) parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, h.Config.MaxUploadBytes)
	var err error
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
		err = r.ParseMultipartForm(16 << 20)
	} else {
		err = r.ParseForm()
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid form data: "+err.Error())
		return false
	}
	return true
}

func (h *Handler) identityForRequest(r *http.Request) (middleware.Identity, bool) {
	identity, ok := currentIdentity(r)
	if !ok || strings.TrimSpace(identity.Username) == "" {
		return middleware.Identity{}, false
	}
	if identity.UserID != "" {
		return identity, true
	}
	if identity.APIKey {
		identity.UserID = "api-client"
		identity.Provider = "api"
		return identity, true
	}
	if h.Users == nil {
		return middleware.Identity{}, false
	}
	provider := identity.Provider
	if provider == "" {
		provider = "local"
	}
	var stableID string
	if provider == "sso" {
		external, err := h.Users.EnsureExternalIdentity(r.Context(), "sso", identity.Username, identity.Username, identity.DisplayName)
		if err != nil {
			return middleware.Identity{}, false
		}
		stableID = external.ID
	} else {
		user, err := h.Users.FindByUsername(r.Context(), identity.Username)
		if err != nil {
			return middleware.Identity{}, false
		}
		local, err := h.Users.EnsureLocalIdentity(r.Context(), user)
		if err != nil {
			return middleware.Identity{}, false
		}
		stableID = local.ID
		provider = "local"
	}
	identity.UserID = stableID
	identity.Provider = provider
	return identity, true
}

func currentIdentity(r *http.Request) (middleware.Identity, bool) {
	return middleware.IdentityFromContext(r.Context())
}

func (h *Handler) callCozeJSON(w http.ResponseWriter, r *http.Request, method, path string, query url.Values, payload any, headers http.Header) {
	resp, err := h.Coze.DoJSON(r.Context(), method, path, query, payload, headers)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": err.Error(), "error": true})
		return
	}
	h.writeCozeResponse(w, resp)
}

func (h *Handler) writeCozeResponse(w http.ResponseWriter, resp *http.Response) {
	if resp == nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "empty Coze response", "error": true})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 502, "msg": err.Error(), "error": true})
		return
	}
	if resp.StatusCode >= 400 {
		if json.Valid(body) {
			writeJSON(w, http.StatusOK, json.RawMessage(body))
		} else {
			writeJSON(w, http.StatusOK, map[string]any{"code": resp.StatusCode, "msg": string(body), "error": true})
		}
		return
	}
	if len(body) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success"})
		return
	}
	if !json.Valid(body) {
		body, _ = json.Marshal(map[string]any{"data": string(body)})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func readJSONMap(r *http.Request) (map[string]any, error) {
	var payload map[string]any
	if err := decodeJSON(r, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = make(map[string]any)
	}
	return payload, nil
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func intValue(value any, fallback int) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case string:
		if parsed, err := strconv.Atoi(number); err == nil {
			return parsed
		}
	}
	return fallback
}
