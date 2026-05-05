package app

import (
	"errors"
	"strconv"
	"strings"
)

// ParsePositiveID parses a string into a positive int64 id. Returns an error
// for non-numeric, zero, or negative inputs. Used by the CLI surface to
// validate `get <id>` / `update <id>` arguments before touching the service.
func ParsePositiveID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, errors.New("id must be a positive integer")
	}
	if id <= 0 {
		return 0, errors.New("id must be a positive integer")
	}
	return id, nil
}

// FormatNote renders a MemoryItem for human-readable CLI output, using UTC
// ISO timestamps so output is stable across locales and timezones.
func FormatNote(it MemoryItem) string {
	const layout = "2006-01-02T15:04:05Z"
	return "#" + strconv.FormatInt(it.ID, 10) +
		" created=" + it.CreatedAt.UTC().Format(layout) +
		" updated=" + it.UpdatedAt.UTC().Format(layout) +
		"\n" + it.Content
}
