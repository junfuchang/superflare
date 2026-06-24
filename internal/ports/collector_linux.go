//go:build linux

package ports

import (
	"bufio"
	"os"
	"os/exec"
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
		return collectCommandRuntimePorts()
	}
	inodeOwners := collectProcSocketOwners(procRoot)
	items := make([]runtimePort, 0, len(sockets))
	needsFallback := false
	for _, socket := range sockets {
		owner := inodeOwners[socket.inode]
		if owner.pid <= 0 || strings.TrimSpace(owner.name) == "" {
			needsFallback = true
		}
		items = append(items, runtimePort{
			Port:        socket.port,
			Protocol:    socket.protocol,
			PID:         owner.pid,
			ServiceName: owner.name,
		})
	}
	if needsFallback {
		fillMissingRuntimePortOwners(items, collectCommandRuntimePorts())
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
	pidDir := filepath.Join(procRoot, strconv.Itoa(pid))
	raw, err := os.ReadFile(filepath.Join(pidDir, "comm"))
	if err == nil {
		if name := strings.TrimSpace(string(raw)); name != "" {
			return name
		}
	}
	raw, err = os.ReadFile(filepath.Join(pidDir, "cmdline"))
	if err == nil {
		for _, part := range strings.Split(string(raw), "\x00") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			return normalizeProcessName(part)
		}
	}
	target, err := os.Readlink(filepath.Join(pidDir, "exe"))
	if err == nil {
		if name := normalizeProcessName(target); name != "" {
			return name
		}
	}
	return ""
}

func fillMissingRuntimePortOwners(items []runtimePort, fallback []runtimePort) {
	if len(items) == 0 || len(fallback) == 0 {
		return
	}
	owners := map[string]runtimePort{}
	for _, item := range fallback {
		if item.Port <= 0 || item.Port > 65535 {
			continue
		}
		key := bindingKey(item.Protocol, item.Port)
		prev := owners[key]
		if runtimePortOwnerScore(item) >= runtimePortOwnerScore(prev) {
			owners[key] = item
		}
	}
	for idx := range items {
		key := bindingKey(items[idx].Protocol, items[idx].Port)
		owner, ok := owners[key]
		if !ok {
			continue
		}
		if items[idx].PID <= 0 && owner.PID > 0 {
			items[idx].PID = owner.PID
		}
		if strings.TrimSpace(items[idx].ServiceName) == "" && strings.TrimSpace(owner.ServiceName) != "" {
			items[idx].ServiceName = strings.TrimSpace(owner.ServiceName)
		}
	}
}

func runtimePortOwnerScore(item runtimePort) int {
	score := 0
	if item.PID > 0 {
		score++
	}
	if strings.TrimSpace(item.ServiceName) != "" {
		score += 2
	}
	return score
}

func collectCommandRuntimePorts() []runtimePort {
	merged := map[string]runtimePort{}
	for _, source := range [][]runtimePort{
		collectSSRuntimePorts(),
		collectNetstatRuntimePorts(),
	} {
		for _, item := range source {
			if item.Port <= 0 || item.Port > 65535 {
				continue
			}
			key := bindingKey(item.Protocol, item.Port)
			prev := merged[key]
			if runtimePortOwnerScore(item) >= runtimePortOwnerScore(prev) {
				merged[key] = item
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	items := make([]runtimePort, 0, len(merged))
	for _, item := range merged {
		items = append(items, item)
	}
	return items
}

func collectSSRuntimePorts() []runtimePort {
	out, err := exec.Command("ss", "-H", "-lntup").Output()
	if err != nil {
		return nil
	}
	return parseSSOutput(string(out))
}

func parseSSOutput(raw string) []runtimePort {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	var items []runtimePort
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		protocol := normalizeCommandProtocol(fields[0])
		if protocol == "" {
			continue
		}
		port, ok := parseCommandPort(fields[4])
		if !ok {
			continue
		}
		pid, name := 0, ""
		if len(fields) > 6 {
			pid, name = parseSSProcessField(strings.Join(fields[6:], " "))
		}
		items = append(items, runtimePort{
			Port:        port,
			Protocol:    protocol,
			PID:         pid,
			ServiceName: name,
		})
	}
	return items
}

func collectNetstatRuntimePorts() []runtimePort {
	out, err := exec.Command("netstat", "-tunlp").Output()
	if err != nil {
		return nil
	}
	return parseNetstatOutput(string(out))
}

func parseNetstatOutput(raw string) []runtimePort {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	var items []runtimePort
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Proto") || strings.HasPrefix(line, "Active") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		protocol := normalizeCommandProtocol(fields[0])
		if protocol == "" {
			continue
		}
		port, ok := parseCommandPort(fields[3])
		if !ok {
			continue
		}
		pid, name := 0, ""
		if len(fields) > 4 {
			pid, name = parseNetstatProcessField(fields[len(fields)-1])
		}
		items = append(items, runtimePort{
			Port:        port,
			Protocol:    protocol,
			PID:         pid,
			ServiceName: name,
		})
	}
	return items
}

func normalizeCommandProtocol(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(value, "tcp"):
		return "tcp"
	case strings.HasPrefix(value, "udp"):
		return "udp"
	default:
		return ""
	}
}

func parseCommandPort(value string) (int, bool) {
	value = strings.TrimSpace(value)
	idx := strings.LastIndex(value, ":")
	if idx < 0 || idx == len(value)-1 {
		return 0, false
	}
	port, err := strconv.Atoi(value[idx+1:])
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

func parseSSProcessField(value string) (int, string) {
	pid := parseIntAfter(value, "pid=")
	name := ""
	start := strings.Index(value, "\"")
	if start >= 0 {
		end := strings.Index(value[start+1:], "\"")
		if end >= 0 {
			name = value[start+1 : start+1+end]
		}
	}
	return pid, normalizeProcessName(name)
}

func parseNetstatProcessField(value string) (int, string) {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return 0, ""
	}
	parts := strings.SplitN(value, "/", 2)
	pid, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	if len(parts) == 1 {
		return pid, ""
	}
	return pid, normalizeProcessName(parts[1])
}

func parseIntAfter(value string, prefix string) int {
	idx := strings.Index(value, prefix)
	if idx < 0 {
		return 0
	}
	value = value[idx+len(prefix):]
	end := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	port, err := strconv.Atoi(value[:end])
	if err != nil {
		return 0
	}
	return port
}

func normalizeProcessName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = filepath.Base(value)
	if value == "." || value == string(filepath.Separator) {
		return ""
	}
	return value
}
