// internal/web/render.go
package web

import (
	"log"
	"net/http"

	"github.com/a-h/templ"
)

// render writes a templ.Component to w as text/html, logging (not
// panicking) on a render error, since by the time Render is called
// headers may already be committed.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("web: render error: %v", err)
	}
}
