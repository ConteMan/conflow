package server

import (
	"io/fs"
	"net/http"
	"path"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/webui"
)

func New(service *app.Service) http.Handler {
	assets, err := webui.Files()
	if err != nil {
		panic(err)
	}
	root := http.NewServeMux()
	root.Handle("/api/", newAPI(service).handler())
	root.Handle("/", frontend(assets))
	return root
}

func frontend(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if request.URL.Path == "/" {
			request.URL.Path = "/index.html"
		}
		if path.Ext(request.URL.Path) == "" {
			request.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(writer, request)
	})
}
