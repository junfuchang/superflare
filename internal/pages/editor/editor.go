package editor

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/soulteary/memfs"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/pool"
	portscollector "github.com/junfuchang/superflare/internal/ports"
)

var MemFs *memfs.FS

const _ASSETS_BASE_DIR = "assets/editor"
const _ASSETS_WEB_URI = "/" + _ASSETS_BASE_DIR
const _ASSETS_TABLE_URI = "/assets/table"

//go:embed editor-assets
var editorAssets embed.FS

func Init() error {
	MemFs = memfs.New()
	err := MemFs.MkdirAll(_ASSETS_BASE_DIR, 0777)
	if err != nil {
		return err
	}
	return nil
}

func RegisterRouting(e *echo.Echo) {
	RegisterAssetRouting(e)
	e.GET(define.RegularPages.Editor.Path, render, auth.AuthRequired)
	e.POST(define.RegularPages.Editor.Path, updateData, auth.AuthRequired)
	e.GET(define.RegularPages.Editor.Path+"/backup", backupData, auth.AuthRequired)
	e.POST(define.RegularPages.Editor.Path+"/restore", restoreData, auth.AuthRequired)
	e.POST(define.RegularPages.Editor.Path+"/check-links", checkLinks, auth.AuthRequired)
}

func RegisterAssetRouting(e *echo.Echo) {
	var assetFS fs.FS
	if define.AppFlags.DebugMode && registerLocalVendorAssets(e) {
		assetFS = os.DirFS("embed/assets/vendor/editor-assets")
		// Local development can run without copying generated assets first.
	} else if introAssets, err := fs.Sub(editorAssets, "editor-assets"); err == nil {
		assetFS = introAssets
		e.StaticFS(_ASSETS_WEB_URI, introAssets)
	}
	if assetFS != nil {
		registerEditorAssetAliases(e, assetFS)
	}
}

func registerLocalVendorAssets(e *echo.Echo) bool {
	const vendorAssets = "embed/assets/vendor/editor-assets"
	if stat, err := os.Stat(vendorAssets); err == nil && stat.IsDir() {
		e.Static(_ASSETS_WEB_URI, vendorAssets)
		return true
	}
	return false
}

func registerEditorAssetAliases(e *echo.Echo, assetFS fs.FS) {
	e.GET(_ASSETS_TABLE_URI+"/grid.css", serveEditorAsset(assetFS, "handsontable.full.min.css", "text/css; charset=utf-8"))
	e.GET(_ASSETS_TABLE_URI+"/grid.js", serveEditorAsset(assetFS, "handsontable.full.min.js", "text/javascript; charset=utf-8"))
	e.GET(_ASSETS_TABLE_URI+"/grid.zh-CN.js", serveEditorAsset(assetFS, "zh-CN.min.js", "text/javascript; charset=utf-8"))
}

func serveEditorAsset(assetFS fs.FS, name string, contentType string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		raw, err := fs.ReadFile(assetFS, name)
		if err != nil {
			return c.NoContent(http.StatusNotFound)
		}
		return c.Blob(http.StatusOK, contentType, raw)
	}
}

func getDebugAssetVersion() string {
	if define.AppFlags.DebugMode {
		return "?v=dev"
	}
	return ""
}

