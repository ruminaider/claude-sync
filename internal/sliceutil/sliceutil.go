package sliceutil

// AppendUnique appends items from add to base, skipping duplicates.
// Returns a new slice; does not modify the original.
func AppendUnique(base, add []string) []string {
	if len(add) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	result := make([]string, len(base))
	copy(result, base)
	for _, s := range add {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// RemoveAll returns base with all items in remove filtered out.
func RemoveAll(base, remove []string) []string {
	if len(remove) == 0 {
		return base
	}
	drop := make(map[string]bool, len(remove))
	for _, s := range remove {
		drop[s] = true
	}
	var result []string
	for _, s := range base {
		if !drop[s] {
			result = append(result, s)
		}
	}
	return result
}
