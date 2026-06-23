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
