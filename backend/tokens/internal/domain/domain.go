// Package domain is the vocabulary of the tokens module: API tokens and the calendar feed
// with the rules for their secrets. It imports nothing but the standard library
// (docs/adr/0012-module-layout.md).
package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type UserID uint

type TokenID uint

type FeedID uint

const (
	EventAPITokenCreated = "api_token_created"
	EventAPITokenRevoked = "api_token_revoked"
	EventFeedCreated     = "ics_feed_created"
	EventFeedRevoked     = "ics_feed_revoked"
)

// FetchThrottle bounds how often a fetch is recorded on the feed.
const FetchThrottle = time.Minute

var (
	ErrTokenNotFound = errors.New("api token not found")
	// ErrFeedNotFound covers an unknown token, a revoked feed and a feed whose owner is gone.
	ErrFeedNotFound = errors.New("calendar feed not found")
)

type APIToken struct {
	ID         TokenID
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

type Feed struct {
	ID            FeedID
	Owner         UserID
	TokenHash     string
	CreatedAt     time.Time
	LastFetchedAt *time.Time
	Language      string
}

// Client is who made the request, for audit entries.
type Client struct{ IP, UserAgent string }

var feedFilePattern = regexp.MustCompile(`^([A-Za-z0-9_-]{43})\.ics$`)

func HashSecret(secret string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(secret))) }

// SameHash compares in constant time, whatever the database matched on.
func SameHash(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// NewAPITokenSecret returns the plaintext, shown once, and the hash that is stored.
func NewAPITokenSecret() (plaintext, hash string, err error) {
	t, err := randomToken()
	if err != nil {
		return "", "", err
	}
	plaintext = "meerkat_" + t
	return plaintext, HashSecret(plaintext), nil
}

func NewFeedToken() (token, hash string, err error) {
	token, err = randomToken()
	if err != nil {
		return "", "", err
	}
	return token, HashSecret(token), nil
}

// ParseFeedFile takes the last URL segment (<token>.ics). Anything else cannot be a token,
// so the database is not asked about it.
func ParseFeedFile(file string) (token string, ok bool) {
	m := feedFilePattern.FindStringSubmatch(file)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func TokenAuditDetail(name string) string { return "token:" + strings.ReplaceAll(name, ";", ",") }

func FeedLanguage(userLanguage string) string {
	if userLanguage == "" {
		return "en"
	}
	return userLanguage
}
