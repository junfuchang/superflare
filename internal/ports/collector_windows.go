//go:build windows

package ports

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const portCommandTimeout = 2 * time.Second

var runPowerShell = func(script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), portCommandTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
}

var lookupWindowsProcessNameErr = func(pid int) (string, error) {
	out, err := runPowerShell("try { (Get-Process -Id " + strconv.Itoa(pid) + ").ProcessName } catch { '' }")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func collectRuntimePorts() []runtimePort {
	return collectRuntimePortsResult().Ports
}

func collectRuntimePortsResult() runtimeCollectorResult {
	processNames, processErr := collectWindowsProcessNamesErr()
	tcp := collectWindowsPortsDetailed(`Get-NetTCPConnection -State Listen | Select-Object LocalPort,OwningProcess | Sort-Object LocalPort,OwningProcess -Unique | ConvertTo-Csv -NoTypeInformation`, "tcp", processNames)
	udp := collectWindowsPortsDetailed(`Get-NetUDPEndpoint | Select-Object LocalPort,OwningProcess | Sort-Object LocalPort,OwningProcess -Unique | ConvertTo-Csv -NoTypeInformation`, "udp", processNames)

	items := append(tcp.items, udp.items...)
	var warnings []CollectionWarning
	if len(items) == 0 {
		switch {
		case tcp.err != nil && udp.err != nil:
			return runtimeCollectorResult{Err: errors.Join(tcp.err, udp.err)}
		case tcp.err != nil:
			return runtimeCollectorResult{Err: tcp.err}
		case udp.err != nil:
			return runtimeCollectorResult{Err: udp.err}
		}
	}
	missingOwners := countRuntimePortsMissingOwnerInfo(items)
	if missingOwners > 0 {
		ownerErr := errors.Join(processErr, tcp.ownerErr, udp.ownerErr)
		if ownerErr == nil {
			ownerErr = fmt.Errorf("resolved runtime ports but could not determine complete owner info for all ports")
		}
		warnings = append(warnings, ownerResolutionWarning(items, ownerErr.Error()))
	}
	return runtimeCollectorResult{Ports: items, Warnings: warnings}
}

func collectWindowsPorts(script string, protocol string, processNames map[int]string) []runtimePort {
	items, _ := collectWindowsPortsErr(script, protocol, processNames)
	return items
}

func collectWindowsPortsErr(script string, protocol string, processNames map[int]string) ([]runtimePort, error) {
	result := collectWindowsPortsDetailed(script, protocol, processNames)
	return result.items, result.err
}

type windowsPortCollectResult struct {
	items    []runtimePort
	err      error
	ownerErr error
}

func collectWindowsPortsDetailed(script string, protocol string, processNames map[int]string) windowsPortCollectResult {
	out, err := runPowerShell(script)
	if err != nil {
		return windowsPortCollectResult{err: err}
	}
	reader := csv.NewReader(bytes.NewReader(out))
	records, err := reader.ReadAll()
	if err != nil {
		return windowsPortCollectResult{err: err}
	}
	if len(records) == 0 {
		if strings.TrimSpace(string(out)) == "" {
			return windowsPortCollectResult{}
		}
		return windowsPortCollectResult{err: fmt.Errorf("unexpected empty csv output")}
	}
	if err := validateWindowsPortCSVHeader(records[0]); err != nil {
		return windowsPortCollectResult{err: err}
	}
	if len(records) == 1 {
		return windowsPortCollectResult{}
	}
	items := make([]runtimePort, 0, len(records)-1)
	validRows := 0
	var ownerErr error
	for _, record := range records[1:] {
		if len(record) < 2 {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(record[0]))
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		validRows++
		pid, _ := strconv.Atoi(strings.TrimSpace(record[1]))
		name := processNames[pid]
		if pid > 0 && name == "" {
			var lookupErr error
			name, lookupErr = lookupWindowsProcessNameErr(pid)
			if lookupErr != nil {
				ownerErr = errors.Join(ownerErr, fmt.Errorf("lookup process name for pid %d failed: %w", pid, lookupErr))
			}
			processNames[pid] = name
		}
		items = append(items, runtimePort{
			Port:        port,
			Protocol:    protocol,
			PID:         pid,
			ServiceName: name,
		})
	}
	if len(records) > 1 && validRows == 0 {
		return windowsPortCollectResult{err: fmt.Errorf("no valid port rows in command output")}
	}
	return windowsPortCollectResult{items: items, ownerErr: ownerErr}
}

func validateWindowsPortCSVHeader(header []string) error {
	if len(header) < 2 {
		return fmt.Errorf("unexpected port csv header column count")
	}
	first := strings.Trim(strings.TrimSpace(header[0]), "\"")
	second := strings.Trim(strings.TrimSpace(header[1]), "\"")
	if first != "LocalPort" || second != "OwningProcess" {
		return fmt.Errorf("unexpected port csv header %q,%q", first, second)
	}
	return nil
}

func collectWindowsProcessNames() map[int]string {
	result, _ := collectWindowsProcessNamesErr()
	return result
}

func collectWindowsProcessNamesErr() (map[int]string, error) {
	out, err := runPowerShell(`Get-Process | Select-Object Id,ProcessName | Sort-Object Id -Unique | ConvertTo-Csv -NoTypeInformation`)
	if err != nil {
		return map[int]string{}, err
	}
	reader := csv.NewReader(bytes.NewReader(out))
	records, err := reader.ReadAll()
	if err != nil {
		return map[int]string{}, err
	}
	if len(records) == 0 {
		if strings.TrimSpace(string(out)) == "" {
			return map[int]string{}, nil
		}
		return map[int]string{}, fmt.Errorf("unexpected empty process csv output")
	}
	if err := validateWindowsProcessCSVHeader(records[0]); err != nil {
		return map[int]string{}, err
	}
	if len(records) == 1 {
		return map[int]string{}, nil
	}
	result := make(map[int]string, len(records)-1)
	for _, record := range records[1:] {
		if len(record) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(record[0]))
		if err != nil || pid <= 0 {
			continue
		}
		name := strings.TrimSpace(record[1])
		if name == "" {
			continue
		}
		result[pid] = name
	}
	return result, nil
}

func validateWindowsProcessCSVHeader(header []string) error {
	if len(header) < 2 {
		return fmt.Errorf("unexpected process csv header column count")
	}
	first := strings.Trim(strings.TrimSpace(header[0]), "\"")
	second := strings.Trim(strings.TrimSpace(header[1]), "\"")
	if first != "Id" || second != "ProcessName" {
		return fmt.Errorf("unexpected process csv header %q,%q", first, second)
	}
	return nil
}

func countWindowsRuntimePortsWithPID(items []runtimePort) int {
	count := 0
	for _, item := range items {
		if item.PID > 0 {
			count++
		}
	}
	return count
}

func countWindowsRuntimePortsWithServiceName(items []runtimePort) int {
	count := 0
	for _, item := range items {
		if strings.TrimSpace(item.ServiceName) != "" {
			count++
		}
	}
	return count
}
