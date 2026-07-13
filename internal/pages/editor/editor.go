package editor

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/soulteary/memfs"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/footer"
	"github.com/junfuchang/superflare/internal/pool"
	portscollector "github.com/junfuchang/superflare/internal/ports"
	"github.com/junfuchang/superflare/internal/statuspage"
)

var MemFs *memfs.FS
var marshalEditorPortsJSON = json.Marshal
var resolveRestoreConfigPath = restoreConfigPathErr
var refreshPagePaletteCache = define.UpdatePagePalettes
var syncRestoreTempFile = func(file *os.File) error { return file.Sync() }

const _ASSETS_BASE_DIR = "assets/editor"
const _ASSETS_WEB_URI = "/" + _ASSETS_BASE_DIR
const _ASSETS_TABLE_URI = "/assets/table"
const linkCheckWorkerCount = 8
const linkCheckRequestTimeout = 2 * time.Second
const linkCheckOverallTimeout = 4 * time.Second
const linkCheckRetryDelay = 120 * time.Millisecond
const restoreUploadMaxBytes = 16 * 1024 * 1024
const restoreZipEntryMaxBytes = 8 * 1024 * 1024
const restoreZipMaxEntries = 256
const editorNoticeQueryKey = "notice"
const editorNoticeSaveSuccess = "save_success"
const editorNoticeRestoreSuccess = "restore_success"

type editorRuntimeSnapshot struct {
	DebugMode bool
}

type editorRuntimeHolder struct {
	mu  sync.RWMutex
	set bool
	cfg editorRuntimeSnapshot
}

func (h *editorRuntimeHolder) Load() editorRuntimeSnapshot {
	if h == nil {
		return editorRuntimeSnapshot{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.set {
		return editorRuntimeSnapshot{}
	}
	return h.cfg
}

func (h *editorRuntimeHolder) Store(cfg editorRuntimeSnapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.set = true
	h.cfg = cfg
	h.mu.Unlock()
}

var editorRuntimeFlags = &editorRuntimeHolder{}

func editorRuntimeSnapshotFromFlags(flags model.Flags) editorRuntimeSnapshot {
	return editorRuntimeSnapshot{DebugMode: flags.DebugMode}
}

func currentEditorRuntime() editorRuntimeSnapshot {
	editorRuntimeFlags.mu.RLock()
	hasValue := editorRuntimeFlags.set
	cfg := editorRuntimeFlags.cfg
	editorRuntimeFlags.mu.RUnlock()
	if hasValue {
		return cfg
	}
	cfg = editorRuntimeSnapshotFromFlags(define.CurrentAppRuntimeFlags())
	editorRuntimeFlags.Store(cfg)
	return cfg
}

func SetRuntimeFlags(flags model.Flags) {
	editorRuntimeFlags.Store(editorRuntimeSnapshotFromFlags(flags))
}

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
	if currentEditorRuntime().DebugMode && registerLocalVendorAssets(e) {
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
	e.GET(_ASSETS_TABLE_URI+"/runtime.js", serveEditorAsset(assetFS, "regenerator-runtime.js", "text/javascript; charset=utf-8"))
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
	if currentEditorRuntime().DebugMode {
		return "?v=dev"
	}
	return ""
}

func renderEditorErrorPage(c *echo.Context, status int, err error) error {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	if bindErr := statuspage.BindCurrentOptions(c); bindErr != nil {
		detail := "settings config error: " + strings.TrimSpace(bindErr.Error())
		switch {
		case message == "":
			message = detail
		case !strings.Contains(message, detail) && !strings.Contains(message, strings.TrimSpace(bindErr.Error())):
			message = message + "; " + detail
		}
	}
	return statuspage.HTML(c, status, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), status, message))
}

func editorJSONError(message string) map[string]string {
	return map[string]string{"error": message}
}

func editorWantsJSON(c *echo.Context) bool {
	accept := strings.ToLower((*c).Request().Header.Get(echo.HeaderAccept))
	return strings.Contains(accept, echo.MIMEApplicationJSON)
}

func buildEditorNoticeRedirectURL(notice string) string {
	if strings.TrimSpace(notice) == "" {
		return define.RegularPages.Editor.Path
	}
	return define.RegularPages.Editor.Path + "?" + editorNoticeQueryKey + "=" + url.QueryEscape(notice)
}

