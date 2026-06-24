//go:build linux

package ports

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProcNetFiltersTCPListenAndKeepsUDP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "net")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000
   1: 00000000:2382 00000000:0000 01 00000000:00000000 00:00000000 00000000  1000        0 67890 1 0000000000000000
`
	file := filepath.Join(path, "tcp")
	if err := os.WriteFile(file, []byte(raw), 0644); err != nil {
		t.Fatalf("write proc net fixture: %v", err)
	}
	tcp := parseProcNet(file, "tcp", true)
	if len(tcp) != 1 || tcp[0].port != 8080 || tcp[0].inode != "12345" {
		t.Fatalf("unexpected tcp sockets: %#v", tcp)
	}
	udp := parseProcNet(file, "udp", false)
	if len(udp) != 2 || udp[0].port != 8080 || udp[1].port != 9090 {
		t.Fatalf("unexpected udp sockets: %#v", udp)
	}
}

func TestGetProcRootCanUseHostProcOverride(t *testing.T) {
	t.Setenv(envPortProcRoot, "/host/proc/")
	if got := getProcRoot(); got != filepath.Clean("/host/proc/") {
		t.Fatalf("unexpected proc root: %q", got)
	}
}

func TestParseSSOutputExtractsPIDAndName(t *testing.T) {
	raw := `tcp LISTEN 0 4096 0.0.0.0:5668 0.0.0.0:* users:(("superflare",pid=321,fd=9))
udp UNCONN 0 0 0.0.0.0:5353 0.0.0.0:* users:(("avahi-daemon",pid=112,fd=12))`
	got := parseSSOutput(raw)
	if len(got) != 2 {
		t.Fatalf("unexpected ss items: %#v", got)
	}
	if got[0].Port != 5668 || got[0].Protocol != "tcp" || got[0].PID != 321 || got[0].ServiceName != "superflare" {
		t.Fatalf("unexpected tcp item: %#v", got[0])
	}
	if got[1].Port != 5353 || got[1].Protocol != "udp" || got[1].PID != 112 || got[1].ServiceName != "avahi-daemon" {
		t.Fatalf("unexpected udp item: %#v", got[1])
	}
}

func TestParseNetstatOutputExtractsPIDAndName(t *testing.T) {
	raw := `Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 0.0.0.0:5668            0.0.0.0:*               LISTEN      321/superflare
udp        0      0 0.0.0.0:5353            0.0.0.0:*                           112/avahi-daemon`
	got := parseNetstatOutput(raw)
	if len(got) != 2 {
		t.Fatalf("unexpected netstat items: %#v", got)
	}
	if got[0].Port != 5668 || got[0].Protocol != "tcp" || got[0].PID != 321 || got[0].ServiceName != "superflare" {
		t.Fatalf("unexpected tcp item: %#v", got[0])
	}
	if got[1].Port != 5353 || got[1].Protocol != "udp" || got[1].PID != 112 || got[1].ServiceName != "avahi-daemon" {
		t.Fatalf("unexpected udp item: %#v", got[1])
	}
}

func TestFillMissingRuntimePortOwnersUsesFallbackNames(t *testing.T) {
	items := []runtimePort{
		{Port: 5668, Protocol: "tcp"},
		{Port: 5353, Protocol: "udp", PID: 112},
	}
	fillMissingRuntimePortOwners(items, []runtimePort{
		{Port: 5668, Protocol: "tcp", PID: 321, ServiceName: "superflare"},
		{Port: 5353, Protocol: "udp", PID: 112, ServiceName: "avahi-daemon"},
	})
	if items[0].PID != 321 || items[0].ServiceName != "superflare" {
		t.Fatalf("missing fallback owner fill: %#v", items[0])
	}
	if items[1].PID != 112 || items[1].ServiceName != "avahi-daemon" {
		t.Fatalf("missing fallback service name fill: %#v", items[1])
	}
}
