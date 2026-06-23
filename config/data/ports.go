package data

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/junfuchang/superflare/config/model"
)

func LoadPortBindings() (model.Ports, error) {
	var result model.Ports
	filePath := getPortsConfigPath()
	if !checkExists(filePath) {
		out, err := yaml.Marshal(result)
		if err != nil {
			return result, err
		}
		if !saveFile(filePath, out) {
			return result, fmt.Errorf("init ports config failed: %s", filePath)
		}
		return result, nil
	}
	configFile, err := readFileCached("ports", func() ([]byte, error) { return readFile(filePath) })
	if err != nil {
		return result, err
	}
	if err := yaml.Unmarshal(configFile, &result); err != nil {
		return result, err
	}
	result.Items = normalizePortBindings(result.Items)
	return result, nil
}

func SavePortBindings(data model.Ports) bool {
	data.Items = normalizePortBindings(data.Items)
	out, err := yaml.Marshal(data)
	if err != nil {
		log.Println("marshal ports failed")
		return false
	}
	if !saveFile(getPortsConfigPath(), out) {
		log.Println("save ports failed")
		return false
	}
	invalidateFileCache("ports")
	return true
}

func UpdatePortRemarks(items []model.PortBinding) bool {
	return SavePortBindings(model.Ports{Items: items})
}

func GetPortBindingMap() map[string]model.PortBinding {
	ports, err := LoadPortBindings()
	if err != nil {
		return map[string]model.PortBinding{}
	}
	bindings := make(map[string]model.PortBinding, len(ports.Items))
	for _, item := range ports.Items {
		if item.Port <= 0 {
			continue
		}
		item.Protocol = normalizePortProtocol(item.Protocol)
		item.Remark = strings.TrimSpace(item.Remark)
		bindings[portBindingKey(item.Protocol, item.Port)] = item
	}
	return bindings
}

func GetPortRemarkMap() map[string]string {
	bindings := GetPortBindingMap()
	remarks := make(map[string]string, len(bindings))
	for key, item := range bindings {
		remarks[key] = strings.TrimSpace(item.Remark)
	}
	return remarks
}

func normalizePortBindings(items []model.PortBinding) []model.PortBinding {
	unique := map[string]model.PortBinding{}
	for _, item := range items {
		if item.Port <= 0 || item.Port > 65535 {
			continue
		}
		item.Protocol = normalizePortProtocol(item.Protocol)
		item.Remark = strings.TrimSpace(item.Remark)
		if item.Remark == "" && !item.Hidden {
			continue
		}
		key := portBindingKey(item.Protocol, item.Port)
		unique[key] = item
	}
	result := make([]model.PortBinding, 0, len(unique))
	for _, item := range unique {
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

func normalizePortProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case "udp":
		return "udp"
	default:
		return "tcp"
	}
}

func portBindingKey(protocol string, port int) string {
	return normalizePortProtocol(protocol) + ":" + strconv.Itoa(port)
}

func GetPortsConfigPath() string {
	return getPortsConfigPath()
}

func getPortsConfigPath() string {
	rootDir, err := os.Getwd()
	if err != nil {
		return filepath.Join(".", "ports.yaml")
	}
	return filepath.Join(rootDir, "ports.yaml")
}
