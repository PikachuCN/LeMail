package frontend

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embedded embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return embedded
	}
	return sub
}
