//go:build windows

package ports

import (
	"bytes"
	"encoding/csv"
	"os/exec"
	"strconv"
	"strings"
)

func collectRuntimePorts() []runtimePort {
	processNames := map[int]string{}
	items := collectWindowsPorts(`Get-NetTCPConnection -State Listen | Select-Object LocalPort,OwningProcess | Sort-Object LocalPort,OwningProcess -Unique | ConvertTo-Csv -NoTypeInformation`, "tcp", processNames)
	items = append(items, collectWindowsPorts(`Get-NetUDPEndpoint | Select-Object LocalPort,OwningProcess | Sort-Object LocalPort,OwningProcess -Unique | ConvertTo-Csv -NoTypeInformation`, "udp", processNames)...)
	return items
}

func collectWindowsPorts(script string, protocol string, processNames map[int]string) []runtimePort {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	reader := csv.NewReader(bytes.NewReader(out))
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return nil
	}
	items := make([]runtimePort, 0, len(records)-1)
	for _, record := range records[1:] {
		if len(record) < 2 {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(record[0]))
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(record[1]))
		name := ""
		if pid > 0 {
			name = processNames[pid]
			if name == "" {
				name = lookupWindowsProcessName(pid)
				processNames[pid] = name
			}
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

func lookupWindowsProcessName(pid int) string {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "try { (Get-Process -Id "+strconv.Itoa(pid)+").ProcessName } catch { '' }")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
