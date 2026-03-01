package templates

import (
	"embed"
	"io/fs"
)

//go:embed *.html partials/*.html
var content embed.FS

func FS() fs.FS {
	return content
}