func resolveEditorOperationNotice(locale string, code string) map[string]string {
	switch strings.TrimSpace(code) {
	case editorNoticeSaveSuccess:
		if locale == "en" {
			return map[string]string{"Type": "success", "Text": "Bookmark data saved successfully."}
		}
		return map[string]string{"Type": "success", "Text": "书签与分类数据保存成功。"}
	case editorNoticeRestoreSuccess:
		if locale == "en" {
			return map[string]string{"Type": "success", "Text": "Backup restore completed successfully."}
		}
		return map[string]string{"Type": "success", "Text": "备份恢复导入成功。"}
	default:
		return nil
	}
}

func updateData(c *echo.Context) error {
	var body struct {
		Categories string `form:"categories"`
		Bookmarks  string `form:"bookmarks"`
	}
	if err := c.Bind(&body); err != nil {
		if editorWantsJSON(c) {
			return c.JSON(http.StatusBadRequest, editorJSONError("missing form data"))
		}
		return renderEditorErrorPage(c, http.StatusBadRequest, fmt.Errorf("missing form data"))
	}
	if err := data.UpdateBookmarksFromEditor(body.Categories, body.Bookmarks); err != nil {
		if editorWantsJSON(c) {
			return c.JSON(http.StatusBadRequest, editorJSONError(err.Error()))
		}
		return renderEditorErrorPage(c, http.StatusBadRequest, err)
	}
	if editorWantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]string{"notice": editorNoticeSaveSuccess})
	}
	return c.Redirect(http.StatusFound, buildEditorNoticeRedirectURL(editorNoticeSaveSuccess))
}

func backupData(c *echo.Context) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"config", "bookmarks", "apps", "ports"} {
		path, pathErr := resolveRestoreConfigPath(name)
		if pathErr != nil {
			_ = zw.Close()
			return renderEditorErrorPage(c, http.StatusInternalServerError, fmt.Errorf("resolve backup source path failed: %s: %w", name, pathErr))
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				_ = zw.Close()
				return renderEditorErrorPage(c, http.StatusInternalServerError, fmt.Errorf("backup source file is missing: %s", path))
			}
			_ = zw.Close()
			return renderEditorErrorPage(c, http.StatusInternalServerError, fmt.Errorf("read backup source file failed: %s: %w", path, err))
		}
		if len(raw) == 0 {
			_ = zw.Close()
			return renderEditorErrorPage(c, http.StatusInternalServerError, fmt.Errorf("backup source file is empty: %s", path))
		}
		w, err := zw.Create(restoreConfigFileName(name))
		if err != nil {
			_ = zw.Close()
			return renderEditorErrorPage(c, http.StatusInternalServerError, fmt.Errorf("create backup archive entry failed: %w", err))
		}
		if _, err := w.Write(raw); err != nil {
			_ = zw.Close()
			return renderEditorErrorPage(c, http.StatusInternalServerError, fmt.Errorf("write backup archive entry failed: %w", err))
		}
	}
	if err := zw.Close(); err != nil {
		return renderEditorErrorPage(c, http.StatusInternalServerError, fmt.Errorf("finalize backup archive failed: %w", err))
	}
	filename := "superflare-backup-" + time.Now().Format("20060102-150405") + ".zip"
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+filename+`"`)
	return c.Blob(http.StatusOK, "application/zip", buf.Bytes())
}

