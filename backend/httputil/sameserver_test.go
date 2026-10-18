package httputil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSameServer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"https://dav.example.com/x", "https://DAV.example.com/y", true},
		{"https://dav.example.com", "https://dav.example.com:443/", true},
		{"http://dav.example.com", "http://dav.example.com:80", true},
		{"webcal://cal.example.com/a.ics", "https://cal.example.com/b.ics", true},
		{"https://dav.example.com", "http://dav.example.com", false},
		{"https://dav.example.com", "https://dav.example.com:8443", false},
		{"https://dav.example.com", "https://other.example.com", false},
		{"https://dav.example.com", "http://dav.example.com:443", false},
		{"", "https://dav.example.com", false},
		{"://bad", "://bad", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, SameServer(tt.a, tt.b), "%s vs %s", tt.a, tt.b)
	}
}
