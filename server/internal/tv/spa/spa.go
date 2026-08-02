// Package spa embeds the built TV admin SPA. The dist directory is populated
// at build time (`make tv-bundle`); the checked-in placeholder keeps local
// builds working without a web build. Serving is delegated to internal/spa so
// both admin SPAs share one index.html fallback implementation.
package spa

import (
	"embed"
	"io/fs"
	"net/http"

	basespa "github.com/aleksclark/primer/server/internal/spa"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded TV admin SPA.
func Handler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // embed is static; failure here is a build error
	}
	return basespa.HandlerFS(dist)
}
