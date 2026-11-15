package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	for _, expected := range []string{
		"智能AI",
		"./config/index.js",
		"./assets/index-BMDc8Iok.js",
		"./assets/index-B3VI5I9g.css",
	} {
		if !strings.Contains(index.Body.String(), expected) {
			t.Fatalf("embedded index missing %q", expected)
		}
	}

	config := httptest.NewRecorder()
	handler.ServeHTTP(config, httptest.NewRequest(http.MethodGet, "/config/index.js", nil))
	if config.Code != http.StatusOK || !strings.Contains(config.Body.String(), "baseURL") {
		t.Fatalf("embedded config response: status=%d body=%q", config.Code, config.Body.String())
	}

	script := httptest.NewRecorder()
	handler.ServeHTTP(script, httptest.NewRequest(http.MethodGet, "/assets/index-BMDc8Iok.js", nil))
	if script.Code != http.StatusOK || script.Body.Len() < 100000 {
		t.Fatalf("embedded JS bundle missing or incomplete: status=%d bytes=%d", script.Code, script.Body.Len())
	}

	route := httptest.NewRecorder()
	handler.ServeHTTP(route, httptest.NewRequest(http.MethodGet, "/workspace/chat", nil))
	if route.Code != http.StatusOK || !strings.Contains(route.Body.String(), "智能AI") {
		t.Fatalf("embedded SPA fallback: status=%d body=%q", route.Code, route.Body.String())
	}
}
