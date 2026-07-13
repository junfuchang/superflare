package ports

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/footer"
	"github.com/junfuchang/superflare/internal/pool"
	portscollector "github.com/junfuchang/superflare/internal/ports"
	settingsroot "github.com/junfuchang/superflare/internal/settings"
	"github.com/junfuchang/superflare/internal/statuspage"
)

var portscollectorCollectReportWithBindingsErr = portscollector.CollectReportWithBindingsErr

func RegisterRouting(e *echo.Echo) {
	e.GET(define.SettingPages.Ports.Path, pagePorts, auth.AuthRequired)
	e.GET(portsDataPath(), pagePortsData, auth.AuthRequired)
	e.POST(define.SettingPages.Ports.Path, updatePortRemarks, auth.AuthRequired)
}

func rejectWhenLoginDisabled(c *echo.Context) error {
	if auth.IsLoginDisabled(c) {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return nil
}

func portsFailurePayload(message string, err error) map[string]any {
	payload := map[string]any{
		"ok":    false,
		"error": message,
	}
	if err != nil {
		payload["detail"] = err.Error()
	}
	return payload
}

type portsDataPayload struct {
	OK       bool             `json:"ok"`
	Items    []model.PortInfo `json:"items"`
	Warnings []string         `json:"warnings,omitempty"`
}

func ensureSettingsConfigLoadable(c *echo.Context) error {
	if _, err := data.GetAllSettingsOptions(); err != nil {
		return c.JSON(http.StatusInternalServerError, portsFailurePayload("settings config error", err))
	}
	return nil
}

func updatePortRemarks(c *echo.Context) error {
	if err := rejectWhenLoginDisabled(c); err != nil {
		return err
	}
	var body struct {
		Ports         string `form:"ports"`
		IncludeHidden string `form:"includeHidden"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, portsFailurePayload("missing form data", err))
	}
	if strings.TrimSpace(body.Ports) == "" {
		body.Ports = c.FormValue("ports")
	}
	if strings.TrimSpace(body.Ports) == "" {
		return c.JSON(http.StatusBadRequest, portsFailurePayload("missing ports payload", nil))
	}
	bindings, err := parsePortBindings(body.Ports)
	if err != nil {
		return c.JSON(http.StatusBadRequest, portsFailurePayload("parse ports payload failed", err))
	}
	if err := ensureSettingsConfigLoadable(c); err != nil {
		return err
	}
	if !parseBoolValue(body.IncludeHidden) {
		bindings, err = keepExistingHiddenBindings(bindings)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, portsFailurePayload("ports config error", err))
		}
	}
	if err := data.UpdatePortRemarks(bindings); err != nil {
		return c.JSON(http.StatusInternalServerError, portsFailurePayload("save ports failed", err))
	}
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
	for index, row := range rows {
		remark := strings.TrimSpace(row.Remark)
		hidden := parseBoolValue(row.Hidden)
		if remark == "" {
			if !hidden {
				continue
			}
		}
		port, err := parsePortValue(row.Port)
		if err != nil {
			return nil, fmt.Errorf("invalid port value at row %d: %w", index+1, err)
		}
		if port <= 0 || port > 65535 {
			return nil, fmt.Errorf("port out of range at row %d: %d", index+1, port)
		}
		protocol, err := parsePortProtocol(row.Protocol)
		if err != nil {
			return nil, fmt.Errorf("invalid protocol at row %d: %w", index+1, err)
		}
		result = append(result, model.PortBinding{Port: port, Protocol: protocol, Remark: remark, Hidden: hidden})
	}
	return result, nil
}

func parsePortValue(value any) (int, error) {
	switch v := value.(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("port must be an integer")
		}
		return int(v), nil
	case string:
		raw := strings.TrimSpace(v)
		if raw == "" {
			return 0, fmt.Errorf("port is empty")
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("port %q is not a valid integer", raw)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unsupported port type %T", value)
	}
}

func parsePortProtocol(value string) (string, error) {
	protocol := strings.ToLower(strings.TrimSpace(value))
	switch protocol {
	case "", "tcp":
		return "tcp", nil
	case "udp":
		return "udp", nil
	default:
		return "", fmt.Errorf("%q is not supported", value)
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

func keepExistingHiddenBindings(items []model.PortBinding) ([]model.PortBinding, error) {
	current, err := data.LoadPortBindings()
	if err != nil {
		return nil, err
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
	return result, nil
}

func portBindingKey(protocol string, port int) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol != "udp" {
		protocol = "tcp"
	}
	return protocol + ":" + strconv.Itoa(port)
}

func pagePortsData(c *echo.Context) error {
	if err := rejectWhenLoginDisabled(c); err != nil {
		return err
	}
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, portsFailurePayload("settings config error", err))
	}
	locale := options.Locale
	includeHidden := parseBoolValue(c.QueryParam("includeHidden"))
	bindings, err := data.GetPortBindingMapWithError()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, portsFailurePayload("ports config error", err))
	}
	report, err := portscollectorCollectReportWithBindingsErr(bindings, includeHidden)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, portsFailurePayload("ports runtime collect error", err))
	}
	return c.JSON(http.StatusOK, portsDataPayload{
		OK:       true,
		Items:    report.Items,
		Warnings: formatPortCollectionWarnings(locale, report.Warnings),
	})
}

func pagePorts(c *echo.Context) error {
	if err := rejectWhenLoginDisabled(c); err != nil {
		return err
	}
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareSettingsOptionsForRender(options)
	if _, err := data.LoadPortBindings(); err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(options.Locale, http.StatusInternalServerError, err.Error()))
	}
	locale := options.Locale
	showLoginInfo := false
	userName := ""
	loginDate := ""
	loginDisplay, err := auth.ResolveLoginDisplayStateForStrictView(c)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	showLoginInfo = loginDisplay.ShowLoginInfo
	userName = loginDisplay.UserName
	loginDate = loginDisplay.LoginDate
	renderWarnings = auth.AppendSessionWarnings(c, locale, renderWarnings)
	pageStyle, styleWarning, err := statuspage.RequireConfiguredBodyStyleForRender(locale, "settings")
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	if styleWarning != "" {
		renderWarnings = append(renderWarnings, styleWarning)
	}
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["DebugMode"] = settingsroot.CurrentRuntime().DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageName"] = "Ports"
	m["PageAppearance"] = pageStyle
	m["SettingPages"] = define.SettingPages
	m["ShowSettingsSidebar"] = true
	m["ShowPortsSettings"] = true
	m["DisableLoginMode"] = false
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["ShowLoginInfo"] = showLoginInfo
	m["UserIsLogin"] = showLoginInfo
	m["UserName"] = userName
	m["LoginDate"] = loginDate
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	footer.BindTemplateData(m, options.Footer)
	m["DebugAssetVersion"] = getDebugAssetVersion()
	m["RenderWarnings"] = renderWarnings
	m["PortsData"] = template.HTML("[]")
	m["PortsDataURI"] = portsDataPath()
	return c.Render(http.StatusOK, "settings.html", m)
}

func portsDataPath() string {
	return define.SettingPages.Ports.Path + "/data"
}

func getDebugAssetVersion() string {
	if settingsroot.CurrentRuntime().DebugMode {
		return "?v=dev"
	}
	return ""
}

func formatPortCollectionWarnings(locale string, warnings []portscollector.CollectionWarning) []string {
	if len(warnings) == 0 {
		return nil
	}
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		text := formatPortCollectionWarning(locale, warning)
		if text == "" {
			continue
		}
		result = append(result, text)
	}
	return result
}

func formatPortCollectionWarning(locale string, warning portscollector.CollectionWarning) string {
	detail := strings.TrimSpace(warning.Detail)
	switch warning.Code {
	case "owner_resolution_partial":
		if locale == "en" {
			message := fmt.Sprintf("Port owner resolution is incomplete: %d of %d running ports are missing a service name or PID. Available results are still shown.", warning.MissingOwners, warning.RuntimePorts)
			if detail != "" {
				message += " Detail: " + detail
			}
			return message
		}
		message := fmt.Sprintf("端口所有者解析不完整：%d 个运行中端口中有 %d 个未能解析出服务名或 PID，当前已展示可用结果。", warning.RuntimePorts, warning.MissingOwners)
		if detail != "" {
			message += " 详细信息：" + detail
		}
		return message
	default:
		return detail
	}
}
