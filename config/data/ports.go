package data

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/junfuchang/superflare/config/model"
)

func LoadPortBindings() (model.Ports, error) {
	var result model.Ports
	filePath, err := portsConfigPath()
	if err != nil {
		return result, err
	}
	exists, err := pathExists(filePath)
	if err != nil {
		return result, fmt.Errorf("stat ports config failed: %w", err)
	}
	if !exists {
		return result, fmt.Errorf("ports config is missing")
	}
	configFile, err := readFileCached(filePath, func() ([]byte, error) { return readFile(filePath) })
	if err != nil {
		return result, err
	}
	if err := yaml.Unmarshal(configFile, &result); err != nil {
		return result, err
	}
	if err := validateLoadedPortBindings(result.Items); err != nil {
		return result, err
	}
	result.Items = normalizePortBindings(result.Items)
	return result, nil
}

func LoadPortBindingsFromRaw(raw []byte) (model.Ports, error) {
	var result model.Ports
	if err := yaml.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("parse ports raw failed: %w", err)
	}
	if err := validateLoadedPortBindings(result.Items); err != nil {
		return result, err
	}
	result.Items = normalizePortBindings(result.Items)
	return result, nil
}

func SavePortBindings(data model.Ports) error {
	return withConfigWriteLock(func() error {
		data.Items = normalizePortBindings(data.Items)
		out, err := yaml.Marshal(data)
		if err != nil {
			log.Println("marshal ports failed")
			return fmt.Errorf("marshal ports failed: %w", err)
		}
		filePath, err := portsConfigPath()
		if err != nil {
			return err
		}
		if err := saveFileLocked(filePath, out); err != nil {
			log.Println("save ports failed")
			return fmt.Errorf("save ports failed: %w", err)
		}
		invalidateFileCachePath(filePath)
		return nil
	})
}

func UpdatePortRemarks(items []model.PortBinding) error {
	return SavePortBindings(model.Ports{Items: items})
}

func GetPortBindingMapWithError() (map[string]model.PortBinding, error) {
	ports, err := LoadPortBindings()
	if err != nil {
		return map[string]model.PortBinding{}, err
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
	return bindings, nil
}

func GetPortRemarkMapWithError() (map[string]string, error) {
	bindings, err := GetPortBindingMapWithError()
	if err != nil {
		return map[string]string{}, err
	}
	remarks := make(map[string]string, len(bindings))
	for key, item := range bindings {
		remarks[key] = strings.TrimSpace(item.Remark)
	}
	return remarks, nil
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

func validateLoadedPortBindings(items []model.PortBinding) error {
	for index, item := range items {
		if item.Port <= 0 || item.Port > 65535 {
			return fmt.Errorf("invalid port binding at row %d: port %d is out of range", index+1, item.Port)
		}
		protocol := strings.ToLower(strings.TrimSpace(item.Protocol))
		switch protocol {
		case "", "tcp", "udp":
		default:
			return fmt.Errorf("invalid port binding at row %d: protocol %q is not supported", index+1, item.Protocol)
		}
	}
	return nil
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

func portsConfigPath() (string, error) {
	rootDir, err := configRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(rootDir, "ports.yaml"), nil
}

func getPortsConfigPath() string {
	rootDir, err := configRootDir()
	if err != nil {
		return filepath.Join(".", "ports.yaml")
	}
	return filepath.Join(rootDir, "ports.yaml")
}

func GetPortsConfigPathErr() (string, error) {
	return portsConfigPath()
}
