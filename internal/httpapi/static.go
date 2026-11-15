package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed all:web
var embeddedFrontend embed.FS

func spaFiles(root string) http.Handler {
	if strings.TrimSpace(root) != "" {
		absolute, err := filepath.Abs(root)
		if err == nil {
			if info, statErr := os.Stat(filepath.Join(absolute, "index.html")); statErr == nil && !info.IsDir() {
				return spaFS(os.DirFS(absolute))
			}
		}
	}

	site, err := fs.Sub(embeddedFrontend, "web")
	if err != nil {
		return http.NotFoundHandler()
	}
	return spaFS(site)
}

func spaFS(site fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(site))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}

		if fs.ValidPath(name) {
			if info, err := fs.Stat(site, name); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}

		fallback := r.Clone(r.Context())
		urlCopy := *r.URL
		urlCopy.Path = "/"
		fallback.URL = &urlCopy
		fileServer.ServeHTTP(w, fallback)
	})
}
