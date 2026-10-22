// Package domain is the vocabulary of the activities module: the Activity entity and its rules.
// It imports nothing but the standard library (docs/adr/0012-module-layout.md).
package domain

import (
	"errors"
	"time"
)

type UserID uint

type ActivityID uint

type ContactID uint

// ErrNotFound means there is no such activity for this actor; one of another user looks the same.
var ErrNotFound = errors.New("activity not found")

var ErrContactNotFound = errors.New("contact not found")

// ErrContactsNotFound means at least one of the named contacts is not the actor's.
var ErrContactsNotFound = errors.New("one or more contacts not found")

var (
	ErrImageNotFound = errors.New("image not found")
	ErrTooManyImages = errors.New("too many images")
)

var ErrEndBeforeStart = errors.New("end date before start date")

// ContactRef is the part of a contact an activity shows.
type ContactRef struct {
	ID        ContactID
	Firstname string
	Lastname  string
	Circles   []string
}

// ImageRef is an image of an activity as it is shown: sizes and where to fetch it.
type ImageRef struct {
	ID            uint
	Width, Height int
	Mime          string
	Bytes         int64
	TakenAt       *time.Time
	URL, ThumbURL string
}

type Activity struct {
	ID        ActivityID
	UserID    UserID
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	Title       string
	Description string
	Location    string
	Date        time.Time
	EndDate     *time.Time
	Contacts    []ContactRef
	Images      []ImageRef
}

func (a *Activity) Edit(title, description, location string, date time.Time, endDate *time.Time) {
	a.Title, a.Description, a.Location, a.Date, a.EndDate = title, description, location, date, endDate
}

// EndsBeforeStart reports an end date on an earlier day than the start. Times of day are ignored.
func (a Activity) EndsBeforeStart() bool {
	return a.EndDate != nil && day(*a.EndDate).Before(day(a.Date))
}

func day(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
