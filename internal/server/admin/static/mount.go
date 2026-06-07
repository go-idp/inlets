package static

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-zoox/zoox"
)

// Mount serves the embedded SPA and falls back to index.html for client routes.
func Mount(app *zoox.Application, basePath string) error {
	sub, err := uiRoot()
	if err != nil {
		app.Get("/", func(ctx *zoox.Context) {
			ctx.String(http.StatusOK, "inlets admin API — build UI: cd admin && pnpm install && pnpm build")
		})
		return nil
	}

	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return err
	}
	shell := indexHTML
	fsys := http.FS(sub)
	prefix := normalizeBase(basePath)

	app.Use(func(ctx *zoox.Context) {
		if ctx.Method != http.MethodGet && ctx.Method != http.MethodHead {
			ctx.Next()
			return
		}
		p := ctx.Request.URL.Path
		if strings.HasPrefix(p, "/api/") {
			ctx.Next()
			return
		}
		if prefix != "/" && !strings.HasPrefix(p, prefix) {
			ctx.Next()
			return
		}
		rel := strings.TrimPrefix(p, prefix)
		if rel == "" || rel == "/" {
			ctx.Data(http.StatusOK, "text/html; charset=utf-8", shell)
			return
		}
		if !strings.Contains(rel, ".") {
			ctx.Data(http.StatusOK, "text/html; charset=utf-8", shell)
			return
		}
		ctx.Next()
	})

	mountPath := prefix
	if mountPath != "/" {
		app.StaticFS(mountPath, fsys)
	} else {
		app.StaticFS("/", fsys)
	}
	return nil
}

func normalizeBase(basePath string) string {
	p := strings.TrimSpace(basePath)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}
