package redir

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/statuspage"
)

var requestLooksLocalNetwork = fn.RequestLooksLocalNetwork

func RegisterRouting(e *echo.Echo) {
	e.GET(define.MiscPages.RedirHome.Path, func(c *echo.Context) error {
		return c.Redirect(http.StatusFound, define.RegularPages.Home.Path)
	})

	e.GET(define.MiscPages.RedirHelper.Path, func(c *echo.Context) error {
		encoded := c.QueryParam("go")
		if len(encoded) < 1 {
			return renderRedirectInvalidTarget(c)
		}
		decoded, err := data.Base64DecodeUrl(encoded)
		if err != nil {
			return renderRedirectInvalidTarget(c)
		}
		decodeURL := string(decoded)
		requestURL := fn.ParseRequestURLTo(c.Request())
		appsData, errApps := data.LoadFavoriteBookmarks()
		if errApps != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, errApps.Error()))
		}
		for _, bookmark := range appsData.Items {
			if fn.ParseDynamicUrlWith(bookmark.URL, &requestURL) == decodeURL {
				return c.Redirect(http.StatusFound, string(decoded))
			}
		}
		bookmarksData, errBookmarks := data.LoadNormalBookmarks()
		if errBookmarks != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, errBookmarks.Error()))
		}
		for _, bookmark := range bookmarksData.Items {
			if fn.ParseDynamicUrlWith(bookmark.URL, &requestURL) == decodeURL {
				return c.Redirect(http.StatusFound, string(decoded))
			}
		}
		return renderRedirectInvalidTarget(c)
	})

	e.GET(define.MiscPages.RedirLocal.Path, func(c *echo.Context) error {
		sourceURL, errSource := decodeRedirectParam(c.QueryParam("go"))
		localURL, errLocal := decodeRedirectParam(c.QueryParam("local"))
		if errSource != nil || errLocal != nil || !isHTTPRedirectURL(sourceURL) || !isHTTPRedirectURL(localURL) {
			return renderRedirectInvalidTarget(c)
		}
		pairExists, err := bookmarkLocalURLPairExists(c.Request(), sourceURL, localURL)
		if err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
		}
		if !pairExists {
			return renderRedirectInvalidTarget(c)
		}
		if !requestLooksLocalNetwork(c.Request()) {
			return c.Redirect(http.StatusFound, sourceURL)
		}
		options, err := data.GetAllSettingsOptions()
		if err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
		}
		statuspage.BindOptions(c, options)
		locale := options.Locale
		bodyStyle, styleWarning, err := statuspage.RequireConfiguredBodyStyleForRender(locale, "")
		if err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
		}
		var renderWarnings []string
		if styleWarning != "" {
			renderWarnings = append(renderWarnings, styleWarning)
		}
		page, err := renderLocalRedirectPage(sourceURL, localURL, options, bodyStyle, renderWarnings)
		if err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
		}
		return c.HTMLBlob(http.StatusOK, page)
	})
}

func renderRedirectInvalidTarget(c *echo.Context) error {
	if err := statuspage.BindCurrentOptions(c); err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildRedirectInvalidTargetPage(statuspage.CurrentLocale(c)))
}

func decodeRedirectParam(encoded string) (string, error) {
	if strings.TrimSpace(encoded) == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "missing redirect target")
	}
	decoded, err := data.Base64DecodeUrl(encoded)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(decoded)), nil
}

func isHTTPRedirectURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

func bookmarkLocalURLPairExists(r *http.Request, sourceURL string, localURL string) (bool, error) {
	requestURL := fn.ParseRequestURLTo(r)
	appsData, errApps := data.LoadFavoriteBookmarks()
	if errApps != nil {
		return false, errApps
	}
	if bookmarksContainLocalPair(appsData.Items, &requestURL, sourceURL, localURL) {
		return true, nil
	}
	bookmarksData, errBookmarks := data.LoadNormalBookmarks()
	if errBookmarks != nil {
		return false, errBookmarks
	}
	if bookmarksContainLocalPair(bookmarksData.Items, &requestURL, sourceURL, localURL) {
		return true, nil
	}
	return false, nil
}

func bookmarksContainLocalPair(bookmarks []model.Bookmark, requestURL *fn.DynamicURL, sourceURL string, localURL string) bool {
	for _, bookmark := range bookmarks {
		if fn.ParseDynamicUrlWith(bookmark.URL, requestURL) == sourceURL && fn.ParseDynamicUrlWith(bookmark.LocalURL, requestURL) == localURL {
			return true
		}
	}
	return false
}

type localRedirectTexts struct {
	Lang          string
	Title         string
	Eyebrow       string
	Heading       string
	Lead          string
	Checking      string
	Success       string
	FailedPrefix  string
	FailedSuffix  string
	LocalLabel    string
	SourceLabel   string
	LocalButton   string
	SourceButton  string
	Noscript      string
	CountdownUnit string
}

