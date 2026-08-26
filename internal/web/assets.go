package web

import (
	"embed"
	"net/http"
)

//go:embed ui/index.html ui/app.css ui/app.js
var assets embed.FS

var IndexHTML = mustAsset("ui/index.html")

func mustAsset(name string) []byte {
	b, err := assets.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return b
}

func Register(mux *http.ServeMux) {
	mux.HandleFunc("/static/app.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(mustAsset("ui/app.css"))
	})
	mux.HandleFunc("/static/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write(mustAsset("ui/app.js"))
	})
}
