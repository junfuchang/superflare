//go:build windows

package ports

import (
	"errors"
	"strings"
	"testing"
)

func TestCollectWindowsProcessNamesParsesCSV(t *testing.T) {
	originalRunner := runPowerShell
	defer func() { runPowerShell = originalRunner }()

	runPowerShell = func(script string) ([]byte, error) {
		return []byte("\"Id\",\"ProcessName\"\r\n\"3136\",\"svchost\"\r\n\"5668\",\"superflare\"\r\n"), nil
	}

	got := collectWindowsProcessNames()
	if got[3136] != "svchost" || got[5668] != "superflare" {
		t.Fatalf("unexpected process names: %#v", got)
	}
}

func TestCollectWindowsProcessNamesErrReturnsErrorWhenHeaderInvalid(t *testing.T) {
	originalRunner := runPowerShell
	defer func() { runPowerShell = originalRunner }()

	runPowerShell = func(script string) ([]byte, error) {
		return []byte("\"PID\",\"Name\"\r\n\"3136\",\"svchost\"\r\n"), nil
	}

	_, err := collectWindowsProcessNamesErr()
	if err == nil {
		t.Fatal("expected invalid process header error")
	}
}

func TestCollectWindowsPortsUsesBulkProcessNames(t *testing.T) {
	processNames := map[int]string{
		3136: "svchost",
		5668: "superflare",
	}

	originalRunner := runPowerShell
	defer func() { runPowerShell = originalRunner }()

	runPowerShell = func(script string) ([]byte, error) {
		return []byte("\"LocalPort\",\"OwningProcess\"\r\n\"53\",\"3136\"\r\n\"5668\",\"5668\"\r\n"), nil
	}

	got := collectWindowsPorts("dummy", "tcp", processNames)
	if len(got) != 2 {
		t.Fatalf("unexpected ports: %#v", got)
	}
	if got[0].Port != 53 || got[0].PID != 3136 || got[0].ServiceName != "svchost" {
		t.Fatalf("unexpected first port: %#v", got[0])
	}
	if got[1].Port != 5668 || got[1].PID != 5668 || got[1].ServiceName != "superflare" {
		t.Fatalf("unexpected second port: %#v", got[1])
	}
}

func TestCollectWindowsPortsErrAllowsHeaderOnlyEmptyResult(t *testing.T) {
	originalRunner := runPowerShell
	defer func() { runPowerShell = originalRunner }()

	runPowerShell = func(script string) ([]byte, error) {
		return []byte("\"LocalPort\",\"OwningProcess\"\r\n"), nil
	}

	got, err := collectWindowsPortsErr("dummy", "tcp", map[int]string{})
	if err != nil {
		t.Fatalf("collectWindowsPortsErr: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %#v", got)
	}
}

func TestCollectWindowsPortsErrReturnsErrorWhenHeaderInvalid(t *testing.T) {
	originalRunner := runPowerShell
	defer func() { runPowerShell = originalRunner }()

	runPowerShell = func(script string) ([]byte, error) {
		return []byte("\"Port\",\"Process\"\r\n\"53\",\"3136\"\r\n"), nil
	}

	_, err := collectWindowsPortsErr("dummy", "tcp", map[int]string{})
	if err == nil {
		t.Fatal("expected invalid header error")
	}
}

func TestCollectWindowsPortsErrReturnsErrorWhenRowsPresentButNoValidPorts(t *testing.T) {
	originalRunner := runPowerShell
	defer func() { runPowerShell = originalRunner }()

	runPowerShell = func(script string) ([]byte, error) {
		return []byte("\"LocalPort\",\"OwningProcess\"\r\n\"not-a-port\",\"3136\"\r\n"), nil
	}

	_, err := collectWindowsPortsErr("dummy", "tcp", map[int]string{})
	if err == nil {
		t.Fatal("expected invalid rows error")
	}
}

func TestCollectRuntimePortsResultReturnsErrorWhenWindowsPortCommandsFail(t *testing.T) {
	originalRunner := runPowerShell
	defer func() { runPowerShell = originalRunner }()

	runPowerShell = func(script string) ([]byte, error) {
		return nil, errors.New("powershell unavailable")
	}

	got := collectRuntimePortsResult()
	if got.Err == nil {
		t.Fatal("expected runtime port collection failure")
	}
	if len(got.Ports) != 0 {
		t.Fatalf("expected no runtime ports on failure, got %#v", got.Ports)
	}
}

