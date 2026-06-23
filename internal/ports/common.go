package ports

import (
	"strconv"
	"strings"
)

func normalizeProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case "udp":
		return "udp"
	default:
		return "tcp"
	}
}

func bindingKey(protocol string, port int) string {
	return normalizeProtocol(protocol) + ":" + strconv.Itoa(port)
}

func splitBindingKey(key string) (string, int, bool) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, false
	}
	return normalizeProtocol(parts[0]), port, true
}
