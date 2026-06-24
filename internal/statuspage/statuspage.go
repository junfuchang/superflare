package statuspage

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/background"
)

const (
	defaultBodyStyle = "--color-background:#1a1a1a;--color-primary:#effbff;--color-accent:#ffa500;"
)

type Action struct {
	Label   string
	URL     string
	Primary bool
}

type Page struct {
	Lang        string
	Title       string
	Eyebrow     string
	Heading     string
	Lead        string
	Detail      string
	StatusLabel string
	Actions     []Action
}

func HTML(c *echo.Context, status int, page Page) error {
	options := currentOptionsFor(c)
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.HTMLBlob(status, renderPage(options, status, page))
}

func CurrentLocale(c *echo.Context) string {
	return normalizeLocale(currentOptionsFor(c).Locale)
}

func HTTPErrorHandler(c *echo.Context, err error) {
	if c == nil {
		return
	}

	status := http.StatusInternalServerError
	message := http.StatusText(status)
	if err != nil {
		if derived := echo.StatusCode(err); derived != 0 {
			status = derived
			message = http.StatusText(status)
		}
		if he, ok := err.(*echo.HTTPError); ok {
			status = he.Code
			if strings.TrimSpace(he.Message) != "" {
				message = he.Message
			} else if statusText := http.StatusText(status); statusText != "" {
				message = statusText
			}
		} else if strings.TrimSpace(err.Error()) != "" {
			message = err.Error()
		}
	}

	if shouldRenderHTML(c) {
		options := currentOptionsFor(c)
		c.Response().Header().Set("Cache-Control", "no-store")
		_ = c.HTMLBlob(status, renderPage(options, status, BuildHTTPErrorPage(normalizeLocale(options.Locale), status, message)))
		return
	}

	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(status)
		return
	}
	_ = c.String(status, message)
}

func BuildAuthInvalidCredentialsPage(locale string) Page {
	if isEnglish(locale) {
		return Page{
			Title:       "Login failed",
			Eyebrow:     "Authentication",
			Heading:     "Incorrect username or password",
			Lead:        "Please make sure both fields match the current SuperFlare login settings.",
			Detail:      "Return to the application settings page and try again.",
			StatusLabel: "400 / Login failed",
			Actions: []Action{
				{Label: "Back to settings", URL: define.SettingPages.Others.Path, Primary: true},
				{Label: "Back home", URL: define.RegularPages.Home.Path},
			},
		}
	}

	return Page{
		Title:       "登录失败",
		Eyebrow:     "身份验证",
		Heading:     "请输入正确的用户名和密码",
		Lead:        "当前输入与 SuperFlare 的登录配置不匹配，请检查后重试。",
		Detail:      "可返回应用设置页重新输入，或回到首页继续浏览。",
		StatusLabel: "400 / 登录失败",
		Actions: []Action{
			{Label: "返回应用设置", URL: define.SettingPages.Others.Path, Primary: true},
			{Label: "返回首页", URL: define.RegularPages.Home.Path},
		},
	}
}

func BuildAuthEmptyCredentialsPage(locale string) Page {
	if isEnglish(locale) {
		return Page{
			Title:       "Missing login fields",
			Eyebrow:     "Authentication",
			Heading:     "Username or password cannot be empty",
			Lead:        "Both fields are required before SuperFlare can create a login session.",
			Detail:      "Fill in both fields and submit again.",
			StatusLabel: "400 / Invalid input",
			Actions: []Action{
				{Label: "Back to settings", URL: define.SettingPages.Others.Path, Primary: true},
				{Label: "Back home", URL: define.RegularPages.Home.Path},
			},
		}
	}

	return Page{
		Title:       "登录信息不完整",
		Eyebrow:     "身份验证",
		Heading:     "用户名或密码不能为空",
		Lead:        "创建登录状态前必须同时填写用户名和密码。",
		Detail:      "请补全输入后再次提交。",
		StatusLabel: "400 / 输入无效",
		Actions: []Action{
			{Label: "返回应用设置", URL: define.SettingPages.Others.Path, Primary: true},
			{Label: "返回首页", URL: define.RegularPages.Home.Path},
		},
	}
}