func TestCollectRuntimePortsResultUsesPerPIDProcessLookupWhenBulkProcessCollectionFails(t *testing.T) {
	originalRunner := runPowerShell
	originalLookup := lookupWindowsProcessNameErr
	defer func() {
		runPowerShell = originalRunner
		lookupWindowsProcessNameErr = originalLookup
	}()

	runPowerShell = func(script string) ([]byte, error) {
		switch {
		case strings.Contains(script, "Get-Process |"):
			return nil, errors.New("bulk process query unavailable")
		case strings.Contains(script, "Get-NetTCPConnection"):
			return []byte("\"LocalPort\",\"OwningProcess\"\r\n\"5668\",\"4321\"\r\n"), nil
		case strings.Contains(script, "Get-NetUDPEndpoint"):
			return []byte("\"LocalPort\",\"OwningProcess\"\r\n"), nil
		default:
			return nil, errors.New("unexpected script")
		}
	}
	lookupWindowsProcessNameErr = func(pid int) (string, error) {
		if pid != 4321 {
			return "", errors.New("unexpected pid")
		}
		return "superflare", nil
	}

	got := collectRuntimePortsResult()
	if got.Err != nil {
		t.Fatalf("expected runtime port collection success, got %v", got.Err)
	}
	if len(got.Ports) != 1 || got.Ports[0].ServiceName != "superflare" || got.Ports[0].PID != 4321 {
		t.Fatalf("unexpected runtime ports: %#v", got.Ports)
	}
}

func TestCollectRuntimePortsResultWarnsWhenNoWindowsOwnerNamesResolved(t *testing.T) {
	originalRunner := runPowerShell
	originalLookup := lookupWindowsProcessNameErr
	defer func() {
		runPowerShell = originalRunner
		lookupWindowsProcessNameErr = originalLookup
	}()

	runPowerShell = func(script string) ([]byte, error) {
		switch {
		case strings.Contains(script, "Get-Process |"):
			return nil, errors.New("bulk process query unavailable")
		case strings.Contains(script, "Get-NetTCPConnection"):
			return []byte("\"LocalPort\",\"OwningProcess\"\r\n\"5668\",\"4321\"\r\n"), nil
		case strings.Contains(script, "Get-NetUDPEndpoint"):
			return []byte("\"LocalPort\",\"OwningProcess\"\r\n\"53\",\"3136\"\r\n"), nil
		default:
			return nil, errors.New("unexpected script")
		}
	}
	lookupWindowsProcessNameErr = func(pid int) (string, error) {
		return "", errors.New("lookup denied")
	}

	got := collectRuntimePortsResult()
	if got.Err != nil {
		t.Fatalf("expected partial owner resolution warning, got error %v", got.Err)
	}
	if len(got.Ports) != 2 {
		t.Fatalf("expected runtime ports despite owner lookup failure, got %#v", got.Ports)
	}
	if len(got.Warnings) != 1 {
		t.Fatalf("expected one owner resolution warning, got %#v", got.Warnings)
	}
	if got.Warnings[0].Code != "owner_resolution_partial" {
		t.Fatalf("unexpected warning code: %#v", got.Warnings[0])
	}
	if got.Warnings[0].MissingOwners != 2 || got.Warnings[0].RuntimePorts != 2 {
		t.Fatalf("unexpected warning counts: %#v", got.Warnings[0])
	}
}

func TestCollectRuntimePortsResultReturnsErrorWhenWindowsPortCommandOutputMalformed(t *testing.T) {
	originalRunner := runPowerShell
	defer func() { runPowerShell = originalRunner }()

	runPowerShell = func(script string) ([]byte, error) {
		switch {
		case strings.Contains(script, "Get-Process"):
			return []byte("\"Id\",\"ProcessName\"\r\n\"3136\",\"svchost\"\r\n"), nil
		case strings.Contains(script, "Get-NetTCPConnection"):
			return []byte("\"Port\",\"Process\"\r\n\"53\",\"3136\"\r\n"), nil
		case strings.Contains(script, "Get-NetUDPEndpoint"):
			return []byte("\"Port\",\"Process\"\r\n\"67\",\"3136\"\r\n"), nil
		default:
			return nil, errors.New("unexpected script")
		}
	}

	got := collectRuntimePortsResult()
	if got.Err == nil {
		t.Fatal("expected malformed windows port output failure")
	}
	if len(got.Ports) != 0 {
		t.Fatalf("expected no runtime ports on malformed output, got %#v", got.Ports)
	}
}
