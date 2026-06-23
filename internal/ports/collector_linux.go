//go:build linux

package ports

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type procSocket struct {
	port     int
	protocol string
	inode    string
}

const envPortProcRoot = "FLARE_PORT_PROC_ROOT"

func collectRuntimePorts() []runtimePort {
	procRoot := getProcRoot()
	sockets := collectProcSockets(procRoot)
	if len(sockets) == 0 {
		return nil
	}
	inodeOwners := collectProcSocketOwners(procRoot)
	items := make([]runtimePort, 0, len(sockets))
	for _, socket := range sockets {
		owner := inodeOwners[socket.inode]
		items = append(items, runtimePort{
			Port:        socket.port,
			Protocol:    socket.protocol,
			PID:         owner.pid,
			ServiceName: owner.name,
		})
	}
	return items
}

func getProcRoot() string {
	procRoot := strings.TrimSpace(os.Getenv(envPortProcRoot))
	if procRoot == "" {
		return "/proc"
	}
	return filepath.Clean(procRoot)
}

func collectProcSockets(procRoot string) []procSocket {
	var result []procSocket
	for _, source := range []struct {
		name       string
		protocol   string
		listenOnly bool
	}{
		{"tcp", "tcp", true},
		{"tcp6", "tcp", true},
		{"udp", "udp", false},
		{"udp6", "udp", false},
	} {
		result = append(result, parseProcNet(filepath.Join(procRoot, "net", source.name), source.protocol, source.listenOnly)...)
	}
	return result
}

func parseProcNet(path string, protocol string, listenOnly bool) []procSocket {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var result []procSocket
	scanner := bufio.NewScanner(f)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		if listenOnly && fields[3] != "0A" {
			continue
		}
		port, ok := parseProcAddressPort(fields[1])
		if !ok {
			continue
		}
		result = append(result, procSocket{port: port, protocol: protocol, inode: fields[9]})
	}
	return result
}

func parseProcAddressPort(addr string) (int, bool) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 || idx == len(addr)-1 {
		return 0, false
	}
	value, err := strconv.ParseInt(addr[idx+1:], 16, 32)
	if err != nil || value <= 0 || value > 65535 {
		return 0, false
	}
	return int(value), true
}

type procOwner struct {
	pid  int
	name string
}

func collectProcSocketOwners(procRoot string) map[string]procOwner {
	result := map[string]procOwner{}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		fdDir := filepath.Join(procRoot, entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		name := readProcName(procRoot, pid)
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") || !strings.HasSuffix(link, "]") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			if _, exists := result[inode]; !exists {
				result[inode] = procOwner{pid: pid, name: name}
			}
		}
	}
	return result
}

func readProcName(procRoot string, pid int) string {
	raw, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "comm"))
	if err == nil {
		return strings.TrimSpace(string(raw))
	}
	return ""
}