func BuildAuthSessionSaveErrorPage(locale string) Page {
	if isEnglish(locale) {
		return Page{
			Title:       "Session save failed",
			Eyebrow:     "Authentication",
			Heading:     "SuperFlare could not save the login session",
			Lead:        "The request was accepted, but the session state could not be persisted.",
			Detail:      "Try again from the application settings page. If it keeps failing, verify the cookie secret and write permissions.",
			StatusLabel: "400 / Session error",
			Actions: []Action{
				{Label: "Back to settings", URL: define.SettingPages.Others.Path, Primary: true},
				{Label: "Back home", URL: define.RegularPages.Home.Path},
			},
		}
	}

	return Page{
		Title:       "登录状态保存失败",
		Eyebrow:     "身份验证",
		Heading:     "程序内部错误，保存登录状态失败",
		Lead:        "请求已提交，但当前登录状态没有成功写入会话。",
		Detail:      "请返回应用设置页重试；若持续失败，请检查 Cookie 密钥和运行目录写权限。",
		StatusLabel: "400 / 会话错误",
		Actions: []Action{
			{Label: "返回应用设置", URL: define.SettingPages.Others.Path, Primary: true},
			{Label: "返回首页", URL: define.RegularPages.Home.Path},
		},
	}
}

func BuildRedirectInvalidTargetPage(locale string) Page {
	if isEnglish(locale) {
		return Page{
			Title:       "Redirect target not available",
			Eyebrow:     "Link validation",
			Heading:     "No matching redirect target was found",
			Lead:        "The redirect link may be outdated, incomplete, or manually modified.",
			Detail:      "Return to the home page and open the bookmark again, or verify the source link configuration.",
			StatusLabel: "400 / Redirect blocked",
			Actions: []Action{
				{Label: "Back home", URL: define.RegularPages.Home.Path, Primary: true},
				{Label: "Open app settings", URL: define.SettingPages.Others.Path},
			},
		}
	}

	return Page{
		Title:       "跳转地址无效",
		Eyebrow:     "链接校验",
		Heading:     "找不到匹配的跳转地址",
		Lead:        "该跳转链接可能已经失效、参数不完整，或地址内容已被修改。",
		Detail:      "请返回首页重新打开书签，或检查源链接配置是否正确。",
		StatusLabel: "400 / 跳转已拦截",
		Actions: []Action{
			{Label: "返回首页", URL: define.RegularPages.Home.Path, Primary: true},
			{Label: "打开应用设置", URL: define.SettingPages.Others.Path},
		},
	}
}

func BuildHTTPErrorPage(locale string, status int, message string) Page {
	message = strings.TrimSpace(message)
	statusText := http.StatusText(status)
	if statusText == "" {
		statusText = "Error"
	}

	if isEnglish(locale) {
		page := Page{
			Title:       statusText,
			Eyebrow:     "Request status",
			StatusLabel: fmt.Sprintf("%d / %s", status, statusText),
			Actions: []Action{
				{Label: "Back home", URL: define.RegularPages.Home.Path, Primary: true},
				{Label: "Open app settings", URL: define.SettingPages.Others.Path},
			},
		}
		switch status {
		case http.StatusNotFound:
			page.Heading = "Page not found"
			page.Lead = "The address you opened does not map to any page in SuperFlare."
			page.Detail = "Check the URL or return to the home page."
		case http.StatusForbidden:
			page.Heading = "Access denied"
			page.Lead = "SuperFlare rejected this request."
			page.Detail = "Sign in first or verify the current access conditions."
		case http.StatusBadRequest:
			page.Heading = "Invalid request"
			page.Lead = "The request parameters are incomplete or invalid."
			page.Detail = safeDetail(message, "Review the submitted values and try again.")
		default:
			page.Heading = "This page is temporarily unavailable"
			page.Lead = "SuperFlare could not complete the current request."
			page.Detail = "Refresh the page or return home and try again."
		}
		return page
	}

	page := Page{
		Title:       "请求异常",
		Eyebrow:     "请求状态",
		StatusLabel: fmt.Sprintf("%d / %s", status, localStatusText(status)),
		Actions: []Action{
			{Label: "返回首页", URL: define.RegularPages.Home.Path, Primary: true},
			{Label: "打开应用设置", URL: define.SettingPages.Others.Path},
		},
	}
	switch status {
	case http.StatusNotFound:
		page.Heading = "页面不存在"
		page.Lead = "当前访问地址在 SuperFlare 中没有对应内容。"
		page.Detail = "请检查地址是否正确，或返回首页继续操作。"
	case http.StatusForbidden:
		page.Heading = "访问被拒绝"
		page.Lead = "SuperFlare 拒绝了当前请求。"
		page.Detail = "请先登录，或检查当前访问条件是否满足。"
	case http.StatusBadRequest:
		page.Heading = "请求无效"
		page.Lead = "当前请求参数不完整或格式不正确。"
		page.Detail = safeDetail(message, "请检查输入内容后重新提交。")
	default:
		page.Heading = "页面暂时不可用"
		page.Lead = "SuperFlare 当前无法完成这次请求。"
		page.Detail = "请刷新页面重试，或返回首页继续操作。"
	}
	return page
}

