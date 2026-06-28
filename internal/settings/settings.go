package settings

import (
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/define"
)

func RegisterRouting(e *echo.Echo) {
	e.GET(define.RegularPages.Settings.Path, pageHome)
	e.GET(define.RegularPages.Settings.Path+"/", pageHome)
}

func pageHome(c *echo.Context) error {
	target := define.SettingPages.Theme.Path
	if rawQuery := c.QueryString(); rawQuery != "" {
		if parsed, err := url.Parse(target); err == nil {
			parsed.RawQuery = rawQuery
			target = parsed.String()
		}
	}
	return c.Redirect(http.StatusFound, target)
}
