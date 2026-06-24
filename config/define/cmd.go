package define

import (
	"github.com/junfuchang/superflare/config/model"
)

const (
	DEFAULT_PORT                = 3636
	DEFAULT_ENABLE_GUIDE        = true
	DEFAULT_ENABLE_MINI_REQUEST = false
	DEFAULT_DISABLE_LOGIN       = true
	DEFAULT_USER_NAME           = "admin"
	DEFAULT_ENABLE_EDITOR       = true
	DEFAULT_VISIBILITY          = "DEFAULT"
	DEFAULT_DISABLE_CSP         = false

	DEFAULT_LOGIN_USER = "admin"
	DEFAULT_LOGIN_PASS = "admin"

	DEFAULT_COOKIE_NAME   = "superflare"
	DEFAULT_COOKIE_SECRET = "secret"
)

// get default env config
func GetDefaultEnvVars() model.Envs {
	return model.Envs{
		Port:                 DEFAULT_PORT,
		EnableGuide:          DEFAULT_ENABLE_GUIDE,
		EnableMinimumRequest: DEFAULT_ENABLE_MINI_REQUEST,
		DisableLoginMode:     DEFAULT_DISABLE_LOGIN,
		EnableEditor:         DEFAULT_ENABLE_EDITOR,
		Visibility:           DEFAULT_VISIBILITY,
		DisableCSP:           DEFAULT_DISABLE_CSP,

		User: DEFAULT_LOGIN_USER,
		Pass: DEFAULT_LOGIN_PASS,

		CookieName:   DEFAULT_COOKIE_NAME,
		CookieSecret: DEFAULT_COOKIE_SECRET,
	}
}

var DefaultEnvVars = GetDefaultEnvVars()

var AppFlags model.Flags
var AppBaseFlags model.Flags

// FLARE_VISIBLE defines visibility levels: "DEFAULT" or "PRIVATE".
var FLARE_VISIBLE = "PRIVATE"
