package catalog

import (
	"strconv"
	"strings"
)

// ParseSlugID reads the id a slug starts with ("155-the-dark-knight" -> 155).
func ParseSlugID(slug string) (int, bool) {
	head, _, _ := strings.Cut(slug, "-")
	id, err := strconv.Atoi(head)
	return id, err == nil && id > 0
}
