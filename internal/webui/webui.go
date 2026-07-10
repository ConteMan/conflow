package webui

import (
	"embed"
	"io/fs"
)

//go:embed assets
var assets embed.FS

func Files() (fs.FS, error) {
	return fs.Sub(assets, "assets")
}
