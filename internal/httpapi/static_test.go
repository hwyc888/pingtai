package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSPAFilesAndAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<main>home</main>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("window.app=true"), 0600); err != nil {
		t.Fatal(err)
	}

	handler := spaFiles(root)
	for _, path := range []string{"/", "/workspace/chat"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || recorder.Body.String() != "<main>home</main>" {
			t.Fatalf("SPA path %s: status=%d body=%q", path, recorder.Code, recorder.Body.String())
		}
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if asset.Code != http.StatusOK || asset.Body.String() != "window.app=true" {
		t.Fatalf("asset response: status=%d body=%q", asset.Code, asset.Body.String())
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset should be 404, got %d", missing.Code)
	}
}

func TestEmbeddedFrontendIsServedWithoutExternalDirectory(t *testing.T) {
	handler := spaFiles("")

	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK {
		t.Fatalf("embedded index status=%d", index.Code)
	}
	for _, expected := range []string{"<div id=\"app\"></div>", "/assets/"} {
		if !strings.Contains(index.Body.String(), expected) {
			t.Fatalf("embedded index missing %q", expected)
		}
	}
	scriptPath := regexp.MustCompile("src=\"([^\"]+\\.js)\"").FindStringSubmatch(index.Body.String())
	if len(scriptPath) != 2 {
		t.Fatalf("embedded index has no built JavaScript entry: %s", index.Body.String())
	}
	script := httptest.NewRecorder()
	handler.ServeHTTP(script, httptest.NewRequest(http.MethodGet, scriptPath[1], nil))
	if script.Code != http.StatusOK || script.Body.Len() < 1000 {
		t.Fatalf("embedded JS bundle missing or incomplete: status=%d bytes=%d", script.Code, script.Body.Len())
	}

	route := httptest.NewRecorder()
	handler.ServeHTTP(route, httptest.NewRequest(http.MethodGet, "/workspace/chat", nil))
	if route.Code != http.StatusOK || !strings.Contains(route.Body.String(), "<div id=\"app\"></div>") {
		t.Fatalf("embedded SPA fallback: status=%d body=%q", route.Code, route.Body.String())
	}
}


func TestAdminPageAndProviderAPIRoutesDoNotConflict(t *testing.T) {
	root := http.NewServeMux()
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Route", "admin-api")
		w.WriteHeader(http.StatusNoContent)
	})
	frontend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Route", "frontend")
		w.WriteHeader(http.StatusOK)
	})
	root.Handle("/admin/providers", api)
	root.Handle("/admin/providers/", api)
	root.Handle("/admin", frontend)
	root.Handle("/admin/", frontend)

	for _, path := range []string{"/admin", "/admin/"} {
		response := httptest.NewRecorder()
		root.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Header().Get("X-Route") != "frontend" {
			t.Fatalf("admin UI route %s should serve frontend, got status=%d route=%q", path, response.Code, response.Header().Get("X-Route"))
		}
	}
	for _, path := range []string{"/admin/providers", "/admin/providers/save"} {
		response := httptest.NewRecorder()
		root.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNoContent || response.Header().Get("X-Route") != "admin-api" {
			t.Fatalf("provider API route %s should reach API mux, got status=%d route=%q", path, response.Code, response.Header().Get("X-Route"))
		}
	}
}
