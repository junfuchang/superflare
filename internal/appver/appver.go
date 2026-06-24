package appver

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	version "github.com/soulteary/version-kit"
)

var dateVersionPattern = regexp.MustCompile(`(\d{4})[-_/\.]?(\d{2})[-_/\.]?(\d{2})`)

func DisplayVersion() string {
	return DisplayVersionFromInfo(version.Default())
}

func DisplayVersionFromInfo(info *version.Info) string {
	if info == nil {
		return "dev"
	}

	if buildVersion := buildDateVersion(info); buildVersion != "" {
		return buildVersion
	}

	if fileVersion := executableDateVersion(); fileVersion != "" {
		return fileVersion
	}

	fallback := strings.TrimSpace(info.Version)
	if fallback == "" {
		return "dev"
	}

	return fallback
}

func ProgramVersionString() string {
	return fmt.Sprintf("SuperFlare v%s %s/%s", DisplayVersion(), runtime.GOOS, runtime.GOARCH)
}

func buildDateVersion(info *version.Info) string {
	if info == nil {
		return ""
	}

	if buildTime := info.BuildTimestamp(); !buildTime.IsZero() {
		return buildTime.Format("20060102")
	}

	return extractDateVersion(info.BuildDate)
}

func extractDateVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "unknown" {
		return ""
	}

	matches := dateVersionPattern.FindStringSubmatch(raw)
	if len(matches) != 4 {
		return ""
	}

	return matches[1] + matches[2] + matches[3]
}

func executableDateVersion() string {
	executablePath, err := os.Executable()
	if err != nil {
		return ""
	}

	info, err := os.Stat(filepath.Clean(executablePath))
	if err != nil {
		return ""
	}

	modTime := info.ModTime()
	if modTime.IsZero() {
		return ""
	}

	return modTime.In(time.Local).Format("20060102")
}