func restoreData(c *echo.Context) error {
	file, err := c.FormFile("backup")
	if err != nil {
		return renderEditorErrorPage(c, http.StatusBadRequest, fmt.Errorf("missing backup file"))
	}
	src, err := file.Open()
	if err != nil {
		return renderEditorErrorPage(c, http.StatusBadRequest, fmt.Errorf("open backup file failed: %w", err))
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, restoreUploadMaxBytes+1))
	if err != nil {
		return renderEditorErrorPage(c, http.StatusBadRequest, fmt.Errorf("read backup file failed: %w", err))
	}

	if len(raw) > restoreUploadMaxBytes {
		return renderEditorErrorPage(c, http.StatusBadRequest, fmt.Errorf("backup file too large"))
	}
	if strings.HasSuffix(strings.ToLower(file.Filename), ".zip") {
		if err := restoreZip(raw); err != nil {
			return renderEditorErrorPage(c, http.StatusBadRequest, err)
		}
		refreshRequestLoginRuntime(c)
		return c.Redirect(http.StatusFound, buildEditorNoticeRedirectURL(editorNoticeRestoreSuccess))
	}

	name := normalizeRestoreFileName(file.Filename)
	if !isRestoreConfigName(name) {
		return renderEditorErrorPage(c, http.StatusBadRequest, fmt.Errorf("only config/bookmarks/apps/ports yml-yaml files or zip backups are supported"))
	}
	if err := validateRestorePayload(name, raw); err != nil {
		return renderEditorErrorPage(c, http.StatusBadRequest, err)
	}
	if err := writeRestoreFilesAtomically(map[string][]byte{name: raw}); err != nil {
		return renderEditorErrorPage(c, http.StatusInternalServerError, fmt.Errorf("restore backup failed: %w", err))
	}
	refreshRequestLoginRuntime(c)
	return c.Redirect(http.StatusFound, buildEditorNoticeRedirectURL(editorNoticeRestoreSuccess))
}
func restoreZip(raw []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return err
	}
	if len(zr.File) > restoreZipMaxEntries {
		return echo.NewHTTPError(http.StatusBadRequest, "backup zip contains too many files")
	}
	restored := map[string][]byte{}
	totalExpanded := int64(0)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(strings.TrimPrefix(f.Name, "/"))
		name = strings.TrimSuffix(name, ".yml")
		name = strings.TrimSuffix(name, ".yaml")
		if strings.Contains(name, "/") || strings.Contains(name, `\`) || !isRestoreConfigName(name) {
			continue
		}
		if _, exists := restored[name]; exists {
			return echo.NewHTTPError(http.StatusBadRequest, "backup zip contains duplicate restorable files")
		}
		if f.UncompressedSize64 > restoreZipEntryMaxBytes {
			return echo.NewHTTPError(http.StatusBadRequest, "backup zip contains an oversized data file")
		}
		remaining := int64(restoreUploadMaxBytes) - totalExpanded
		if remaining <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "backup zip expands beyond allowed size")
		}
		entryLimit := int64(restoreZipEntryMaxBytes)
		if remaining < entryLimit {
			entryLimit = remaining
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		rawFile, tooLarge, err := readRestoreZipEntry(rc, entryLimit)
		if err != nil {
			return err
		}
		if tooLarge {
			return echo.NewHTTPError(http.StatusBadRequest, "backup zip contains an oversized data file")
		}
		totalExpanded += int64(len(rawFile))
		if err := validateRestorePayload(name, rawFile); err != nil {
			return err
		}
		restored[name] = rawFile
	}
	if len(restored) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "backup does not contain any restorable data files")
	}
	return writeRestoreFilesAtomically(restored)
}

func readRestoreZipEntry(r io.ReadCloser, limit int64) ([]byte, bool, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	closeErr := r.Close()
	if err != nil {
		if closeErr != nil {
			return nil, false, errors.Join(err, closeErr)
		}
		return nil, false, err
	}
	if int64(len(raw)) > limit {
		return nil, true, nil
	}
	if closeErr != nil {
		return nil, false, closeErr
	}
	return raw, false, nil
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

func restoreConfigPathErr(name string) (string, error) {
	if name == "ports" {
		return data.GetPortsConfigPathErr()
	}
	return data.GetConfigPathErr(name)
}

type pendingRestoreFile struct {
	target string
	temp   string
	backup string
	name   string
}

func validateRestorePayload(name string, raw []byte) error {
	switch name {
	case "config":
		if _, err := data.LoadAppConfigFromRaw(raw); err != nil {
			return fmt.Errorf("parse config restore payload failed: %w", err)
		}
	case "bookmarks":
		if _, err := data.LoadNormalBookmarksFromRaw(raw); err != nil {
			return fmt.Errorf("parse bookmarks restore payload failed: %w", err)
		}
	case "apps":
		if _, err := data.LoadFavoriteBookmarksFromRaw(raw); err != nil {
			return fmt.Errorf("parse apps restore payload failed: %w", err)
		}
	case "ports":
		if _, err := data.LoadPortBindingsFromRaw(raw); err != nil {
			return fmt.Errorf("parse ports restore payload failed: %w", err)
		}
	default:
		return fmt.Errorf("unsupported restore payload name: %s", name)
	}
	return nil
}

func writeRestoreFilesAtomically(files map[string][]byte) error {
	return data.WithConfigWriteLock(func() error {
		return writeRestoreFilesAtomicallyLocked(files)
	})
}

func writeRestoreFilesAtomicallyLocked(files map[string][]byte) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	needsConfigRefresh := false
	for _, name := range names {
		if name == "config" {
			needsConfigRefresh = true
			break
		}
	}

	pending := make([]pendingRestoreFile, 0, len(files))
	for _, name := range names {
		raw := files[name]
		target, err := resolveRestoreConfigPath(name)
		if err != nil {
			cleanupPendingRestoreTemps(pending)
			return err
		}
		dir := filepath.Dir(target)
		temp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".restore-*")
		if err != nil {
			cleanupPendingRestoreTemps(pending)
			return err
		}
		if _, err := temp.Write(raw); err != nil {
			_ = temp.Close()
			_ = os.Remove(temp.Name())
			cleanupPendingRestoreTemps(pending)
			return err
		}
		if err := temp.Chmod(0644); err != nil {
			_ = temp.Close()
			_ = os.Remove(temp.Name())
			cleanupPendingRestoreTemps(pending)
			return err
		}
		if err := syncRestoreTempFile(temp); err != nil {
			_ = temp.Close()
			_ = os.Remove(temp.Name())
			cleanupPendingRestoreTemps(pending)
			return err
		}
		if err := temp.Close(); err != nil {
			_ = os.Remove(temp.Name())
			cleanupPendingRestoreTemps(pending)
			return err
		}
		pending = append(pending, pendingRestoreFile{
			target: target,
			temp:   temp.Name(),
			name:   name,
		})
	}

	for index := range pending {
		item := &pending[index]
		if info, err := os.Stat(item.target); err == nil {
			if info.IsDir() {
				dirErr := fmt.Errorf("target path %s is a directory, cannot overwrite restore target", item.target)
				rollbackErr := rollbackPendingRestoreFiles(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(dirErr, rollbackErr)
				}
				return dirErr
			}
			backup, err := os.CreateTemp(filepath.Dir(item.target), "."+filepath.Base(item.target)+".backup-*")
			if err != nil {
				rollbackErr := rollbackPendingRestoreFiles(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(err, rollbackErr)
				}
				return err
			}
			item.backup = backup.Name()
			_ = backup.Close()
			_ = os.Remove(item.backup)
			if err := os.Rename(item.target, item.backup); err != nil {
				rollbackErr := rollbackPendingRestoreFiles(pending, index-1)
				if rollbackErr != nil {
					return errors.Join(err, rollbackErr)
				}
				return err
			}
		} else if !os.IsNotExist(err) {
			rollbackErr := rollbackPendingRestoreFiles(pending, index-1)
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}
		if err := os.Rename(item.temp, item.target); err != nil {
			rollbackErr := rollbackPendingRestoreFiles(pending, index)
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}
		data.InvalidateConfigCache(item.name)
	}
	if needsConfigRefresh {
		if err := refreshPagePaletteCache(); err != nil {
			rollbackErr := rollbackPendingRestoreFiles(pending, len(pending)-1)
			for _, item := range pending {
				data.InvalidateConfigCache(item.name)
			}
			if rollbackErr != nil {
				return errors.Join(fmt.Errorf("refresh page palette cache failed: %w", err), rollbackErr)
			}
			return fmt.Errorf("refresh page palette cache failed: %w", err)
		}
		if err := refreshRuntimeLoginConfigLocked(); err != nil {
			rollbackErr := rollbackPendingRestoreFiles(pending, len(pending)-1)
			for _, item := range pending {
				data.InvalidateConfigCache(item.name)
			}
			paletteRollbackErr := refreshPagePaletteCache()
			if paletteRollbackErr != nil {
				paletteRollbackErr = fmt.Errorf("refresh page palette cache after rollback failed: %w", paletteRollbackErr)
			}
			err = errors.Join(err, paletteRollbackErr)
			if rollbackErr != nil {
				return errors.Join(fmt.Errorf("refresh runtime login config failed: %w", err), rollbackErr)
			}
			return fmt.Errorf("refresh runtime login config failed: %w", err)
		}
	}
	for _, item := range pending {
		if item.backup != "" {
			_ = os.Remove(item.backup)
		}
	}
	return nil
}

func cleanupPendingRestoreTemps(items []pendingRestoreFile) {
	for _, item := range items {
		if item.temp == "" {
			continue
		}
		_ = os.Remove(item.temp)
	}
}

func refreshRuntimeLoginConfig() error {
	user, pass, err := data.GetLoginConfig()
	return storeRuntimeLoginConfig(user, pass, err)
}

func refreshRuntimeLoginConfigLocked() error {
	user, pass, err := data.GetLoginConfigLocked()
	return storeRuntimeLoginConfig(user, pass, err)
}

func storeRuntimeLoginConfig(user string, pass string, err error) error {
	if err != nil {
		return err
	}
	updated := define.SourceAppRuntimeFlags()
	if strings.TrimSpace(user) != "" {
		updated.User = strings.TrimSpace(user)
		updated.UserIsGenerated = false
	}
	if strings.TrimSpace(pass) != "" {
		updated.Pass = strings.TrimSpace(pass)
		updated.PassIsGenerated = false
	}
	auth.StoreLoginRuntimeConfigFromFlags(updated)
	return nil
}

func refreshRequestLoginRuntime(c *echo.Context) {
	if c == nil {
		return
	}
	auth.StoreLoginRuntimeConfigForRequest(c, auth.SnapshotLoginRuntimeConfigFromFlags(define.CurrentAppRuntimeFlags()))
}

func rollbackPendingRestoreFiles(items []pendingRestoreFile, appliedIndex int) error {
	var rollbackErr error
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		if err := os.Remove(item.temp); err != nil && !os.IsNotExist(err) {
			rollbackErr = errors.Join(rollbackErr, err)
		}
		if index > appliedIndex {
			if item.backup != "" {
				if err := os.Rename(item.backup, item.target); err != nil && !os.IsNotExist(err) {
					rollbackErr = errors.Join(rollbackErr, err)
				}
			}
			continue
		}
		if info, err := os.Stat(item.target); err == nil {
			if info.IsDir() {
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("target path %s is a directory, cannot remove restored config during rollback", item.target))
				continue
			}
			if err := os.Remove(item.target); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, err)
				continue
			}
		} else if !os.IsNotExist(err) {
			rollbackErr = errors.Join(rollbackErr, err)
			continue
		}
		if item.backup != "" {
			if err := os.Rename(item.backup, item.target); err != nil && !os.IsNotExist(err) {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
	}
	return rollbackErr
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

type parsedLinkCheckPayload struct {
	Items           []linkCheckItem
	ImmediateResult []linkCheckResult
}

const linkCheckUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"

func checkLinks(c *echo.Context) error {
	var body struct {
		Bookmarks string `json:"bookmarks" form:"bookmarks"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, editorJSONError("missing form data"))
	}
	payload, err := parseLinksForCheck(body.Bookmarks)
	if err != nil {
		return c.JSON(http.StatusBadRequest, editorJSONError("parse links payload failed: "+err.Error()))
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), linkCheckOverallTimeout)
	defer cancel()
	results := append([]linkCheckResult{}, payload.ImmediateResult...)
	results = append(results, runLinkChecks(ctx, payload.Items)...)
	sortLinkCheckResults(results)
	return c.JSON(http.StatusOK, results)
}

