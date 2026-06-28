//go:build linux

package ports

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	tcp, err := parseProcNet(file, "tcp", true)
	if err != nil {
		t.Fatalf("parseProcNet tcp: %v", err)
	}
	if len(tcp) != 1 || tcp[0].port != 8080 || tcp[0].inode != "12345" {
		t.Fatalf("unexpected tcp sockets: %#v", tcp)
	}
	udp, err := parseProcNet(file, "udp", false)
	if err != nil {
		t.Fatalf("parseProcNet udp: %v", err)
	}
	if len(udp) != 2 || udp[0].port != 8080 || udp[1].port != 9090 {
		t.Fatalf("unexpected udp sockets: %#v", udp)
	}
}

func TestParseProcNetReturnsErrorWhenScannerFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "net")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	raw := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" + strings.Repeat("a", bufio.MaxScanTokenSize+1) + "\n"
	file := filepath.Join(path, "tcp")
	if err := os.WriteFile(file, []byte(raw), 0644); err != nil {
		t.Fatalf("write proc net fixture: %v", err)
	}

	got, err := parseProcNet(file, "tcp", true)
	if err == nil {
		t.Fatal("expected proc scan failure")
	}
	if got != nil {
		t.Fatalf("expected no sockets on scan failure, got %#v", got)
	}
	if !strings.Contains(err.Error(), "scan proc socket file") {
		t.Fatalf("unexpected error: %v", err)
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
	got, err := parseSSOutput(raw)
	if err != nil {
		t.Fatalf("parseSSOutput: %v", err)
	}
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
	got, err := parseNetstatOutput(raw)
	if err != nil {
		t.Fatalf("parseNetstatOutput: %v", err)
	}
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

func TestParseSSOutputReturnsErrorWhenScannerFails(t *testing.T) {
	_, err := parseSSOutput(strings.Repeat("a", bufio.MaxScanTokenSize+1))
	if err == nil {
		t.Fatal("expected ss scanner failure")
	}
	if !strings.Contains(err.Error(), "scan ss output failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseNetstatOutputReturnsErrorWhenScannerFails(t *testing.T) {
	_, err := parseNetstatOutput(strings.Repeat("a", bufio.MaxScanTokenSize+1))
	if err == nil {
		t.Fatal("expected netstat scanner failure")
	}
	if !strings.Contains(err.Error(), "scan netstat output failed") {
		t.Fatalf("unexpected error: %v", err)
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

func TestCollectCommandRuntimePortsReturnsNilWhenCommandsFail(t *testing.T) {
	originalRunner := runPortCommand
	defer func() { runPortCommand = originalRunner }()

	runPortCommand = func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("command failed")
	}

	got := collectCommandRuntimePorts()
	if got != nil {
		t.Fatalf("expected nil runtime ports when commands fail, got %#v", got)
	}
}

func TestCollectCommandRuntimePortsErrReturnsErrorWhenCommandsFail(t *testing.T) {
	originalRunner := runPortCommand
	defer func() { runPortCommand = originalRunner }()

	runPortCommand = func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("command failed")
	}

	got, err := collectCommandRuntimePortsErr()
	if err == nil {
		t.Fatal("expected runtime port command error")
	}
	if got != nil {
		t.Fatalf("expected nil runtime ports on command failure, got %#v", got)
	}
	if !strings.Contains(err.Error(), "command failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectRuntimePortsResultFallsBackWhenProcScanFails(t *testing.T) {
	originalRunner := runPortCommand
	defer func() { runPortCommand = originalRunner }()

	dir := t.TempDir()
	t.Setenv(envPortProcRoot, dir)
	netDir := filepath.Join(dir, "net")
	if err := os.MkdirAll(netDir, 0755); err != nil {
		t.Fatalf("mkdir net dir: %v", err)
	}
	raw := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" + strings.Repeat("a", bufio.MaxScanTokenSize+1) + "\n"
	if err := os.WriteFile(filepath.Join(netDir, "tcp"), []byte(raw), 0644); err != nil {
		t.Fatalf("write proc tcp: %v", err)
	}

	runPortCommand = func(name string, args ...string) ([]byte, error) {
		switch name {
		case "ss":
			return []byte(`tcp LISTEN 0 4096 0.0.0.0:5668 0.0.0.0:* users:(("superflare",pid=321,fd=9))`), nil
		case "netstat":
			return nil, errors.New("netstat unavailable")
		default:
			return nil, errors.New("unexpected command")
		}
	}

	got := collectRuntimePortsResult()
	if got.Err != nil {
		t.Fatalf("expected command fallback success, got error %v", got.Err)
	}
	if len(got.Ports) != 1 {
		t.Fatalf("expected one runtime port, got %#v", got.Ports)
	}
	if got.Ports[0].Port != 5668 || got.Ports[0].ServiceName != "superflare" || got.Ports[0].PID != 321 {
		t.Fatalf("unexpected fallback runtime port: %#v", got.Ports[0])
	}
}

func TestCollectRuntimePortsResultReturnsErrorWhenProcScanAndCommandsFail(t *testing.T) {
	originalRunner := runPortCommand
	defer func() { runPortCommand = originalRunner }()

	dir := t.TempDir()
	t.Setenv(envPortProcRoot, dir)
	netDir := filepath.Join(dir, "net")
	if err := os.MkdirAll(netDir, 0755); err != nil {
		t.Fatalf("mkdir net dir: %v", err)
	}
	raw := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" + strings.Repeat("a", bufio.MaxScanTokenSize+1) + "\n"
	if err := os.WriteFile(filepath.Join(netDir, "tcp"), []byte(raw), 0644); err != nil {
		t.Fatalf("write proc tcp: %v", err)
	}

	runPortCommand = func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("command failed")
	}

	got := collectRuntimePortsResult()
	if got.Err == nil {
		t.Fatal("expected proc scan and command failure to surface")
	}
	if len(got.Ports) != 0 {
		t.Fatalf("expected no runtime ports on total failure, got %#v", got.Ports)
	}
	if !strings.Contains(got.Err.Error(), "scan proc socket file") || !strings.Contains(got.Err.Error(), "command failed") {
		t.Fatalf("expected combined proc and command failure, got %v", got.Err)
	}
}

func TestCollectRuntimePortsResultReturnsErrorWhenOwnerLookupMissingAndFallbackFails(t *testing.T) {
	originalRunner := runPortCommand
	defer func() { runPortCommand = originalRunner }()

	dir := t.TempDir()
	t.Setenv(envPortProcRoot, dir)
	netDir := filepath.Join(dir, "net")
	if err := os.MkdirAll(netDir, 0755); err != nil {
		t.Fatalf("mkdir net dir: %v", err)
	}
	raw := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1624 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000
`
	for _, name := range []string{"tcp", "tcp6", "udp", "udp6"} {
		content := []byte("  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")
		if name == "tcp" {
			content = []byte(raw)
		}
		if err := os.WriteFile(filepath.Join(netDir, name), content, 0644); err != nil {
			t.Fatalf("write proc %s: %v", name, err)
		}
	}

	runPortCommand = func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("command failed")
	}

	got := collectRuntimePortsResult()
	if got.Err == nil {
		t.Fatal("expected owner lookup failure to surface when fallback fails")
	}
	if len(got.Ports) != 0 {
		t.Fatalf("expected no runtime ports on owner lookup failure, got %#v", got.Ports)
	}
	if !strings.Contains(got.Err.Error(), "collect runtime port owners failed") || !strings.Contains(got.Err.Error(), "command failed") {
		t.Fatalf("unexpected error: %v", got.Err)
	}
}

func TestCollectRuntimePortsResultFillsMissingOwnersFromCommandFallback(t *testing.T) {
	originalRunner := runPortCommand
	defer func() { runPortCommand = originalRunner }()

	dir := t.TempDir()
	t.Setenv(envPortProcRoot, dir)
	netDir := filepath.Join(dir, "net")
	if err := os.MkdirAll(netDir, 0755); err != nil {
		t.Fatalf("mkdir net dir: %v", err)
	}
	raw := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1624 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000
`
	for _, name := range []string{"tcp", "tcp6", "udp", "udp6"} {
		content := []byte("  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")
		if name == "tcp" {
			content = []byte(raw)
		}
		if err := os.WriteFile(filepath.Join(netDir, name), content, 0644); err != nil {
			t.Fatalf("write proc %s: %v", name, err)
		}
	}

	runPortCommand = func(name string, args ...string) ([]byte, error) {
		switch name {
		case "ss":
			return []byte(`tcp LISTEN 0 4096 0.0.0.0:5668 0.0.0.0:* users:(("superflare",pid=321,fd=9))`), nil
		case "netstat":
			return nil, errors.New("netstat unavailable")
		default:
			return nil, errors.New("unexpected command")
		}
	}

	got := collectRuntimePortsResult()
	if got.Err != nil {
		t.Fatalf("expected command fallback to fill missing owners, got error %v", got.Err)
	}
	if len(got.Ports) != 1 {
		t.Fatalf("expected one runtime port, got %#v", got.Ports)
	}
	if got.Ports[0].Port != 5668 || got.Ports[0].PID != 321 || got.Ports[0].ServiceName != "superflare" {
		t.Fatalf("unexpected filled runtime port: %#v", got.Ports[0])
	}
}
