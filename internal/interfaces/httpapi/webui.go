package httpapi

import (
	"embed"
	"fmt"
	"html/template"
)

//go:embed web/index.html
var webFS embed.FS

// indexTemplate is parsed once at process start so per-request renders are
// allocation-cheap and benefit from the standard library's safe context-aware
// HTML escaping for any user-supplied configuration values.
var indexTemplate = func() *template.Template {
	tpl, err := template.ParseFS(webFS, "web/index.html")
	if err != nil {
		panic(fmt.Errorf("parse embedded web ui template: %w", err))
	}
	return tpl
}()

// indexViewModel is the data passed to web/index.html. Only safe primitive
// values are exposed so html/template can handle context-aware escaping.
type indexViewModel struct {
	DBPath   string
	PageSize int
}
