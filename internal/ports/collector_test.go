package ports

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestMergeRuntimeAndBindingsKeepsOfflineRemark(t *testing.T) {
	got := MergeRuntimeAndBindings([]runtimePort{
		{Port: 3060, Protocol: "tcp", PID: 100, ServiceName: "app"},
	}, map[string]model.PortBinding{
		bindingKey("tcp", 3060): {Port: 3060, Protocol: "tcp", Remark: "dev app"},
		bindingKey("tcp", 8080): {Port: 8080, Protocol: "tcp", Remark: "old app"},
	}, false)
	if len(got) != 2 {
		t.Fatalf("expected 2 ports, got %#v", got)
	}
	if got[0].Port != 3060 || !got[0].Running || got[0].Remark != "dev app" || got[0].PID != 100 {
		t.Fatalf("unexpected running item: %#v", got[0])
	}
	if got[1].Port != 8080 || got[1].Running || got[1].Remark != "old app" || got[1].PID != 0 || got[1].ServiceName != "" {
		t.Fatalf("unexpected offline item: %#v", got[1])
	}
}

func TestMergeRuntimeAndBindingsHidesPortsByDefault(t *testing.T) {
	got := MergeRuntimeAndBindings([]runtimePort{
		{Port: 3060, Protocol: "tcp", PID: 100, ServiceName: "app"},
	}, map[string]model.PortBinding{
		bindingKey("tcp", 3060): {Port: 3060, Protocol: "tcp", Hidden: true},
		bindingKey("tcp", 8080): {Port: 8080, Protocol: "tcp", Remark: "old app", Hidden: true},
	}, false)
	if len(got) != 0 {
		t.Fatalf("expected hidden ports to be filtered, got %#v", got)
	}

	withHidden := MergeRuntimeAndBindings([]runtimePort{
		{Port: 3060, Protocol: "tcp", PID: 100, ServiceName: "app"},
	}, map[string]model.PortBinding{
		bindingKey("tcp", 3060): {Port: 3060, Protocol: "tcp", Hidden: true},
		bindingKey("tcp", 8080): {Port: 8080, Protocol: "tcp", Remark: "old app", Hidden: true},
	}, true)
	if len(withHidden) != 2 || !withHidden[0].Hidden || !withHidden[1].Hidden {
		t.Fatalf("expected hidden ports when included, got %#v", withHidden)
	}
}

func TestMergeRuntimeAndBindingsKeepsBestRuntimeOwnerInfo(t *testing.T) {
	got := MergeRuntimeAndBindings([]runtimePort{
		{Port: 5668, Protocol: "tcp", PID: 321, ServiceName: "superflare"},
		{Port: 5668, Protocol: "tcp", PID: 321, ServiceName: ""},
	}, map[string]model.PortBinding{}, false)
	if len(got) != 1 {
		t.Fatalf("expected one merged port, got %#v", got)
	}
	if got[0].PID != 321 || got[0].ServiceName != "superflare" {
		t.Fatalf("expected best runtime owner info to win, got %#v", got[0])
	}
}

func TestCollectWithHiddenErrReturnsConfigErrorWhenBindingsBroken(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "ports.yaml"), []byte("ports: [broken"), 0644); err != nil {
		t.Fatalf("write ports.yaml: %v", err)
	}

	_, err = CollectWithHiddenErr(false)
	if err == nil {
		t.Fatal("expected CollectWithHiddenErr to fail")
	}
	if !strings.Contains(err.Error(), "load port bindings failed") {
		t.Fatalf("expected wrapped bindings error, got %v", err)
	}
}

func TestUnsupportedRuntimeCollectorResultReturnsExplicitError(t *testing.T) {
	got := unsupportedRuntimeCollectorResult()
	if got.Err == nil {
		t.Fatal("expected unsupported runtime collector error")
	}
	if !strings.Contains(got.Err.Error(), "not supported on this platform") {
		t.Fatalf("unexpected unsupported runtime collector error: %v", got.Err)
	}
	if len(got.Ports) != 0 {
		t.Fatalf("expected no runtime ports for unsupported platform result, got %#v", got.Ports)
	}
}

func TestOwnerResolutionWarningCountsMissingOwnerInfo(t *testing.T) {
	warning := ownerResolutionWarning([]runtimePort{
		{Port: 53, Protocol: "udp", PID: 3136, ServiceName: ""},
		{Port: 135, Protocol: "tcp", PID: 0, ServiceName: "rpc"},
		{Port: 5668, Protocol: "tcp", PID: 4321, ServiceName: "superflare"},
	}, "lookup failed")
	if warning.Code != "owner_resolution_partial" {
		t.Fatalf("unexpected warning code: %#v", warning)
	}
	if warning.MissingOwners != 2 || warning.RuntimePorts != 3 {
		t.Fatalf("unexpected warning counts: %#v", warning)
	}
	if warning.Detail != "lookup failed" {
		t.Fatalf("unexpected warning detail: %#v", warning)
	}
}
