package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"pingtai/internal/service/openai"
)

func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if !h.Model.Configured() {
		writeModelError(w, http.StatusServiceUnavailable, "Model API is not configured; open /setup", "server_error", "model_not_configured", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		status := http.StatusBadRequest
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			status = http.StatusRequestEntityTooLarge
		}
		writeModelError(w, status, "Cannot read request body (maximum 10 MB)", "invalid_request_error", "invalid_body", "")
		return
	}
	resp, err := h.Model.Do(r.Context(), body)
	if err != nil {
		var requestError *openai.RequestError
		if errors.As(err, &requestError) {
			writeModelError(w, http.StatusBadRequest, requestError.Message, "invalid_request_error", "invalid_request", requestError.Param)
		} else {
			writeModelError(w, http.StatusBadGateway, "Cannot reach the upstream model service", "api_error", "upstream_unavailable", "")
		}
		return
	}
	proxyModelResponse(w, resp)
}

func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	if !h.Model.Configured() {
		writeModelError(w, http.StatusServiceUnavailable, "Model API is not configured", "server_error", "model_not_configured", "")
		return
	}
	if client, ok := h.Model.(interface {
		Models(context.Context) (*http.Response, error)
	}); ok {
		response, err := client.Models(r.Context())
		if err != nil {
			writeModelError(w, http.StatusBadGateway, "Cannot retrieve upstream models", "api_error", "upstream_unavailable", "")
			return
		}
		if (response.StatusCode != http.StatusNotFound && response.StatusCode != http.StatusMethodNotAllowed) || h.Config.DefaultModel() == "" {
			proxyModelResponse(w, response)
			return
		}
		_ = response.Body.Close()
	}
	models := make([]map[string]any, 0, 1)
	if model := h.Config.DefaultModel(); model != "" {
		models = append(models, map[string]any{"id": model, "object": "model", "created": 0, "owned_by": "configured-provider"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

func proxyModelResponse(w http.ResponseWriter, resp *http.Response) {
	defer resp.Body.Close()
	for _, key := range []string{"Content-Type", "Cache-Control", "X-Request-Id", "X-Trace-Id", "Retry-After"} {
		if value := resp.Header.Get(key); value != "" {
			w.Header().Set(key, value)
		}
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
	}
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	buffer := make([]byte, 16*1024)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, err := w.Write(buffer[:n]); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func writeModelError(w http.ResponseWriter, status int, message, kind, code, param string) {
	var parameter any
	if param != "" {
		parameter = param
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"message": message, "type": kind, "param": parameter, "code": code,
	}})
}
