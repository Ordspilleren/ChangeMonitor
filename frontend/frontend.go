package frontend

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"
)

// frontendDist holds the compiled Svelte frontend.
// Run `make frontend` (or `cd frontend && npm run build`) before `go build`.
//
//go:embed all:dist
var frontendDist embed.FS

func FrontendDistFS() fs.FS {
	subFS, err := fs.Sub(frontendDist, "dist")
	if err != nil {
		slog.Error("frontend static files unavailable", "err", err)
		os.Exit(1)
	}
	return subFS
}
