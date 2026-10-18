package httputil

import (
	"net/url"
	"strings"
)

// SameServer reports whether two URLs point at the same server: same scheme (webcal
// counts as https), same host name (case-insensitive) and same port (the default port
// of the scheme when none is given). A stored password may only follow such a URL.
func SameServer(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil || ua.Hostname() == "" || ub.Hostname() == "" {
		return false
	}
	schemeA, schemeB := canonicalScheme(ua.Scheme), canonicalScheme(ub.Scheme)
	return schemeA == schemeB &&
		strings.EqualFold(ua.Hostname(), ub.Hostname()) &&
		effectivePort(ua, schemeA) == effectivePort(ub, schemeB)
}

func canonicalScheme(scheme string) string {
	scheme = strings.ToLower(scheme)
	if scheme == "webcal" || scheme == "webcals" {
		return "https"
	}
	return scheme
}

func effectivePort(u *url.URL, scheme string) string {
	if p := u.Port(); p != "" {
		return p
	}
	if scheme == "http" {
		return "80"
	}
	return "443"
}
