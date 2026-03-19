package helpers

func ApplyIfSet[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}