func updateData(c *echo.Context) error {
	var body struct {
		Categories string `form:"categories"`
		Bookmarks  string `form:"bookmarks"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusForbidden, "提交数据缺失")
	}
	data.UpdateBookmarksFromEditor(body.Categories, body.Bookmarks)
	return render(c)
}

func backupData(c *echo.Context) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"config", "bookmarks", "apps", "ports"} {
		path := restoreConfigPath(name)
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		w, err := zw.Create(restoreConfigFileName(name))
		if err != nil {
			_ = zw.Close()
			return c.String(http.StatusInternalServerError, "backup error")
		}
		if _, err := w.Write(raw); err != nil {
			_ = zw.Close()
			return c.String(http.StatusInternalServerError, "backup error")
		}
	}
	if err := zw.Close(); err != nil {
		return c.String(http.StatusInternalServerError, "backup error")
	}
	filename := "superflare-backup-" + time.Now().Format("20060102-150405") + ".zip"
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+filename+`"`)
	return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
}

func restoreData(c *echo.Context) error {
	file, err := c.FormFile("backup")
	if err != nil {
		return c.String(http.StatusBadRequest, "请选择备份文件")
	}
	src, err := file.Open()
	if err != nil {
		return c.String(http.StatusBadRequest, "读取备份文件失败")
	}
	defer src.Close()
	raw, err := io.ReadAll(src)
	if err != nil {
		return c.String(http.StatusBadRequest, "读取备份文件失败")
	}

	if strings.HasSuffix(strings.ToLower(file.Filename), ".zip") {
		if err := restoreZip(raw); err != nil {
			return c.String(http.StatusBadRequest, err.Error())
		}
		return c.Redirect(http.StatusFound, define.RegularPages.Editor.Path)
	}

	name := normalizeRestoreFileName(file.Filename)
	if !isRestoreConfigName(name) {
		return c.String(http.StatusBadRequest, "仅支持 config/bookmarks/apps/ports 的 yml/yaml 文件或 zip 备份")
	}
	if err := os.WriteFile(restoreConfigPath(name), raw, os.ModePerm); err != nil {
		return c.String(http.StatusInternalServerError, "恢复备份失败")
	}
	data.InvalidateConfigCache(name)
	define.UpdatePagePalettes()
	return c.Redirect(http.StatusFound, define.RegularPages.Editor.Path)
}

func restoreZip(raw []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	restored := false
	for _, f := range zr.File {
		name := strings.ToLower(strings.TrimPrefix(f.Name, "/"))
		name = strings.TrimSuffix(name, ".yml")
		name = strings.TrimSuffix(name, ".yaml")
		if strings.Contains(name, "/") || strings.Contains(name, `\`) || !isRestoreConfigName(name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		rawFile, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(restoreConfigPath(name), rawFile, os.ModePerm); err != nil {
			return err
		}
		data.InvalidateConfigCache(name)
		restored = true
	}
	if !restored {
		return echo.NewHTTPError(http.StatusBadRequest, "备份中没有可恢复的数据文件")
	}
	define.UpdatePagePalettes()
	return nil
}

func normalizeRestoreFileName(filename string) string {
	name := strings.ToLower(filename)
	name = strings.TrimSuffix(name, ".yml")
	name = strings.TrimSuffix(name, ".yaml")
	return name
}

func isRestoreConfigName(name string) bool {
	return name == "config" || name == "bookmarks" || name == "apps" || name == "ports"
}

func restoreConfigPath(name string) string {
	if name == "ports" {
		return data.GetPortsConfigPath()
	}
	return data.GetConfigPath(name)
}

func restoreConfigFileName(name string) string {
	if name == "ports" {
		return "ports.yaml"
	}
	return name + ".yml"
}

type linkCheckItem struct {
	Row int    `json:"row"`
	URL string `json:"url"`
}

type linkCheckResult struct {
	Row    int    `json:"row"`
	URL    string `json:"url"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func checkLinks(c *echo.Context) error {
	var body struct {
		Bookmarks string `json:"bookmarks" form:"bookmarks"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "提交数据缺失"})
	}
	items, err := parseLinksForCheck(body.Bookmarks)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "书签数据格式错误"})
	}
	results := runLinkChecks(items)
	return c.JSON(http.StatusOK, results)
}

func parseLinksForCheck(input string) ([]linkCheckItem, error) {
	if input == "" {
		return nil, nil
	}
	reader := csv.NewReader(strings.NewReader(input))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	items := make([]linkCheckItem, 0, len(records))
	for _, record := range records {
		if len(record) < 3 {
			continue
		}
		row := 0
		_, _ = fmt.Sscanf(record[0], "%d", &row)
		rawURL := strings.TrimSpace(record[2])
		if !shouldCheckURL(rawURL) {
			continue
		}
		items = append(items, linkCheckItem{Row: row, URL: rawURL})
	}
	return items, nil
}

func shouldCheckURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return false
		}
	}
	return true
}

func runLinkChecks(items []linkCheckItem) []linkCheckResult {
	results := make([]linkCheckResult, 0)
	resultsMu := sync.Mutex{}
	client := &http.Client{Timeout: 5 * time.Second}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, item := range items {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result := checkOneLink(client, item)
			if result.Status != "ok" {
				resultsMu.Lock()
				results = append(results, result)
				resultsMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return results
}

func checkOneLink(client *http.Client, item linkCheckItem) linkCheckResult {
	req, err := http.NewRequest(http.MethodHead, item.URL, nil)
	if err != nil {
		return linkCheckResult{Row: item.Row, URL: item.URL, Status: "invalid", Reason: err.Error()}
	}
	req.Header.Set("User-Agent", "SuperFlare-Link-Checker")
	resp, err := client.Do(req)
	if err != nil {
		req, err = http.NewRequest(http.MethodGet, item.URL, nil)
		if err != nil {
			return linkCheckResult{Row: item.Row, URL: item.URL, Status: "invalid", Reason: err.Error()}
		}
		req.Header.Set("User-Agent", "SuperFlare-Link-Checker")
		resp, err = client.Do(req)
	}
	if err != nil {
		return linkCheckResult{Row: item.Row, URL: item.URL, Status: "invalid", Reason: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return linkCheckResult{Row: item.Row, URL: item.URL, Status: "invalid", Reason: resp.Status}
	}
	return linkCheckResult{Row: item.Row, URL: item.URL, Status: "ok"}
}

func render(c *echo.Context) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return c.String(http.StatusInternalServerError, "config error")
	}
	dataCategories, dataBookmarks := data.GetBookmarksForEditor()
	portsConfig, err := data.LoadPortBindings()
	if err != nil {
		portsConfig = model.Ports{}
	}
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["PageName"] = "Editor"
	m["PageAppearance"] = define.GetAppBodyStyle()
	m["SettingPages"] = define.SettingPages
	m["DebugMode"] = define.AppFlags.DebugMode
	m["DebugAssetVersion"] = getDebugAssetVersion()
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["DataCategories"] = template.HTML(dataCategories)
	m["DataBookmarks"] = template.HTML(dataBookmarks)
	m["DataPorts"] = template.HTML(marshalEditorPorts(portsConfig.Items))
	m["LocalLANHost"] = portscollector.LocalLANHost()
	m["OptionTitle"] = options.Title
	m["OptionFooter"] = template.HTML(options.Footer)
	m["OptionOpenAppNewTab"] = options.OpenAppNewTab
	m["OptionOpenBookmarkNewTab"] = options.OpenBookmarkNewTab
	m["OptionShowTitle"] = options.ShowTitle
	m["OptionShowDateTime"] = options.ShowDateTime
	m["OptionShowApps"] = options.ShowApps
	m["OptionShowBookmarks"] = options.ShowBookmarks
	return c.Render(http.StatusOK, "editor.html", m)
}

func marshalEditorPorts(items []model.PortBinding) string {
	type editorPort struct {
		Port     int    `json:"Port"`
		Protocol string `json:"Protocol"`
		Remark   string `json:"Remark"`
	}
	result := make([]editorPort, 0)
	for _, item := range items {
		if item.Port <= 0 || item.Hidden || strings.ToLower(strings.TrimSpace(item.Protocol)) == "udp" || strings.TrimSpace(item.Remark) == "" {
			continue
		}
		result = append(result, editorPort{Port: item.Port, Protocol: item.Protocol, Remark: strings.TrimSpace(item.Remark)})
	}
	raw, _ := json.Marshal(result)
	return string(raw)
}
