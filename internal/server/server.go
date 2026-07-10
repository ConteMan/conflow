package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"

	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/webui"
)

func New(manifest project.Manifest) http.Handler {
	assets, err := webui.Files()
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"status":     "ok",
			"project_id": manifest.Project.ID,
		})
	})
	mux.Handle("GET /", frontend(assets))
	return mux
}

func frontend(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			request.URL.Path = "/index.html"
		}
		if path.Ext(request.URL.Path) == "" {
			request.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(writer, request)
	})
}
