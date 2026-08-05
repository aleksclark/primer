// Package version holds build-time identity for primer-student.
//
// Values are injected via -ldflags:
//
//	-X github.com/aleksclark/primer/server/internal/studentclient/version.Version=…
//	-X github.com/aleksclark/primer/server/internal/studentclient/version.Commit=…
//
// The main package also mirrors these into main.version / main.commit for a
// tiny -version path that does not pull the full app.
package version

// Version is the release or git-describe string (default "dev").
var Version = "dev"

// Commit is the short source revision (default "unknown").
var Commit = "unknown"

// String returns a human-readable version line.
func String() string {
	return Version + " (" + Commit + ")"
}
