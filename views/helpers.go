package views

import (
	"fmt"
	"strings"
	"time"
)

func joinStrings(items []string) string {
	return strings.Join(items, ", ")
}

// countNoun renders "1 photo" / "3 photos" -- both nouns used on the
// party pulse line (photo, note) pluralize with a plain "s".
func countNoun(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func formatBirthday(s string) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return t.Format("January 2, 2006")
}
