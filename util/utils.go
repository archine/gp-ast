package util

import "strings"

// SplitAndTrim splits a string by commas, trims whitespace and slashes from each item,
func SplitAndTrim(input string) []string {
	if input == "" {
		return nil
	}

	split := strings.Split(input, ",")
	result := make([]string, 0, len(split))

	for _, item := range split {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "/")
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
