package templates

import (
	"bytes"
	"embed"
	"html/template"
	"io"
	"sync"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/i18n"
	"github.com/junfuchang/superflare/internal/resources/assets"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

//go:embed html
var TPL embed.FS

var bufPool = sync.Pool{
	New: func() any { return &bytes.Buffer{} },
}

// Renderer implements echo.Renderer for HTML templates.
type Renderer struct {
	templates *template.Template
}

func (r *Renderer) Render(c *echo.Context, w io.Writer, templateName string, data any) error {
	tmplName := templateName
	for _, cand := range []string{templateName, "html/" + templateName, "embed/templates/" + templateName} {
		if r.templates.Lookup(cand) != nil {
			tmplName = cand
			break
		}
	}
	buf, ok := bufPool.Get().(*bytes.Buffer)
	if !ok || buf == nil {
		buf = &bytes.Buffer{}
	}
	buf.Reset()
	defer bufPool.Put(buf)
	if err := r.templates.ExecuteTemplate(buf, tmplName, data); err != nil {
		return err
	}
	_, err := buf.WriteTo(w)
	return err
}

var templateFuncMap = template.FuncMap{
	"T":                   i18n.T,
	"IconURL":             mdi.GetIconURLByName,
	"SiteIconURL":         func(name string) string { return assets.SiteIconURL(mdi.GetIconURLByName, name) },
	"AppleTouchIconURL":   assets.AppleTouchIconURL,
	"AndroidChrome192URL": assets.AndroidChrome192URL,
	"AndroidChrome512URL": assets.AndroidChrome512URL,
}

func RegisterRouting(e *echo.Echo) error {
	var t *template.Template
	var err error
	if define.AppFlags.DebugMode {
		t, err = template.New("").Funcs(templateFuncMap).ParseGlob("embed/templates/*.html")
	} else {
		t, err = template.New("").Funcs(templateFuncMap).ParseFS(TPL, "html/*.html")
	}
	if err != nil {
		return err
	}
	e.Renderer = &Renderer{templates: t}
	return nil
}
