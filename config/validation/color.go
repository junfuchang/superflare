package validation

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	hexColorPattern  = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
	rgbColorPattern  = regexp.MustCompile(`^rgba?\((.*)\)$`)
	cssNumberPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)
)

func SafeCSSColor(input string, fallback string) string {
	input = strings.TrimSpace(input)
	if hexColorPattern.MatchString(input) || safeRGBColor(input) {
		return input
	}
	return fallback
}

func safeRGBColor(input string) bool {
	match := rgbColorPattern.FindStringSubmatch(input)
	if match == nil {
		return false
	}
	parts := strings.Split(match[1], ",")
	if len(parts) != 3 && len(parts) != 4 {
		return false
	}
	for i := 0; i < 3; i++ {
		value := strings.TrimSpace(parts[i])
		if strings.HasSuffix(value, "%") {
			num, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
			if err != nil || num < 0 || num > 100 {
				return false
			}
			continue
		}
		if !cssNumberPattern.MatchString(value) {
			return false
		}
		num, err := strconv.Atoi(strings.Split(value, ".")[0])
		if err != nil || num < 0 || num > 255 {
			return false
		}
		if strings.Contains(value, ".") {
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil || parsed < 0 || parsed > 255 {
				return false
			}
		}
	}
	if len(parts) == 4 {
		alpha := strings.TrimSpace(parts[3])
		if strings.HasSuffix(alpha, "%") {
			num, err := strconv.ParseFloat(strings.TrimSuffix(alpha, "%"), 64)
			return err == nil && num >= 0 && num <= 100
		}
		num, err := strconv.ParseFloat(alpha, 64)
		return err == nil && num >= 0 && num <= 1
	}
	return true
}
