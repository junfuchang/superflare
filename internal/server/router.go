package server

import (
	"fmt"
	"net/http"
	"os"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/logger"
	"github.com/junfuchang/superflare/internal/misc/health"
	"github.com/junfuchang/superflare/internal/misc/redir"
	"github.com/junfuchang/superflare/internal/pages/editor"
	"github.com/junfuchang/superflare/internal/pages/guide"
	"github.com/junfuchang/superflare/internal/pages/home"
	"github.com/junfuchang/superflare/internal/resources/assets"
	"github.com/junfuchang/superflare/internal/resources/mdi"
	"github.com/junfuchang/superflare/internal/resources/templates"
	"github.com/junfuchang/superflare/internal/settings"
	"github.com/junfuchang/superflare/internal/settings/appearance"
	"github.com/junfuchang/superflare/internal/settings/others"
	settingsports "github.com/junfuchang/superflare/internal/settings/ports"
	"github.com/junfuchang/superflare/internal/settings/search"
	"github.com/junfuchang/superflare/internal/settings/theme"
)

// NewRouter builds the Echo app and returns an http.Handler for the server.
// It returns an error if any required initialization (templates, mdi, guide, editor) fails.
// The given appFlags are used as the single source of truth and synced to define.AppFlags.
func NewRouter(appFlags *model.Flags) (http.Handler, error) {
	define.Init()
	if appFlags != nil {
		if define.AppBaseFlags.Port == 0 {
			define.AppBaseFlags = *appFlags
		}
		define.AppFlags = *appFlags
	}
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{MinLength: 1024}))
	if os.Getenv("FLARE_BASELINE") != "1" {
		log := logger.GetLogger()
		e.Use(logger.NewEchoWithConfig(log, logger.LoggerConfig{Skipper: logger.DefaultRequestLogSkipper}))
	}
	auth.RequestHandle(e)
	if err := templates.RegisterRouting(e); err != nil {
		return nil, fmt.Errorf("初始化模板: %w", err)
	}
	assets.RegisterRouting(e)
	health.RegisterRouting(e)
	if !define.AppFlags.EnableEditor {
		editor.RegisterAssetRouting(e)
	}
	home.RegisterRouting(e)
	settings.RegisterRouting(e)
	theme.RegisterRouting(e)
	search.RegisterRouting(e)
	appearance.RegisterRouting(e)
	settingsports.RegisterRouting(e)
	others.RegisterRouting(e)
	if err := mdi.Init(); err != nil {
		return nil, fmt.Errorf("初始化 MDI 资源: %w", err)
	}
	mdi.RegisterRouting(e)
	redir.RegisterRouting(e)
	if define.AppFlags.EnableGuide {
		if err := guide.Init(); err != nil {
			return nil, fmt.Errorf("初始化引导页: %w", err)
		}
		guide.RegisterRouting(e)
	}
	if define.AppFlags.EnableEditor {
		if err := editor.Init(); err != nil {
			return nil, fmt.Errorf("初始化编辑器: %w", err)
		}
		editor.RegisterRouting(e)
	}
	return e, nil
}
