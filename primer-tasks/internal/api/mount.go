package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Mount serves one independent Tasks application. The ingress forwards the
// public path unchanged: this is the only layer that strips base + /api.
func Mount(api http.Handler, base, webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/health", api)
	if webDir == "" && base == "" {
		return api
	} // existing API-only development server
	prefix := base + "/api"
	mux.Handle(prefix+"/", http.StripPrefix(prefix, api))
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	if base != "" {
		mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, base+"/", http.StatusPermanentRedirect)
		})
	}
	files := http.StripPrefix(base+"/", http.FileServer(http.Dir(webDir)))
	mux.Handle(base+"/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, base)
		if path == "/" || path == "/parent" || path == "/student" || strings.HasPrefix(path, "/parent/") || strings.HasPrefix(path, "/student/") {
			if _, err := os.Stat(filepath.Join(webDir, "index.html")); err != nil {
				http.Error(w, "Tasks UI unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	}))
	return mux
}
