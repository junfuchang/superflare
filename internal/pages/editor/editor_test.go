package editor

import (
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestMarshalEditorPortsOnlyIncludesRemarkedPorts(t *testing.T) {
	got := marshalEditorPorts([]model.PortBinding{
		{Port: 3060, Protocol: "tcp", Remark: "dev"},
		{Port: 8080, Protocol: "tcp"},
		{Port: 5353, Protocol: "udp", Remark: "dns"},
		{Port: 9090, Protocol: "tcp", Remark: "hidden", Hidden: true},
	})
	if !strings.Contains(got, `"Port":3060`) || !strings.Contains(got, `"Remark":"dev"`) {
		t.Fatalf("expected remarked port in %s", got)
	}
	if strings.Contains(got, "8080") {
		t.Fatalf("unexpected unremarked port in %s", got)
	}
	if strings.Contains(got, "5353") {
		t.Fatalf("unexpected udp port in %s", got)
	}
	if strings.Contains(got, "9090") {
		t.Fatalf("unexpected hidden port in %s", got)
	}
}
