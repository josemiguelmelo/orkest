package sliceutil

// Map preserves order and distinguishes nil from a non-nil empty slice.
func Map[S, D any](values []S, convert func(S) D) []D {
	if values == nil {
		return nil
	}
	result := make([]D, len(values))
	for i, value := range values {
		result[i] = convert(value)
	}
	return result
}
