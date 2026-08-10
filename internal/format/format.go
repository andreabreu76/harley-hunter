package format

import (
	"strconv"
	"strings"
)

const countryCode = "+55"

func Phone(digits string) string {
	switch len(digits) {
	case 10, 11:
		return "(" + digits[:2] + ") " + digits[2:len(digits)-4] + "-" + digits[len(digits)-4:]
	}
	return digits
}

func PhoneLink(digits string) string {
	if digits == "" {
		return ""
	}
	return countryCode + digits
}

func Thousands(value int64) string {
	digits := strconv.FormatInt(value, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)
	return sign + strings.Join(parts, ".")
}
