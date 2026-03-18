package template

import "strings"

// Interpolate replaces all {key} placeholders in s with values from vars.
func Interpolate(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// InterpolateMap applies Interpolate to all values of a map.
func InterpolateMap(m map[string]string, vars map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = Interpolate(v, vars)
	}
	return result
}

// InterpolateSlice applies Interpolate to all elements of a slice.
func InterpolateSlice(ss []string, vars map[string]string) []string {
	result := make([]string, len(ss))
	for i, s := range ss {
		result[i] = Interpolate(s, vars)
	}
	return result
}