func getLocalRedirectTexts(locale string) localRedirectTexts {
	if strings.EqualFold(locale, "en") {
		return localRedirectTexts{
			Lang:          "en",
			Title:         "Trying the local address",
			Eyebrow:       "Local network first",
			Heading:       "Connecting to the local address",
			Lead:          "This device looks like it can use the LAN address, so SuperFlare is trying it before falling back to the source bookmark.",
			Checking:      "Checking local address availability...",
			Success:       "Local address is reachable. Opening...",
			FailedPrefix:  "The local address is not reachable. Opening the source bookmark in ",
			FailedSuffix:  " seconds.",
			LocalLabel:    "Local address",
			SourceLabel:   "Source bookmark",
			LocalButton:   "Open local address",
			SourceButton:  "Use source address",
			Noscript:      "JavaScript is disabled. Choose one address manually.",
			CountdownUnit: "s",
		}
	}
	return localRedirectTexts{
		Lang:          "zh-CN",
		Title:         "正在尝试内网地址",
		Eyebrow:       "内网优先访问",
		Heading:       "正在连接内网地址",
		Lead:          "当前访问环境看起来可以使用局域网地址，正在优先尝试打开内网地址。",
		Checking:      "正在检测内网地址可用性...",
		Success:       "内网地址可用，正在打开...",
		FailedPrefix:  "内网地址暂时无法访问，",
		FailedSuffix:  " 秒后打开源书签地址。",
		LocalLabel:    "内网地址",
		SourceLabel:   "源书签地址",
		LocalButton:   "打开内网地址",
		SourceButton:  "打开源书签地址",
		Noscript:      "浏览器未启用 JavaScript，请手动选择访问地址。",
		CountdownUnit: "秒",
	}
}