func parseLinksForCheck(input string) (parsedLinkCheckPayload, error) {
	if strings.TrimSpace(input) == "" {
		return parsedLinkCheckPayload{}, nil
	}
	reader := csv.NewReader(strings.NewReader(input))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return parsedLinkCheckPayload{}, err
	}
	result := parsedLinkCheckPayload{
		Items:           make([]linkCheckItem, 0, len(records)),
		ImmediateResult: make([]linkCheckResult, 0),
	}
	for lineIndex, record := range records {
		rowIndex := lineIndex + 1
		record = normalizeLinkCheckRecord(record)
		if isBlankLinkCheckRecord(record) {
			continue
		}
		if len(record) < 3 {
			return parsedLinkCheckPayload{}, fmt.Errorf("link check row %d is incomplete", rowIndex)
		}
		switch len(record) {
		case 6, 7, 8:
		default:
			return parsedLinkCheckPayload{}, fmt.Errorf("link check row %d has unsupported field count: %d", rowIndex, len(record))
		}
		row, convErr := strconv.Atoi(record[0])
		if convErr != nil || row <= 0 {
			return parsedLinkCheckPayload{}, fmt.Errorf("link check row %d has invalid row number %q", rowIndex, record[0])
		}
		rawURL := record[2]
		if rawURL == "" {
			continue
		}
		candidate := classifyLinkCheckCandidate(rawURL)
		if candidate.Skip {
			continue
		}
		if candidate.InvalidReason != "" {
			result.ImmediateResult = append(result.ImmediateResult, linkCheckResult{
				Row:    row,
				URL:    rawURL,
				Status: "invalid",
				Reason: candidate.InvalidReason,
			})
			continue
		}
		result.Items = append(result.Items, linkCheckItem{Row: row, URL: rawURL})
	}
	sortLinkCheckResults(result.ImmediateResult)
	return result, nil
}

