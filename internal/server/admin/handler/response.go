package handler

import (
	"net/http"

	"github.com/go-zoox/zoox"
)

func ok(ctx *zoox.Context, data any) {
	ctx.JSON(http.StatusOK, zoox.H{"ok": true, "data": data})
}

func fail(ctx *zoox.Context, code int, message string) {
	ctx.JSON(code, zoox.H{"ok": false, "error": message})
}
