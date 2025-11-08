package stringslice

func Contains(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}

	return false
}

// deduplicate removes duplicates from a slice of strings
func Deduplicate(slice []string) ([]string, []string) {
	seen := make(map[string]bool)

	var (
		result []string
		dupes  []string
	)

	for _, s := range slice {
		if seen[s] {
			dupes = append(dupes, s)

			continue
		}

		seen[s] = true
		result = append(result, s)
	}

	return result, dupes
}
