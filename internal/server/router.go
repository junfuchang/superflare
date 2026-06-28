package server

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/junfuchang/superflare/config/data"
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
	"github.com/junfuchang/superflare/internal/statuspage"
)

func normalizeRouterFlags(flags model.Flags) model.Flags {
	flags.Visibility = strings.ToUpper(strings.TrimSpace(flags.Visibility))
	flags.User = strings.TrimSpace(flags.User)
	flags.Pass = strings.TrimSpace(flags.Pass)
	flags.CookieName = strings.TrimSpace(flags.CookieName)
	flags.CookieSecret = strings.TrimSpace(flags.CookieSecret)
	return flags
}

func validateRouterFlags(flags model.Flags) error {
	if flags.Port < 1 || flags.Port > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", flags.Port)
	}
	if flags.Visibility != "" {
		switch flags.Visibility {
		case "DEFAULT", "PRIVATE":
		default:
			return fmt.Errorf("invalid visibility %q: must be DEFAULT or PRIVATE", flags.Visibility)
		}
	}
	if !flags.DisableLoginMode {
		if err := data.ValidateLoginCredentialPair(flags.User, flags.Pass, "router login config"); err != nil {
			return err
		}
		if flags.User == "" || flags.Pass == "" {
			return fmt.Errorf("invalid login credentials: username and password cannot be empty when login is enabled")
		}
		if strings.TrimSpace(flags.CookieName) == "" {
			return fmt.Errorf("invalid cookie name: value cannot be empty when login is enabled")
		}
		if strings.TrimSpace(flags.CookieSecret) == "" {
			return fmt.Errorf("invalid cookie secret: value cannot be empty when login is enabled")
		}
	}
	return nil
}

// NewRouter builds the Echo app and returns an http.Handler for the server.
// It returns an error if any required initialization (templates, mdi, guide, editor) fails.
// The given appFlags are used as the single source of truth and synced to define.AppFlags.
func NewRouter(appFlags *model.Flags) (http.Handler, error) {
	effectiveFlags := define.AppFlags
	if appFlags != nil {
		effectiveFlags = *appFlags
	}
	effectiveFlags = normalizeRouterFlags(effectiveFlags)
	if err := validateRouterFlags(effectiveFlags); err != nil {
		return nil, fmt.Errorf("validate router flags: %w", err)
	}
	if err := data.EnsureAppConfigExists(); err != nil {
		return nil, fmt.Errorf("initialize app config: %w", err)
	}
	if err := data.EnsureRuntimeDataFiles(); err != nil {
		return nil, fmt.Errorf("initialize runtime data: %w", err)
	}

	if appFlags != nil {
		define.AppSourceFlags = effectiveFlags
		define.AppBaseFlags = effectiveFlags
		define.AppFlags = effectiveFlags
		auth.StoreLoginRuntimeConfigFromFlags(effectiveFlags)
	}

	if err := define.InitE(); err != nil {
		return nil, fmt.Errorf("initialize theme state: %w", err)
	}

	e := echo.New()
	e.HTTPErrorHandler = statuspage.HTTPErrorHandler
	e.Use(middleware.Recover())
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{MinLength: 1024}))
	if os.Getenv("FLARE_BASELINE") != "1" {
		log := logger.GetLogger()
		e.Use(logger.NewEchoWithConfig(log, logger.LoggerConfig{Skipper: logger.DefaultRequestLogSkipper}))
	}

	auth.RequestHandle(e)
	if err := templates.RegisterRouting(e); err != nil {
		return nil, fmt.Errorf("initialize templates: %w", err)
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
		return nil, fmt.Errorf("initialize mdi resources: %w", err)
	}
	mdi.RegisterRouting(e)
	redir.RegisterRouting(e)

	if define.AppFlags.EnableGuide {
		if err := guide.Init(); err != nil {
			return nil, fmt.Errorf("initialize guide page: %w", err)
		}
		guide.RegisterRouting(e)
	}
	if define.AppFlags.EnableEditor {
		if err := editor.Init(); err != nil {
			return nil, fmt.Errorf("initialize editor: %w", err)
		}
		editor.RegisterRouting(e)
	}

	return e, nil
}
