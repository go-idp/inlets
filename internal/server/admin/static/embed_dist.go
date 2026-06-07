//go:build adminui

package static

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func uiRoot() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