func renderLocalRedirectPage(sourceURL string, localURL string, options model.Application, bodyStyle template.CSS, renderWarnings []string) ([]byte, error) {
	texts := getLocalRedirectTexts(options.Locale)
	sourceHTML := template.HTMLEscapeString(sourceURL)
	localHTML := template.HTMLEscapeString(localURL)
	bodyStyleValue := template.HTMLEscapeString(string(bodyStyle))
	backgroundHTML := renderLocalRedirectBackground(options)
	customStyle := renderLocalRedirectCustomStyle(options)
	warningsHTML := renderLocalRedirectWarnings(renderWarnings)

	sourceJS, err := marshalRedirectJSValue("source url", sourceURL)
	if err != nil {
		return nil, err
	}
	localJS, err := marshalRedirectJSValue("local url", localURL)
	if err != nil {
		return nil, err
	}
	checkingJS, err := marshalRedirectJSValue("checking text", texts.Checking)
	if err != nil {
		return nil, err
	}
	successJS, err := marshalRedirectJSValue("success text", texts.Success)
	if err != nil {
		return nil, err
	}
	failedPrefixJS, err := marshalRedirectJSValue("failed prefix text", texts.FailedPrefix)
	if err != nil {
		return nil, err
	}
	failedSuffixJS, err := marshalRedirectJSValue("failed suffix text", texts.FailedSuffix)
	if err != nil {
		return nil, err
	}

	page := `<!doctype html><html lang="` + texts.Lang + `"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><title>` + template.HTMLEscapeString(texts.Title) + `</title><style>*{margin:0;padding:0;box-sizing:border-box;}body{--color-background:#2d3436;--color-primary:#effbff;--color-accent:#ffa500;--spacing-ui:10px;min-height:100vh;background:var(--color-background);color:var(--color-primary);font-family:Roboto,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;font-size:14px;}a{text-decoration:none;color:var(--color-primary);}.page-background{position:fixed;inset:0;z-index:-2;pointer-events:none;overflow:hidden;}.page-background img{position:absolute;inset:0;width:100%;height:100%;object-fit:cover;object-position:center center;transform:scale(1.08);will-change:auto;}.page-background-preview{opacity:1;}.page-background-full{opacity:0;}.page-background.is-loaded .page-background-full{opacity:1;}.page-background.is-loaded .page-background-preview{opacity:0;}.page-background.is-failed .page-background-full{opacity:0;}.pageview{min-height:100vh;display:flex;align-items:center;justify-content:center;padding:40px 20px;position:relative;overflow:hidden;}.local-redirect-shell{width:min(680px,100%);display:grid;gap:14px;}.local-redirect-warnings{display:grid;gap:10px;}.local-redirect-warning{padding:12px 14px;border:1px solid color-mix(in srgb,var(--color-accent) 36%,transparent);border-radius:6px;background:color-mix(in srgb,var(--color-background) 78%,var(--color-accent) 22%);color:var(--color-primary);line-height:1.5;box-shadow:inset 0 0 0 1px color-mix(in srgb,var(--color-primary) 8%,transparent);}.local-redirect-panel{width:100%;border:1px solid color-mix(in srgb,var(--color-primary) 18%,transparent);background:color-mix(in srgb,var(--color-background) 82%,transparent);border-radius:8px;padding:28px;box-shadow:0 24px 70px rgba(0,0,0,.22);}.local-redirect-eyebrow{color:var(--color-accent);font-weight:700;letter-spacing:0;text-transform:uppercase;font-size:12px;margin-bottom:10px;}.local-redirect-panel h1{font-size:30px;line-height:1.2;margin-bottom:12px;color:var(--color-primary);}.local-redirect-lead{line-height:1.8;opacity:.82;margin-bottom:22px;}.local-status{display:flex;align-items:center;gap:12px;padding:13px 14px;border-radius:8px;background:rgba(0,0,0,.16);margin-bottom:18px;}.local-status-dot{width:10px;height:10px;border-radius:50%;background:var(--color-accent);box-shadow:0 0 0 0 color-mix(in srgb,var(--color-accent) 45%,transparent);animation:pulse 1.3s infinite;flex:0 0 auto;}.local-status.done .local-status-dot{animation:none;}.local-countdown{margin-left:auto;display:flex;align-items:center;gap:5px;color:var(--color-accent);font-weight:700;}.local-countdown[hidden]{display:none;}.local-countdown strong{min-width:18px;text-align:center;}.address-list{display:grid;gap:10px;margin:18px 0 22px;}.address-row{border-left:3px solid var(--color-accent);background:rgba(0,0,0,.12);border-radius:4px;padding:10px 12px;min-width:0;}.address-row span{display:block;color:var(--color-accent);font-size:12px;margin-bottom:5px;}.address-row code{display:block;color:var(--color-primary);font-family:Consolas,"SFMono-Regular",monospace;font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;}.actions{display:flex;gap:12px;flex-wrap:wrap;}.btn{display:inline-flex;align-items:center;justify-content:center;min-height:38px;padding:9px 14px;border-radius:4px;border:1px solid var(--color-accent);color:var(--color-primary);transition:background-color .15s,color .15s,opacity .15s;}.btn-primary{background:var(--color-accent);color:var(--color-background);font-weight:700;}.btn:hover{background:var(--color-primary);color:var(--color-background);}.btn-primary:hover{opacity:.86;}noscript p{margin-top:18px;line-height:1.7;color:var(--color-accent);}@keyframes pulse{0%{box-shadow:0 0 0 0 color-mix(in srgb,var(--color-accent) 45%,transparent);}70%{box-shadow:0 0 0 10px transparent;}100%{box-shadow:0 0 0 0 transparent;}}@supports not (color:color-mix(in srgb,#fff 50%,transparent)){.local-redirect-panel{border-color:rgba(255,255,255,.16);background:rgba(0,0,0,.22);}.local-status-dot{box-shadow:none;}}@media (max-width:520px){.pageview{padding:20px 14px;align-items:flex-start;}.local-redirect-panel{padding:22px 18px;}.local-redirect-panel h1{font-size:24px;}.local-status{align-items:flex-start;}.local-countdown{margin-left:0;}.actions{display:grid;grid-template-columns:1fr;width:100%;}.btn{width:100%;}.address-row code{white-space:normal;word-break:break-all;}}</style>` + customStyle + `</head><body style="` + bodyStyleValue + `">` + backgroundHTML + `<main class="pageview"><div class="local-redirect-shell">` + warningsHTML + `<section class="local-redirect-panel"><p class="local-redirect-eyebrow">` + template.HTMLEscapeString(texts.Eyebrow) + `</p><h1>` + template.HTMLEscapeString(texts.Heading) + `</h1><p class="local-redirect-lead">` + template.HTMLEscapeString(texts.Lead) + `</p><div class="local-status" id="status-box"><span class="local-status-dot" aria-hidden="true"></span><p id="status-text">` + template.HTMLEscapeString(texts.Checking) + `</p><span class="local-countdown" id="countdown-wrap" hidden><strong id="countdown">3</strong><span>` + template.HTMLEscapeString(texts.CountdownUnit) + `</span></span></div><div class="address-list"><div class="address-row"><span>` + template.HTMLEscapeString(texts.LocalLabel) + `</span><code>` + localHTML + `</code></div><div class="address-row"><span>` + template.HTMLEscapeString(texts.SourceLabel) + `</span><code>` + sourceHTML + `</code></div></div><div class="actions"><a class="btn btn-primary" href="` + localHTML + `" rel="noopener">` + template.HTMLEscapeString(texts.LocalButton) + `</a><a class="btn" href="` + sourceHTML + `" rel="noopener">` + template.HTMLEscapeString(texts.SourceButton) + `</a></div><noscript><p>` + template.HTMLEscapeString(texts.Noscript) + `</p></noscript></section></div></main><script>` + background.InlineLoaderScript + `</script><script>(function(){var localURL=` + string(localJS) + `;var sourceURL=` + string(sourceJS) + `;var checkingText=` + string(checkingJS) + `;var successText=` + string(successJS) + `;var failedPrefix=` + string(failedPrefixJS) + `;var failedSuffix=` + string(failedSuffixJS) + `;var statusBox=document.getElementById("status-box");var statusText=document.getElementById("status-text");var countdownWrap=document.getElementById("countdown-wrap");var countdown=document.getElementById("countdown");var done=false;var probeTimer=window.setTimeout(startFallbackCountdown,2500);function setStatus(text){if(statusText){statusText.textContent=text;}}function failureMessage(seconds){return failedPrefix+seconds+failedSuffix;}function startFallbackCountdown(){if(done){return;}done=true;window.clearTimeout(probeTimer);var seconds=3;if(statusBox){statusBox.classList.add("done");}if(countdownWrap){countdownWrap.hidden=false;}function render(){if(countdown){countdown.textContent=String(seconds);}setStatus(failureMessage(seconds));}render();var interval=window.setInterval(function(){seconds-=1;if(seconds<=0){window.clearInterval(interval);window.location.replace(sourceURL);return;}render();},1000);}function openLocal(){if(done){return;}done=true;window.clearTimeout(probeTimer);if(statusBox){statusBox.classList.add("done");}setStatus(successText);window.location.replace(localURL);}setStatus(checkingText);try{fetch(localURL,{method:"GET",mode:"no-cors",cache:"no-store"}).then(openLocal).catch(startFallbackCountdown);}catch(e){startFallbackCountdown();}}());</script></body></html>`
	return []byte(page), nil
}

