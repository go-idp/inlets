//go:build !adminui

package static

import (
	"io/fs"
)

func uiRoot() (fs.FS, error) {
	return nil, fs.ErrNotExist
}