func normalizeLinkCheckRecord(record []string) []string {
	normalized := make([]string, len(record))
	for i, field := range record {
		normalized[i] = strings.TrimSpace(field)
	}
	return normalized
}

func isBlankLinkCheckRecord(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}

type linkCheckCandidate struct {
	Skip          bool
	InvalidReason string
}

func classifyLinkCheckCandidate(rawURL string) linkCheckCandidate {
	u, err := url.Parse(rawURL)
	if err != nil {
		return linkCheckCandidate{InvalidReason: "invalid URL format"}
	}
	if u.Scheme == "" {
		return linkCheckCandidate{InvalidReason: "missing URL scheme"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return linkCheckCandidate{Skip: true}
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "" {
		if host == "" {
			return linkCheckCandidate{InvalidReason: "missing URL host"}
		}
		return linkCheckCandidate{Skip: true}
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return linkCheckCandidate{Skip: true}
		}
	}
	return linkCheckCandidate{}
}

type linkCheckRunner func(context.Context, *http.Client, linkCheckItem) linkCheckResult

func runLinkChecks(ctx context.Context, items []linkCheckItem) []linkCheckResult {
	results := make([]linkCheckResult, 0)
	resultsMu := sync.Mutex{}
	client := &http.Client{Timeout: linkCheckRequestTimeout}
	return runLinkChecksWithChecker(ctx, client, items, linkCheckWorkerCount, &results, &resultsMu, checkOneLink)
}

func runLinkChecksWithChecker(ctx context.Context, client *http.Client, items []linkCheckItem, workerCount int, results *[]linkCheckResult, resultsMu *sync.Mutex, checker linkCheckRunner) []linkCheckResult {
	if len(items) == 0 {
		return nil
	}
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > len(items) {
		workerCount = len(items)
	}

	jobs := make(chan linkCheckItem)
	scheduledRows := make(map[int]struct{}, len(items))
	scheduledMu := sync.Mutex{}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-jobs:
					if !ok {
						return
					}
					result := checker(ctx, client, item)
					if result.Status != "ok" {
						resultsMu.Lock()
						*results = append(*results, result)
						resultsMu.Unlock()
					}
				}
			}
		}()
	}