func currentOptionsFor(c *echo.Context) model.Application {
	options, err := data.GetAllSettingsOptions()
	if err == nil {
		options.Locale = normalizeLocale(options.Locale)
		if options.GlassEffect == "" {
			options.GlassEffect = "none"
		}
		return options
	}

	return model.Application{
		Locale:            localeFromRequest(c),
		BackgroundImage:   "",
		BackgroundBlur:    0,
		BackgroundOpacity: 100,
		GlassEffect:       "none",
	}
}

func renderPage(options model.Application, status int, page Page) []byte {
	page = normalizePage(options, status, page)
	assets := background.ResolveAssets(options)
	bodyStyle := bodyStyleValue()
	backgroundStyle := renderBackgroundStyle(options, assets)

	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="`)
	b.WriteString(template.HTMLEscapeString(page.Lang))
	b.WriteString(`"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><title>`)
	b.WriteString(template.HTMLEscapeString(page.Title))
	b.WriteString(`</title><style>*{margin:0;padding:0;box-sizing:border-box;}html{scrollbar-width:thin;scrollbar-color:var(--status-scrollbar, var(--color-accent)) rgba(255,255,255,.08);}body,*{scrollbar-width:thin;scrollbar-color:var(--status-scrollbar, var(--color-accent)) rgba(255,255,255,.08);}*::-webkit-scrollbar{width:10px;height:10px;}*::-webkit-scrollbar-track{background:rgba(255,255,255,.08);border-radius:999px;}*::-webkit-scrollbar-thumb{background:linear-gradient(180deg,var(--status-scrollbar, var(--color-accent)) 0%,color-mix(in srgb,var(--status-scrollbar, var(--color-accent)) 72%,transparent) 100%);border:2px solid color-mix(in srgb,var(--color-background) 55%,transparent);border-radius:999px;}body{min-height:100vh;background:var(--color-background);color:var(--color-primary);font-family:Roboto,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;font-size:14px;line-height:1.6;position:relative;overflow-x:hidden;}body::after{content:"";position:fixed;inset:0;z-index:-1;background:linear-gradient(180deg,rgba(0,0,0,.18),rgba(0,0,0,.40));}`)
	b.WriteString(backgroundStyle)
	b.WriteString(`a{text-decoration:none;color:inherit;}.status-page{min-height:100vh;display:flex;align-items:center;justify-content:center;padding:32px 18px;}.status-panel{width:min(720px,100%);padding:28px 26px;border-radius:8px;border:1px solid color-mix(in srgb,var(--color-primary) 16%,transparent);background:color-mix(in srgb,var(--color-background) 84%,transparent);box-shadow:0 24px 70px rgba(0,0,0,.24);}.status-brand{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:20px;}.status-brand span{font-size:12px;letter-spacing:0;text-transform:uppercase;color:var(--color-accent);font-weight:700;}.status-code{display:inline-flex;align-items:center;justify-content:center;padding:6px 10px;border-radius:999px;font-size:12px;font-weight:700;color:var(--color-background);background:var(--color-accent);}.status-eyebrow{font-size:12px;font-weight:700;color:var(--color-accent);text-transform:uppercase;margin-bottom:10px;}.status-panel h1{font-size:30px;line-height:1.18;margin-bottom:12px;color:var(--color-primary);}.status-lead{opacity:.86;font-size:15px;margin-bottom:10px;}.status-detail{opacity:.72;margin-bottom:22px;}.status-actions{display:flex;flex-wrap:wrap;gap:12px;}.status-action{display:inline-flex;align-items:center;justify-content:center;min-height:40px;padding:10px 16px;border-radius:4px;border:1px solid var(--color-accent);color:var(--color-primary);transition:background-color .15s,color .15s,opacity .15s;}.status-action-primary{background:var(--color-accent);color:var(--color-background);font-weight:700;}.status-action:hover{background:var(--color-primary);color:var(--color-background);}.status-action-primary:hover{opacity:.88;}@supports not (color:color-mix(in srgb,#fff 50%,transparent)){.status-panel{border-color:rgba(255,255,255,.16);background:rgba(0,0,0,.28);}*::-webkit-scrollbar-thumb{background:var(--status-scrollbar, var(--color-accent));border-color:rgba(0,0,0,.2);}}@media (max-width:560px){.status-page{padding:18px 14px;align-items:flex-start;}.status-panel{padding:22px 18px;}.status-brand{align-items:flex-start;flex-direction:column;}.status-panel h1{font-size:24px;}.status-actions{display:grid;grid-template-columns:1fr;}.status-action{width:100%;}}</style></head><body style="`)
	b.WriteString(template.HTMLEscapeString(bodyStyle))
	b.WriteString(`"><main class="status-page"><section class="status-panel"><div class="status-brand"><span>`)
	b.WriteString(template.HTMLEscapeString(siteBrand(options)))
	b.WriteString(`</span><strong class="status-code">`)
	b.WriteString(template.HTMLEscapeString(page.StatusLabel))
	b.WriteString(`</strong></div><p class="status-eyebrow">`)
	b.WriteString(template.HTMLEscapeString(page.Eyebrow))
	b.WriteString(`</p><h1>`)
	b.WriteString(template.HTMLEscapeString(page.Heading))
	b.WriteString(`</h1><p class="status-lead">`)
	b.WriteString(template.HTMLEscapeString(page.Lead))
	b.WriteString(`</p>`)
	if strings.TrimSpace(page.Detail) != "" {
		b.WriteString(`<p class="status-detail">`)
		b.WriteString(template.HTMLEscapeString(page.Detail))
		b.WriteString(`</p>`)
	}
	if len(page.Actions) > 0 {
		b.WriteString(`<div class="status-actions">`)
		for _, action := range page.Actions {
			if strings.TrimSpace(action.URL) == "" || strings.TrimSpace(action.Label) == "" {
				continue
			}
			className := "status-action"
			if action.Primary {
				className += " status-action-primary"
			}
			b.WriteString(`<a class="`)
			b.WriteString(className)
			b.WriteString(`" href="`)
			b.WriteString(template.HTMLEscapeString(action.URL))
			b.WriteString(`">`)
			b.WriteString(template.HTMLEscapeString(action.Label))
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section></main></body></html>`)
	return []byte(b.String())
}

func normalizePage(options model.Application, status int, page Page) Page {
	if strings.TrimSpace(page.Lang) == "" {
		page.Lang = normalizeLocale(options.Locale)
	}
	if strings.TrimSpace(page.Title) == "" {
		page.Title = siteBrand(options)
	}
	if strings.TrimSpace(page.Eyebrow) == "" {
		if isEnglish(page.Lang) {
			page.Eyebrow = "Request status"
		} else {
			page.Eyebrow = "请求状态"
		}
	}
	if strings.TrimSpace(page.StatusLabel) == "" {
		if statusText := http.StatusText(status); statusText != "" {
			page.StatusLabel = fmt.Sprintf("%d / %s", status, statusText)
		} else {
			page.StatusLabel = strconv.Itoa(status)
		}
	}
	if len(page.Actions) == 0 {
		if isEnglish(page.Lang) {
			page.Actions = []Action{{Label: "Back home", URL: define.RegularPages.Home.Path, Primary: true}}
		} else {
			page.Actions = []Action{{Label: "返回首页", URL: define.RegularPages.Home.Path, Primary: true}}
		}
	}
	return page
}

func shouldRenderHTML(c *echo.Context) bool {
	if c == nil || c.Request() == nil {
		return false
	}

	method := c.Request().Method
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}

	path := c.Request().URL.Path
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/user-assets/") || path == "/favicon.ico" || path == "/ping" {
		return false
	}
	if ext := strings.TrimSpace(filepath.Ext(path)); ext != "" {
		return false
	}

	accept := strings.ToLower(c.Request().Header.Get("Accept"))
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml") || strings.Contains(accept, "*/*")
}

func renderBackgroundStyle(options model.Application, assets background.Assets) string {
	source := strings.TrimSpace(background.PreviewSource(assets))
	if source == "" {
		source = strings.TrimSpace(assets.FullURL)
	}

	var b strings.Builder
	if source != "" {
		opacity := clampInt(options.BackgroundOpacity, 0, 100)
		if opacity == 0 {
			opacity = 100
		}
		blur := clampInt(options.BackgroundBlur, 0, 80)
		b.WriteString(`body::before{content:"";position:fixed;inset:-24px;z-index:-2;background-image:url('`)
		b.WriteString(cssURLValue(source))
		b.WriteString(`');background-position:center center;background-size:cover;background-repeat:no-repeat;filter:blur(`)
		b.WriteString(strconv.Itoa(blur))
		b.WriteString(`px);opacity:`)
		b.WriteString(strconv.FormatFloat(float64(opacity)/100, 'f', 2, 64))
		b.WriteString(`;transform:scale(1.05);}`)
	}
	if assets.AccentColor != "" {
		b.WriteString(`body{--status-scrollbar:`)
		b.WriteString(assets.AccentColor)
		b.WriteString(`;}`)
	}
	if options.GlassEffect == "frosted" || options.GlassEffect == "liquid" {
		intensity := clampInt(options.GlassIntensity, 0, 100)
		if intensity > 0 {
			blur := 4 + intensity/6
			alphaTop := 0.08 + float64(intensity)/900
			alphaBottom := 0.18 + float64(intensity)/500
			b.WriteString(`body::after{backdrop-filter:blur(`)
			b.WriteString(strconv.Itoa(blur))
			b.WriteString(`px);-webkit-backdrop-filter:blur(`)
			b.WriteString(strconv.Itoa(blur))
			b.WriteString(`px);background:linear-gradient(180deg,rgba(255,255,255,`)
			b.WriteString(strconv.FormatFloat(alphaTop, 'f', 3, 64))
			b.WriteString(`),rgba(0,0,0,`)
			b.WriteString(strconv.FormatFloat(alphaBottom, 'f', 3, 64))
			b.WriteString(`));}`)
		}
	}
	return b.String()
}

func bodyStyleValue() string {
	style := strings.TrimSpace(string(define.GetAppBodyStyle()))
	if style == "" {
		return defaultBodyStyle
	}
	return style
}

func normalizeLocale(locale string) string {
	if isEnglish(locale) {
		return "en"
	}
	return "zh"
}

func localeFromRequest(c *echo.Context) string {
	if c == nil || c.Request() == nil {
		return "zh"
	}
	if strings.Contains(strings.ToLower(c.Request().Header.Get("Accept-Language")), "en") {
		return "en"
	}
	return "zh"
}

func isEnglish(locale string) bool {
	return strings.EqualFold(strings.TrimSpace(locale), "en")
}

func localStatusText(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "请求无效"
	case http.StatusForbidden:
		return "访问被拒绝"
	case http.StatusNotFound:
		return "页面不存在"
	case http.StatusInternalServerError:
		return "服务异常"
	default:
		if text := http.StatusText(status); text != "" {
			return text
		}
		return "请求异常"
	}
}

func safeDetail(message string, fallback string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return fallback
	}
	lower := strings.ToLower(message)
	if lower == "bad request" || lower == "forbidden" || lower == "not found" || lower == "internal server error" || lower == "request failed" {
		return fallback
	}
	return message
}

func siteBrand(options model.Application) string {
	title := strings.TrimSpace(options.Title)
	if title != "" {
		return title
	}
	return "SuperFlare"
}

func clampInt(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func cssURLValue(input string) string {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `'`, `\'`)
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	return input
}
