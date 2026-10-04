package console

import (
	"embed"
	"io/fs"
	"net/http"
)

// The UI is plain HTML, CSS and ES modules: no build step and no
// frameworks. Test files (*.test.mjs) are not embedded.
//
//go:embed web/*.html web/*.css web/*.js
var webFiles embed.FS

// UI serves the embedded console front end.
func UI() http.Handler {
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
