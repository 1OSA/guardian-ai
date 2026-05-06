package config

import (
	"testing"
)

func TestIsMLAllowlisted(t *testing.T) {
	tests := []struct {
		domain   string
		expected bool
	}{
		// Exact matches from extra allowlist
		{"google.com", true},
		{"GOOGLE.COM", true}, // Case insensitivity check
		{"apple.com", true},
		{"netflix.com", true}, // From predefined services

		// Not in allowlist
		{"malicious-site.com", false},
		{"unknown-domain.net", false},

		// Subdomain matches (these should be allowlisted if the base domain is)
		{"mail.google.com", true},
		{"cdn.netflix.com", true},
		{"api.github.com", true},

		// Edge cases
		{"", false},
		{"google.com.br", false}, // Assuming it's not explicitly in the list
		{"com", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			result := IsMLAllowlisted(tt.domain)
			if result != tt.expected {
				t.Errorf("IsMLAllowlisted(%q) = %v; expected %v", tt.domain, result, tt.expected)
			}
		})
	}
}