sendLoop:
	for _, item := range items {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- item:
			scheduledMu.Lock()
			scheduledRows[item.Row] = struct{}{}
			scheduledMu.Unlock()
		}
	}
	close(jobs)
	wg.Wait()
	appendPendingLinkCheckResults(ctx, items, scheduledRows, results, resultsMu)
	sortLinkCheckResults(*results)
	return *results
}

func appendPendingLinkCheckResults(ctx context.Context, items []linkCheckItem, scheduledRows map[int]struct{}, results *[]linkCheckResult, resultsMu *sync.Mutex) {
	if ctx == nil || ctx.Err() == nil {
		return
	}
	reason := pendingLinkCheckReason(ctx.Err())
	if reason == "" {
		return
	}
	pending := make([]linkCheckResult, 0)
	for _, item := range items {
		if _, ok := scheduledRows[item.Row]; ok {
			continue
		}
		pending = append(pending, linkCheckResult{
			Row:    item.Row,
			URL:    item.URL,
			Status: "unstable",
			Reason: reason,
		})
	}
	if len(pending) == 0 {
		return
	}
	resultsMu.Lock()
	*results = append(*results, pending...)
	resultsMu.Unlock()
}

func pendingLinkCheckReason(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "check timed out before this URL could be verified"
	}
	if errors.Is(err, context.Canceled) {
		return "check was canceled before this URL could be verified"
	}
	return strings.TrimSpace(err.Error())
}

func sortLinkCheckResults(results []linkCheckResult) {
	sort.Slice(results, func(i int, j int) bool {
		if results[i].Row == results[j].Row {
			return results[i].URL < results[j].URL
		}
		return results[i].Row < results[j].Row
	})
}

