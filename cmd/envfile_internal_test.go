package cmd

import (
	"os"
	"testing"

	"github.com/junfuchang/superflare/config/model"
	"github.com/stretchr/testify/assert"
)

func TestParseEnvFile_GetwdFailureReturnsError(t *testing.T) {
	originalResolve := resolveDotEnvPath
	defer func() { resolveDotEnvPath = originalResolve }()

	resolveDotEnvPath = func() (string, error) {
		return "", os.ErrPermission
	}

	baseFlags := model.Flags{
		Port: 3636,
		User: "fallback-user",
		Pass: "fallback-pass",
	}

	flags, err := ParseEnvFileE(baseFlags)
	if err == nil {
		t.Fatal("expected ParseEnvFileE to fail")
	}
	assert.Equal(t, baseFlags, flags)
	assert.ErrorContains(t, err, "resolve .env path failed")
}
