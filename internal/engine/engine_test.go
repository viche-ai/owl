package engine

import "testing"

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Code Reviewer",
			expected: "code-reviewer",
		},
		{
			input:    "A Very Long Name That Exceeds Thirty Characters limit",
			expected: "a-very-long-name-that-exceeds-",
		},
		{
			input:    "already-good",
			expected: "already-good",
		},
		{
			input:    "MIXED Case Name",
			expected: "mixed-case-name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			actual := sanitizeName(tc.input)
			if actual != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}
