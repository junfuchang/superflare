package i18n

import (
	"strings"
	"testing"
)

func TestDefaultLoginCredentialsWarningDoesNotExposeDefaultCredentialValue(t *testing.T) {
	for _, locale := range []string{"zh", "en"} {
		got := T(locale, "default_login_credentials_warning")
		lower := strings.ToLower(got)
		if strings.Contains(lower, "admin/admin") || strings.Contains(lower, "admin / admin") {
			t.Fatalf("default credential warning for %s exposes the default credential value: %q", locale, got)
		}
	}
}
