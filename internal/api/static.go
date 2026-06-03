package api

import (
	"io/fs"
	"net/http"

	"github.com/USERNAME/visiorama/web"
)

// spaHandler serves the embedded Vue dist files.
// Unknown paths fall back to index.html for SPA client-side routing.
type spaHandler struct {
	fs http.FileSystem
}

func newSPAHandler() http.Handler {
	sub, err := fs.Sub(web.DistFS, "app/dist")
	if err != nil {
		panic("embed app/dist not found: " + err.Error())
	}
	return &spaHandler{fs: http.FS(sub)}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, err := h.fs.Open(r.URL.Path)
	if err == nil {
		f.Close()
		http.FileServer(h.fs).ServeHTTP(w, r)
		return
	}
	// Not found → index.html (Vue Router handles the path client-side)
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	http.FileServer(h.fs).ServeHTTP(w, r2)
}
