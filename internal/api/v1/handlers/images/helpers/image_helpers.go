package helpers

func HasRealTag(tags []string) bool {
	for _, t := range tags {
		if t != "" && t != "<none>" && t != "<none>:<none>" {
			return true
		}
	}
	return false
}

func IsInternalImage(tags []string, internalImages map[string]bool) bool {
	for _, t := range tags {
		if internalImages[t] {
			return true
		}
	}
	return false
}
