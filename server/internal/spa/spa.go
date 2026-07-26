// Package spa embeds the built admin SPA and serves it with an index.html
// fallback for client-side routing. The dist directory is populated at build
// time (see the Dockerfile and `make web`); the checked-in placeholder keeps
// local builds working without a web build.
package spa

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded SPA. Requests for files that exist are served
// directly; everything else falls back to index.html so client-side routes
// resolve on hard refresh.
func Handler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed is static; failure here is a build error
	}
	fileServer := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
