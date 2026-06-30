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

func materializeRuntimeSecrets(flags model.Flags) (model.Flags, bool) {
	if flags.DisableLoginMode {
		return flags, false
	}
	if strings.TrimSpace(flags.CookieSecret) != define.DEFAULT_COOKIE_SECRET {
		return flags, false
	}
	flags.CookieSecret = data.GenerateRandomString(64)
	return flags, true
}

// NewRouter builds the Echo app and returns an http.Handler for the server.
// It returns an error if any required initialization (templates, mdi, guide, editor) fails.
// The given appFlags are used as the single source of truth and stored as the runtime snapshot.
func NewRouter(appFlags *model.Flags) (http.Handler, error) {
	effectiveFlags := define.CurrentAppRuntimeFlags()
	if appFlags != nil {
		effectiveFlags = *appFlags
	}
	effectiveFlags = normalizeRouterFlags(effectiveFlags)
	if err := validateRouterFlags(effectiveFlags); err != nil {
		return nil, fmt.Errorf("validate router flags: %w", err)
	}
	var replacedDefaultCookieSecret bool
	effectiveFlags, replacedDefaultCookieSecret = materializeRuntimeSecrets(effectiveFlags)
	if err := data.EnsureAppConfigExists(); err != nil {
		return nil, fmt.Errorf("initialize app config: %w", err)
	}
	if err := data.EnsureRuntimeDataFiles(); err != nil {
		return nil, fmt.Errorf("initialize runtime data: %w", err)
	}

	if appFlags != nil {
		define.StoreAppRuntimeFlags(effectiveFlags, effectiveFlags, effectiveFlags)
		auth.StoreLoginRuntimeConfigFromFlags(effectiveFlags)
	}
	if replacedDefaultCookieSecret {
		logger.GetLogger().Warn("登录已启用且 CookieSecret 仍为默认值，已生成仅当前进程有效的临时会话密钥；请设置 FLARE_COOKIE_SECRET 或 --cookie-secret 以保持重启后的登录会话稳定")
	}

	if err := define.InitE(); err != nil {
		return nil, fmt.Errorf("initialize theme state: %w", err)
	}

	assets.SetRuntimeFlags(effectiveFlags)
	templates.SetRuntimeFlags(effectiveFlags)
	mdi.SetRuntimeFlags(effectiveFlags)
	editor.SetRuntimeFlags(effectiveFlags)
	home.StoreRuntimeFlags(effectiveFlags)
	home.SetHelpRuntimeFlags(effectiveFlags)
	guide.SetRuntimeFlags(effectiveFlags)

	e := echo.New()
	e.HTTPErrorHandler = statuspage.HTTPErrorHandler
	e.Use(middleware.Recover())
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{MinLength: 1024}))
	if os.Getenv("FLARE_BASELINE") != "1" {
		log := logger.GetLogger()
		e.Use(logger.NewEchoWithConfig(log, logger.LoggerConfig{Skipper: logger.DefaultRequestLogSkipper}))
	}

	auth.RequestHandleWithFlags(e, effectiveFlags)
	if err := templates.RegisterRouting(e); err != nil {
		return nil, fmt.Errorf("initialize templates: %w", err)
	}
	settings.SetRuntimeFlags(effectiveFlags)
	assets.RegisterRouting(e)
	health.RegisterRouting(e)
	if !effectiveFlags.EnableEditor {
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

	if effectiveFlags.EnableGuide {
		if err := guide.Init(); err != nil {
			return nil, fmt.Errorf("initialize guide page: %w", err)
		}
		guide.RegisterRouting(e)
	}
	if effectiveFlags.EnableEditor {
		if err := editor.Init(); err != nil {
			return nil, fmt.Errorf("initialize editor: %w", err)
		}
		editor.RegisterRouting(e)
	}

	return e, nil
}
