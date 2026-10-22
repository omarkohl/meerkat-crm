// Package domain holds what the global search is about: the kinds of results and how a typed
// term is marked in the text that matched it (docs/adr/0012-module-layout.md).
package domain

import "time"

type UserID uint

// Segment is a piece of text; Match says the typed term was found in it.
type Segment struct {
	Text  string
	Match bool
}

type Kind string

const (
	KindContacts   Kind = "contacts"
	KindNotes      Kind = "notes"
	KindActivities Kind = "activities"
)

type Contact struct {
	ID        uint
	Firstname string
	Lastname  string
	Nickname  string
	Emails    []string
	Phones    []string
	Others    []Field // looked at in this order after the name, e-mails and phones
}

// Field is one searchable value; Name is the custom field's name when Key is "custom_field".
type Field struct {
	Key   string
	Name  string
	Value string
}

type Note struct {
	ID          uint
	ContactID   *uint
	ContactName string
	Date        time.Time
	Content     string
}

type Activity struct {
	ID          uint
	Title       string
	Description string
	Location    string
	Date        time.Time
	EndDate     *time.Time
	Contacts    []string // display names of the actor's contacts that took part
}

type ContactHit struct {
	Contact
	Name []Segment
	// Field is "name", "email", "phone" or the Key of a Field; Snippet is the matching value.
	Field     string
	FieldName string
	Snippet   []Segment
}

type NoteHit struct {
	Note
	Snippet []Segment
}

type ActivityHit struct {
	Activity
	Title   []Segment
	Snippet []Segment
}

// DisplayName is the name as the contact list shows it: first name, "nickname", last name.
func (c Contact) DisplayName() string {
	name := c.Firstname
	if c.Nickname != "" {
		name += ` "` + c.Nickname + `"`
	}
	if c.Lastname != "" {
		name += " " + c.Lastname
	}
	return name
}

func HitContact(c Contact, term string, substring bool) ContactHit {
	hit := ContactHit{Contact: c, Name: Highlight(c.DisplayName(), term, substring)}
	if hasMatch(hit.Name) {
		hit.Field = "name"
		return hit
	}
	for _, f := range []struct {
		field  string
		values []string
	}{{"email", c.Emails}, {"phone", c.Phones}} {
		for _, v := range f.values {
			if segs := Highlight(v, term, substring); hasMatch(segs) {
				hit.Field, hit.Snippet = f.field, segs
				return hit
			}
		}
	}
	for _, f := range c.Others {
		if segs := Excerpt(f.Value, term, substring); hasMatch(segs) {
			hit.Field, hit.FieldName, hit.Snippet = f.Key, f.Name, segs
			return hit
		}
	}
	return hit
}

func HitNote(n Note, term string, substring bool) NoteHit {
	return NoteHit{Note: n, Snippet: Excerpt(n.Content, term, substring)}
}

func HitActivity(a Activity, term string, substring bool) ActivityHit {
	hit := ActivityHit{Activity: a, Title: Highlight(a.Title, term, substring)}
	for _, text := range []string{a.Description, a.Location} {
		if segs := Excerpt(text, term, substring); hasMatch(segs) {
			hit.Snippet = segs
			return hit
		}
	}
	if !hasMatch(hit.Title) {
		hit.Snippet = Excerpt(a.Description, term, substring)
	}
	return hit
}

func hasMatch(segs []Segment) bool {
	for _, s := range segs {
		if s.Match {
			return true
		}
	}
	return false
}
