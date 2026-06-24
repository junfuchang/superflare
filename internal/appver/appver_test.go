package appver_test

import (
	"testing"

	"github.com/junfuchang/superflare/internal/appver"
	version "github.com/soulteary/version-kit"
	"github.com/stretchr/testify/assert"
)

func TestDisplayVersionFromInfoUsesBuildDate(t *testing.T) {
	info := version.New("1.0.0", "abc1234", "2026-06-24T12:30:00Z")

	assert.Equal(t, "20260624", appver.DisplayVersionFromInfo(info))
}

func TestDisplayVersionFromInfoFallsBackToDateText(t *testing.T) {
	info := version.New("1.0.0", "abc1234", "2026-06-24 12:30:00")

	assert.Equal(t, "20260624", appver.DisplayVersionFromInfo(info))
}

func TestDisplayVersionFromInfoFallsBackToVersion(t *testing.T) {
	info := version.New("dev", "abc1234", "unknown")

	assert.Regexp(t, `^\d{8}$|^dev$`, appver.DisplayVersionFromInfo(info))
}
