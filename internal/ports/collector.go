package ports

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/model"
)

type runtimePort struct {
	Port        int
	Protocol    string
	ServiceName string
	PID         int
}

type CollectionWarning struct {
	Code          string
	Detail        string
	MissingOwners int
	RuntimePorts  int
}

type CollectionReport struct {
	Items    []model.PortInfo
	Warnings []CollectionWarning
}

type runtimeCollectorResult struct {
	Ports    []runtimePort
	Warnings []CollectionWarning
	Err      error
}

var collectRuntimePortsErr = collectRuntimePortsResult

func unsupportedRuntimeCollectorResult() runtimeCollectorResult {
	return runtimeCollectorResult{
		Err: fmt.Errorf("runtime port collection is not supported on this platform"),
	}
}

func CollectErr() ([]model.PortInfo, error) {
	return CollectWithHiddenErr(false)
}

func CollectWithHiddenErr(includeHidden bool) ([]model.PortInfo, error) {
	bindings, err := data.GetPortBindingMapWithError()
	if err != nil {
		return nil, fmt.Errorf("load port bindings failed: %w", err)
	}
	return CollectWithBindingsErr(bindings, includeHidden)
}

func CollectReportWithBindingsErr(bindings map[string]model.PortBinding, includeHidden bool) (CollectionReport, error) {
	result := collectRuntimePortsErr()
	if result.Err != nil {
		return CollectionReport{}, fmt.Errorf("collect runtime ports failed: %w", result.Err)
	}
	runtimePorts := result.Ports
	return CollectionReport{
		Items:    MergeRuntimeAndBindings(runtimePorts, bindings, includeHidden),
		Warnings: result.Warnings,
	}, nil
}

func CollectWithBindingsErr(bindings map[string]model.PortBinding, includeHidden bool) ([]model.PortInfo, error) {
	report, err := CollectReportWithBindingsErr(bindings, includeHidden)
	if err != nil {
		return nil, err
	}
	return report.Items, nil
}

func MergeRuntimeAndBindings(runtimePorts []runtimePort, bindings map[string]model.PortBinding, includeHidden bool) []model.PortInfo {
	merged := map[string]model.PortInfo{}
	for _, item := range runtimePorts {
		if item.Port <= 0 || item.Port > 65535 {
			continue
		}
		protocol := normalizeProtocol(item.Protocol)
		key := bindingKey(protocol, item.Port)
		binding := bindings[key]
		if binding.Hidden && !includeHidden {
			continue
		}
		info := model.PortInfo{
			Port:        item.Port,
			Protocol:    protocol,
			ServiceName: strings.TrimSpace(item.ServiceName),
			Running:     true,
			PID:         item.PID,
			Remark:      strings.TrimSpace(binding.Remark),
			Hidden:      binding.Hidden,
		}
		if prev, ok := merged[key]; ok && runtimePortInfoScore(prev) >= runtimePortInfoScore(info) {
			info.PID = prev.PID
			info.ServiceName = prev.ServiceName
		}
		merged[key] = info
	}
	for key, binding := range bindings {
		if _, ok := merged[key]; ok {
			continue
		}
		if binding.Hidden && !includeHidden {
			continue
		}
		protocol, port, ok := splitBindingKey(key)
		if !ok {
			continue
		}
		merged[key] = model.PortInfo{
			Port:     port,
			Protocol: protocol,
			Running:  false,
			Remark:   strings.TrimSpace(binding.Remark),
			Hidden:   binding.Hidden,
		}
	}
	result := make([]model.PortInfo, 0, len(merged))
	for _, item := range merged {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Port == result[j].Port {
			return result[i].Protocol < result[j].Protocol
		}
		return result[i].Port < result[j].Port
	})
	return result
}

func runtimePortInfoScore(item model.PortInfo) int {
	score := 0
	if item.PID > 0 {
		score++
	}
	if strings.TrimSpace(item.ServiceName) != "" {
		score += 2
	}
	return score
}

func countRuntimePortsMissingOwnerInfo(items []runtimePort) int {
	count := 0
	for _, item := range items {
		if item.Port <= 0 || item.Port > 65535 {
			continue
		}
		if item.PID <= 0 || strings.TrimSpace(item.ServiceName) == "" {
			count++
		}
	}
	return count
}

func ownerResolutionWarning(items []runtimePort, detail string) CollectionWarning {
	return CollectionWarning{
		Code:          "owner_resolution_partial",
		Detail:        strings.TrimSpace(detail),
		MissingOwners: countRuntimePortsMissingOwnerInfo(items),
		RuntimePorts:  len(items),
	}
}

func LocalLANHost() string {
	if host := firstPrivateIPv4(); host != "" {
		return host
	}
	return "127.0.0.1"
}

func firstPrivateIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 != nil && ip4.IsPrivate() {
				return ip4.String()
			}
		}
	}
	return ""
}
