package mdi

import (
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
)

func TestGetIconByNameNormalizesMDIInput(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	define.ThemeCurrent = "blackboard"
	define.ThemePrimaryColor = "rgba(255, 253, 234, 1)"

	for _, name := range []string{"home-circle", "homeCircle", "home_circle"} {
		got := GetIconByName(name)
		if !strings.Contains(got, "/assets/mdi/blackboard-homecircle.svg") {
			t.Fatalf("GetIconByName(%q) did not resolve homecircle icon: %s", name, got)
		}
	}

	if got := GetIconURLByName("home-circle"); got != "/assets/mdi/blackboard-homecircle.svg" {
		t.Fatalf("GetIconURLByName did not resolve normalized favicon icon: %s", got)
	}
}
