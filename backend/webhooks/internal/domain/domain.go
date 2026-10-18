// Package domain is the vocabulary of the webhooks module: a user's webhook subscriptions
// and the records of their deliveries. It imports nothing but the standard library
// (docs/adr/0012-module-layout.md).
package domain

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"
)

type UserID uint

type WebhookID uint

// MaxPerUser is how many webhooks a user may have.
const MaxPerUser = 20

var (
	ErrNotFound = errors.New("webhook not found")
	ErrLimit    = errors.New("webhook limit reached")
)

type Webhook struct {
	ID        WebhookID
	Name      string
	URL       string
	Events    []string
	Active    bool
	CreatedAt time.Time
}

// Input is what the user can set on a webhook.
type Input struct {
	Name   string
	URL    string
	Events []string
	Active bool
}

type Delivery struct {
	ID          uint
	WebhookID   WebhookID
	EventType   string
	StatusCode  *int
	Error       *string
	Attempts    int
	Status      string
	NextRetryAt *time.Time
	CreatedAt   time.Time
}

// NewSecret is the key the receiver verifies signatures with; it is shown once.
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
