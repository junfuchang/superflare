package ports

import "testing"

import "github.com/junfuchang/superflare/config/model"

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
