package views

import (
	"strings"
	"time"
)

func joinStrings(items []string) string {
	return strings.Join(items, ", ")
}

func formatBirthday(s string) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return t.Format("January 2, 2006")
}
