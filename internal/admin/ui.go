package admin

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed index.html favicon.png static
var uiFS embed.FS

func UIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/admin/")
		if path == "" {
			path = "index.html"
		}

		// Try to serve the exact file (CSS, JS, favicon, etc.)
		data, err := uiFS.ReadFile(path)
		if err != nil {
			// SPA fallback: serve index.html for any unmatched path
			data, err = uiFS.ReadFile("index.html")
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}

		switch {
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case strings.HasSuffix(path, ".png"):
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=86400")
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		w.Write(data)
	})
}
