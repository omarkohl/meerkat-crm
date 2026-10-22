// Package domain is the vocabulary of the contacts module: the Contact entity and its rules.
// It imports nothing but the standard library (docs/adr/0012-module-layout.md).
package domain

import (
	"errors"
	"strings"
	"time"
)

// UserID identifies the user who owns data; every operation is done by an actor.
type UserID uint

// ContactID identifies a contact.
type ContactID uint

// ErrNotFound means there is no such contact for this actor. It is also what a contact of
// another user looks like, so existence is never leaked.
var ErrNotFound = errors.New("contact not found")

// Value is a typed value: an email address, phone number, website or messaging handle.
type Value struct {
	Type  string
	Value string
}

// Address is a structured postal address.
type Address struct {
	Type    string
	Street  string
	City    string
	Region  string
	Postal  string
	Country string
}

// Line renders the address as one line, skipping empty parts.
func (a Address) Line() string {
	var parts []string
	for _, p := range []string{a.Street, a.City, a.Region, a.Postal, a.Country} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// Relationship links a contact to a person, who may be a contact too.
type Relationship struct {
	ID               uint
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
	ContactID        ContactID
	RelatedContactID *ContactID
	Name             string
	Type             string
	Birthday         string
	Gender           string
}

// Note is a dated remark about a contact.
type Note struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
	Content   string
	Date      time.Time
	ContactID *ContactID
}

// Activity is something done with one or more contacts.
type Activity struct {
	ID          uint
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
	Title       string
	Description string
	Location    string
	Date        time.Time
	EndDate     *time.Time
}

// Reminder asks the user to get in touch with a contact.
type Reminder struct {
	ID                    uint
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             *time.Time
	Message               string
	ByMail                *bool
	RemindAt              time.Time
	Recurrence            string
	ReoccurFromCompletion *bool
	Completed             bool
	EmailSent             bool
	LastSent              *time.Time
	ContactID             *ContactID
}

// Birthday is an upcoming birthday of a contact or of a person in a relationship.
type Birthday struct {
	Type                  string
	Name                  string
	Birthday              string
	PhotoThumbnail        string
	ContactID             ContactID
	RelationshipType      string
	AssociatedContactName string
}

// ImageRef is an image of a contact as it is shown: sizes, the face crop and where to fetch it.
type ImageRef struct {
	ID            uint
	Width, Height int
	Mime          string
	Bytes         int64
	TakenAt       *time.Time
	CropX, CropY  int
	CropSize      int // 0: no crop
	URL, ThumbURL string
}

// Contact is a person in a user's address book.
type Contact struct {
	ID        ContactID
	UserID    UserID
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	Firstname string
	Lastname  string
	Nickname  string
	Gender    string
	Email     string
	Phone     string
	Birthday  string
	Photo     string
	// PhotoThumbnailFile names the thumbnail file; "" means none. PhotoThumbnail is the URL
	// it is served from.
	PhotoThumbnailFile string
	PhotoThumbnail     string
	// MainImageID is the image the photo is cut from. Images is loaded in full only.
	MainImageID *uint
	Images      []ImageRef

	Address            string
	HowWeMet           string
	FoodPreference     string
	WorkInformation    string
	ContactInformation string
	Circles            []string

	Emails    []Value
	Phones    []Value
	Addresses []Address
	URLs      []Value
	IMPPs     []Value

	Prefix     string
	MiddleName string
	Suffix     string

	Organization string
	Department   string
	JobTitle     string
	Role         string

	Anniversary string
	Note        string

	CustomFields map[string]string

	Archived bool

	// Loaded in full only: Relationships is then non-nil even when empty.
	Relationships []Relationship
	Notes         []Note
	Activities    []Activity
	Reminders     []Reminder
}

// Archive hides the contact from the active lists. Archiving also ends the contact's
// reminders, which is a rule of the use case because reminders are another module's data.
func (c *Contact) Archive() { c.Archived = true }

// Unarchive brings an archived contact back.
func (c *Contact) Unarchive() { c.Archived = false }

// SyncPrimary makes Email, Phone and Address the first entry of Emails, Phones and Addresses,
// the value search and list views show. An empty list leaves the scalar as it is.
func (c *Contact) SyncPrimary() {
	if len(c.Emails) > 0 {
		c.Email = c.Emails[0].Value
	}
	if len(c.Phones) > 0 {
		c.Phone = c.Phones[0].Value
	}
	if len(c.Addresses) > 0 {
		c.Address = c.Addresses[0].Line()
	}
}

// Replace overwrites everything the user edits with in. Identity, timestamps, photo, archive
// state and relationships stay.
func (c *Contact) Replace(in Contact) {
	in.ID, in.UserID, in.CreatedAt, in.UpdatedAt, in.DeletedAt = c.ID, c.UserID, c.CreatedAt, c.UpdatedAt, c.DeletedAt
	in.Photo, in.PhotoThumbnailFile, in.PhotoThumbnail, in.Archived, in.Relationships = c.Photo, c.PhotoThumbnailFile, c.PhotoThumbnail, c.Archived, c.Relationships
	in.MainImageID, in.Images = c.MainImageID, c.Images
	in.Notes, in.Activities, in.Reminders = c.Notes, c.Activities, c.Reminders
	*c = in
	c.SyncPrimary()
}
