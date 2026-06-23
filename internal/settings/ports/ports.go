package ports

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/pool"
	portscollector "github.com/junfuchang/superflare/internal/ports"
)

func RegisterRouting(e *echo.Echo) {
	e.GET(define.SettingPages.Ports.Path, pagePorts, auth.AuthRequired)
	e.GET(portsDataPath(), pagePortsData, auth.AuthRequired)
	e.POST(define.SettingPages.Ports.Path, updatePortRemarks, auth.AuthRequired)
}

func updatePortRemarks(c *echo.Context) error {
	var body struct {
		Ports         string `form:"ports"`
		IncludeHidden string `form:"includeHidden"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusForbidden, "missing form data")
	}
	if strings.TrimSpace(body.Ports) == "" {
		body.Ports = c.FormValue("ports")
	}
	bindings, err := parsePortBindings(body.Ports)
	if err != nil {
		return c.String(http.StatusBadRequest, "invalid ports data")
	}
	if !parseBoolValue(body.IncludeHidden) {
		bindings = keepExistingHiddenBindings(bindings)
	}
	data.UpdatePortRemarks(bindings)
	return c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

func parsePortBindings(raw string) ([]model.PortBinding, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var rows []struct {
		Port     any    `json:"Port"`
		Protocol string `json:"Protocol"`
		Remark   string `json:"Remark"`
		Hidden   any    `json:"Hidden"`
	}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, err
	}
	result := make([]model.PortBinding, 0, len(rows))
	for _, row := range rows {
		remark := strings.TrimSpace(row.Remark)
		if remark == "" {
			hidden := parseBoolValue(row.Hidden)
			if !hidden {
				continue
			}
		}
		port := parsePortValue(row.Port)
		if port <= 0 || port > 65535 {
			continue
		}
		protocol := strings.ToLower(strings.TrimSpace(row.Protocol))
		if protocol != "udp" {
			protocol = "tcp"
		}
		result = append(result, model.PortBinding{Port: port, Protocol: protocol, Remark: remark, Hidden: parseBoolValue(row.Hidden)})
	}
	return result, nil
}

func parsePortValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func parseBoolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "true" || v == "1" || v == "yes" || v == "on"
	case float64:
		return v != 0
	default:
		return false
	}
}

func keepExistingHiddenBindings(items []model.PortBinding) []model.PortBinding {
	current, err := data.LoadPortBindings()
	if err != nil {
		return items
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[portBindingKey(item.Protocol, item.Port)] = true
	}
	result := make([]model.PortBinding, 0, len(items)+len(current.Items))
	result = append(result, items...)
	for _, item := range current.Items {
		if !item.Hidden {
			continue
		}
		key := portBindingKey(item.Protocol, item.Port)
		if seen[key] {
			continue
		}
		result = append(result, item)
	}
	return result
}

func portBindingKey(protocol string, port int) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol != "udp" {
		protocol = "tcp"
	}
	return protocol + ":" + strconv.Itoa(port)
}

func pagePortsData(c *echo.Context) error {
	includeHidden := parseBoolValue(c.QueryParam("includeHidden"))
	ports := portscollector.CollectWithHidden(includeHidden)
	return c.JSON(http.StatusOK, ports)
}

func pagePorts(c *echo.Context) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return c.String(http.StatusInternalServerError, "config error")
	}
	locale := options.Locale
	if locale == "" {
		locale = "zh"
	}
	showLoginInfo := false
	if !define.AppFlags.DisableLoginMode {
		showLoginInfo = auth.CheckUserIsLogin(c)
	}
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["DebugMode"] = define.AppFlags.DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageName"] = "Ports"
	m["PageAppearance"] = define.GetAppBodyStyle()
	m["SettingPages"] = define.SettingPages
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["ShowLoginInfo"] = showLoginInfo
	m["UserIsLogin"] = showLoginInfo
	m["UserName"] = auth.GetUserName(c)
	m["LoginDate"] = auth.GetUserLoginDate(c)
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	m["OptionFooter"] = template.HTML(options.Footer)
	m["DebugAssetVersion"] = getDebugAssetVersion()
	m["PortsData"] = template.HTML("[]")
	m["PortsDataURI"] = portsDataPath()
	return c.Render(http.StatusOK, "settings.html", m)
}

func portsDataPath() string {
	return define.SettingPages.Ports.Path + "/data"
}

func getDebugAssetVersion() string {
	if define.AppFlags.DebugMode {
		return "?v=dev"
	}
	return ""
}
