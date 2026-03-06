package generator

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseByteSize parses a human-readable byte size string (e.g., "512KB", "10MB", "1GB")
// and returns the size in bytes.
func ParseByteSize(value string) (int64, error) {
	clean := strings.TrimSpace(strings.ToUpper(value))
	if clean == "" {
		return 0, errors.New("empty size value")
	}

	var numberPart strings.Builder
	var unitPart strings.Builder
	for _, r := range clean {
		switch {
		case (r >= '0' && r <= '9') || r == '.':
			numberPart.WriteRune(r)
		case r == '_' || r == ',':
			continue
		default:
			unitPart.WriteRune(r)
		}
	}

	if numberPart.Len() == 0 {
		return 0, errors.New("missing numeric portion")
	}

	parsedNum, err := strconv.ParseFloat(numberPart.String(), 64)
	if err != nil {
		return 0, err
	}

	unit := unitPart.String()
	multiplier := int64(1)
	switch unit {
	case "", "B":
		multiplier = 1
	case "K", "KB":
		multiplier = 1 << 10
	case "M", "MB":
		multiplier = 1 << 20
	case "G", "GB":
		multiplier = 1 << 30
	case "T", "TB":
		multiplier = 1 << 40
	default:
		return 0, fmt.Errorf("unknown size unit %q", unit)
	}

	result := int64(parsedNum * float64(multiplier))
	if result < 0 {
		return 0, errors.New("size must be positive")
	}
	return result, nil
}

// chooseBufferSize returns an appropriate buffer size based on max file size
func chooseBufferSize(maxFileSize int64) int {
	switch {
	case maxFileSize >= 64*1024*1024:
		return 4 * 1024 * 1024
	case maxFileSize >= 8*1024*1024:
		return 2 * 1024 * 1024
	default:
		return 1 * 1024 * 1024
	}
}