func marshalRedirectJSValue(label string, value string) ([]byte, error) {
	if !utf8.ValidString(value) {
		return nil, fmt.Errorf("marshal redirect %s failed: input contains invalid UTF-8", label)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal redirect %s failed: %w", label, err)
	}
	return raw, nil
}

func renderLocalRedirectWarnings(warnings []string) string {
	if len(warnings) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="local-redirect-warnings">`)
	for _, item := range warnings {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		b.WriteString(`<div class="local-redirect-warning">`)
		b.WriteString(template.HTMLEscapeString(item))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func renderLocalRedirectBackground(options model.Application) string {
	assets := background.ResolveAssets(options)
	if !assets.Enabled {
		return ""
	}
	return `<div class="page-background" aria-hidden="true"><img class="page-background-full" src="` + template.HTMLEscapeString(assets.FullURL) + `" alt="" loading="eager" decoding="async"></div>`
}

func renderLocalRedirectCustomStyle(options model.Application) string {
	var b strings.Builder
	assets := background.ResolveAssets(options)
	if assets.Enabled {
		opacity := options.BackgroundOpacity
		if opacity < 0 {
			opacity = 0
		}
		if opacity > 100 {
			opacity = 100
		}
		blur := options.BackgroundBlur
		if blur < 0 {
			blur = 0
		}
		b.WriteString(`<style>.page-background img{filter:blur(`)
		b.WriteString(strconv.Itoa(blur))
		b.WriteString(`px);}.page-background.is-loaded .page-background-full{opacity:`)
		b.WriteString(strconv.FormatFloat(float64(opacity)/100, 'f', 2, 64))
		b.WriteString(`;}</style>`)
	}
	if options.GlassEffect != "" && options.GlassEffect != "none" && options.GlassIntensity > 0 {
		intensity := options.GlassIntensity
		if intensity > 100 {
			intensity = 100
		}
		blur := 4 + intensity/5
		alpha := 0.04 + float64(intensity)/700
		borderAlpha := 0.08 + float64(intensity)/500
		b.WriteString(`<style>.local-redirect-panel{background:rgba(255,255,255,`)
		b.WriteString(strconv.FormatFloat(alpha, 'f', 3, 64))
		b.WriteString(`);backdrop-filter:blur(`)
		b.WriteString(strconv.Itoa(blur))
		b.WriteString(`px);-webkit-backdrop-filter:blur(`)
		b.WriteString(strconv.Itoa(blur))
		b.WriteString(`px);border-color:rgba(255,255,255,`)
		b.WriteString(strconv.FormatFloat(borderAlpha, 'f', 3, 64))
		b.WriteString(`);}`)
		if options.GlassEffect == "liquid" {
			b.WriteString(`.local-redirect-panel{box-shadow:inset 0 1px 0 rgba(255,255,255,.16),0 24px 70px rgba(0,0,0,.24);}`)
		}
		b.WriteString(`</style>`)
	}
	return b.String()
}

func cssURLValue(input string) string {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `'`, `\'`)
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	return input
}
