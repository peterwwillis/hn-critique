package ai

import "testing"

func TestSanitizeRating(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "lowercase", in: "questionable", want: "questionable"},
		{name: "uppercase", in: "Reliable", want: "reliable"},
		{name: "trailing-space", in: "misleading ", want: "misleading"},
		{name: "punctuation", in: "reliable.", want: "reliable"},
		{name: "unavailable", in: "UNAVAILABLE", want: "unavailable"},
		{name: "unknown", in: "not reliable", want: "questionable"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeRating(tc.in); got != tc.want {
				t.Errorf("sanitizeRating(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
