package bridge

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// weight for ordering
var classOrder = []struct {
	Size   string
	Weight int
}{
	{"nano", 0},
	{"micro", 1},
	{"small", 2},
	{"medium", 3},
	{"large", 4},
	{"xlarge", 5},
	{"metal", 9999},
}

// instanceNameWeight creates a sortable string for instance classes
func instanceNameWeight(name string) string {
	parts := strings.Split(name, ".")
	var class string
	var size string
	// remove db. or cache. prefix
	if len(parts) == 3 {
		class = parts[1]
		size = parts[2]
	} else {
		class = parts[0]
		size = parts[1]
	}
	// extract multiplier (8xlarge) from size
	re := regexp.MustCompile(`\d+`)
	multiplier := re.FindString(size)
	if multiplier != "" {
		num, err := strconv.Atoi(multiplier)
		if err != nil {
			return name
		}
		// multiply multiplier by 10 to bump it above the ones without multipliers
		return fmt.Sprintf("%s.%05d", class, num*10)
	}
	// determine string from static classOrder list
	for _, o := range classOrder {
		if size == o.Size {
			return fmt.Sprintf("%s.%05d", class, o.Weight)
		}
	}
	return name
}

// SortInstanceClasses sorts a slice of AWS instance class names
func SortInstanceClasses(classes []string) {
	sort.Slice(classes, func(i, j int) bool {
		return instanceNameWeight(classes[i]) < instanceNameWeight(classes[j])
	})
}
