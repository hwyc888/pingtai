package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"pingtai/internal/service/openai"
)

// Model discovery uses unsaved form values and does not require a configured database.
func (h *Handler) fetchModels(w http.ResponseWriter, r *http.Request) {
	var request struct {
		BaseURL     string
		APIKey      string
		ClearAPIKey bool
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		h.respond(w, http.StatusBadRequest, map[string]string{"error": "Invalid model discovery request."})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		h.respond(w, http.StatusBadRequest, map[string]string{"error": "Only one JSON object is allowed."})
		return
	}
	if _, err := openai.Endpoint(request.BaseURL, "models"); err != nil {
		h.respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	candidate := h.current
	candidate.OpenAIBaseURL = request.BaseURL
	if request.ClearAPIKey {
		candidate.OpenAIAPIKey = ""
	} else if strings.TrimSpace(request.APIKey) != "" {
		candidate.OpenAIAPIKey = strings.TrimSpace(request.APIKey)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	response, err := openai.NewClient(candidate).Models(ctx)
	if err != nil {
		h.respond(w, http.StatusBadGateway, map[string]string{"error": "Cannot reach the model listing endpoint. Check Base URL, connectivity and timeout."})
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		h.respond(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("Model listing returned HTTP %d. Check the API key and /models support; you can still add model IDs manually.", response.StatusCode)})
		return
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(contents) > 4<<20 {
		h.respond(w, http.StatusBadGateway, map[string]string{"error": "Cannot read model listing (maximum 4 MB)."})
		return
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(contents, &result); err != nil || result.Data == nil {
		h.respond(w, http.StatusBadGateway, map[string]string{"error": "Expected an OpenAI model list containing data[].id. Add IDs manually if unsupported."})
		return
	}
	models := make([]string, 0, len(result.Data))
	seen := make(map[string]bool)
	for _, model := range result.Data {
		identifier := strings.TrimSpace(model.ID)
		if identifier != "" && !seen[identifier] {
			models = append(models, identifier)
			seen[identifier] = true
		}
	}
	sort.Strings(models)
	h.respond(w, http.StatusOK, map[string]any{"models": models})
}
