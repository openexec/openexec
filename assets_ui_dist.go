//go:build ui_dist

package openexec

import (
	"embed"
	"io/fs"
)

// Release builds opt into the generated React bundle after npm run build.

//go:embed all:ui/dist
var uiAssets embed.FS

func GetUIFS() fs.FS {
	f, _ := fs.Sub(uiAssets, "ui/dist")
	return f
}
