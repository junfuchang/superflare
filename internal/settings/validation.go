package settings

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/junfuchang/superflare/config/define"
)

func ParseOptionalColor(input string, field string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}
	if define.SafeCSSColor(input, "") == "" {
		if strings.TrimSpace(field) == "" {
			return "", fmt.Errorf("invalid color value: %s", input)
		}
		return "", fmt.Errorf("invalid %s value: %s", field, input)
	}
	return input, nil
}

func ParseOptionalRangedInt(input string, min int, max int, field string) (int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(input)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value: %s", field, input)
	}
	if value < min || value > max {
		return 0, fmt.Errorf("%s must be between %d and %d", field, min, max)
	}
	return value, nil
}
