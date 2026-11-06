package httpapi

import (
	"testing"
)

func TestEventCursorMatchesPostgresRange(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want uint64
		fail bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"42", 42, false},
		{"9223372036854775807", 1<<63 - 1, false},
		{"9223372036854775808", 0, true},
		{"18446744073709551615", 0, true},
		{"-1", 0, true},
		{"invalid", 0, true},
		{" 1", 0, true},
	} {
		t.Run(test.raw, func(t *testing.T) {
			got, err := parseEventCursor(test.raw)
			if (err != nil) != test.fail || (!test.fail && got != test.want) {
				t.Fatalf("cursor %q: got %d, %v", test.raw, got, err)
			}
		})
	}
}