func checkOneLink(ctx context.Context, client *http.Client, item linkCheckItem) linkCheckResult {
	for attempt := 0; attempt < 2; attempt++ {
		result := checkOneLinkOnce(ctx, client, item)
		if result.Status != "unstable" || attempt == 1 {
			return result
		}
		if err := waitForRetry(ctx, linkCheckRetryDelay); err != nil {
			return linkCheckResult{Row: item.Row, URL: item.URL, Status: "unstable", Reason: err.Error()}
		}
	}
	return linkCheckResult{Row: item.Row, URL: item.URL, Status: "unstable", Reason: "unknown transient error"}
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func checkOneLinkOnce(ctx context.Context, client *http.Client, item linkCheckItem) linkCheckResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return linkCheckResult{Row: item.Row, URL: item.URL, Status: "invalid", Reason: err.Error()}
	}
	req.Header.Set("User-Agent", linkCheckUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	resp, err := client.Do(req)
	if err != nil {
		status, reason := classifyLinkRequestError(err)
		return linkCheckResult{Row: item.Row, URL: item.URL, Status: status, Reason: reason}
	}
	defer resp.Body.Close()
	status := classifyLinkResponseStatus(resp.StatusCode)
	return linkCheckResult{Row: item.Row, URL: item.URL, Status: status, Reason: resp.Status}
}

func classifyLinkRequestError(err error) (string, string) {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.IsNotFound {
			return "invalid", err.Error()
		}
		return "unstable", err.Error()
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "unstable", err.Error()
		}
		if netErr.Temporary() {
			return "unstable", err.Error()
		}
	}
	return "unstable", err.Error()
}

func classifyLinkResponseStatus(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 400:
		return "ok"
	case statusCode == http.StatusUnauthorized ||
		statusCode == http.StatusForbidden ||
		statusCode == http.StatusProxyAuthRequired ||
		statusCode == http.StatusMethodNotAllowed ||
		statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusUnavailableForLegalReasons:
		return "restricted"
	case statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusMisdirectedRequest ||
		statusCode == http.StatusTooEarly ||
		statusCode >= 500:
		return "unstable"
	default:
		return "invalid"
	}
}

func render(c *echo.Context) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareSettingsOptionsForRender(options)
	dataCategories, dataBookmarks, err := data.GetBookmarksForEditor()
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	showEditorPortPicker := !auth.IsLoginDisabled(c)
	portsJSON := "[]"
	localLANHost := ""
	if showEditorPortPicker {
		portsConfig, err := data.LoadPortBindings()
		if err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
		}
		portsJSON, err = marshalEditorPorts(portsConfig.Items)
		if err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
		}
		if portsJSON != "[]" {
			localLANHost = portscollector.LocalLANHost()
		}
	}
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["PageName"] = "Editor"
	m["SettingPages"] = define.SettingPages
	m["DebugMode"] = currentEditorRuntime().DebugMode
	m["DebugAssetVersion"] = getDebugAssetVersion()
	m["DataCategories"] = template.HTML(dataCategories)
	m["DataBookmarks"] = template.HTML(dataBookmarks)
	m["DataPorts"] = template.HTML(portsJSON)
	m["LocalLANHost"] = localLANHost
	m["ShowEditorPortPicker"] = showEditorPortPicker
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	m["Locale"] = options.Locale
	footer.BindTemplateData(m, options.Footer)
	m["OptionOpenAppNewTab"] = options.OpenAppNewTab
	m["OptionOpenBookmarkNewTab"] = options.OpenBookmarkNewTab
	m["OptionShowTitle"] = options.ShowTitle
	m["OptionShowDateTime"] = options.ShowDateTime
	m["OptionShowApps"] = options.ShowApps
	m["OptionShowBookmarks"] = options.ShowBookmarks
	m["OperationNotice"] = resolveEditorOperationNotice(options.Locale, c.QueryParam(editorNoticeQueryKey))
	m["RenderWarnings"] = renderWarnings
	return c.Render(http.StatusOK, "editor.html", m)
}

func marshalEditorPorts(items []model.PortBinding) (string, error) {
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
	raw, err := marshalEditorPortsJSON(result)
	if err != nil {
		return "", fmt.Errorf("marshal editor ports failed: %w", err)
	}
	return string(raw), nil
}
