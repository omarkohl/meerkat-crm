package domain

import "time"

type Recipient struct {
	ID               UserID
	Email            string
	Language         string
	DateFormat       string
	BirthdayMailedOn string
}

type DueReminder struct {
	Reminder
	ContactName  string
	ContactKnown bool
	Event        any
}

type Birthday struct {
	Type                  string
	Name                  string
	Birthday              string
	RelationshipType      string
	AssociatedContactName string
	DaysUntil             int
	Event                 any
}

func (b Birthday) Today() bool { return b.DaysUntil == 0 }

type Digest struct {
	Recipient Recipient
	Reminders []DueReminder
	Birthdays []Birthday
}

func EndOfDay(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
}
