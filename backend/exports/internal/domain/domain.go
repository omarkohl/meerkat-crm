// Package domain is the vocabulary of the exports module: what a user's export contains and
// how it is written as CSV. It imports nothing but the standard library
// (docs/adr/0012-module-layout.md).
package domain

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type UserID uint

const (
	EventExport = "export"
	CSVType     = "text/csv; charset=utf-8"
	VCardType   = "text/vcard; charset=utf-8"
)

// Client is who made the request, for audit entries.
type Client struct{ IP, UserAgent string }

type Relationship struct {
	ID                 uint
	Name               string
	Type               string
	Gender             string
	Birthday           string
	RelatedContactID   *uint
	RelatedContactName string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Contact struct {
	ID                 uint
	Firstname          string
	Lastname           string
	Nickname           string
	Gender             string
	Email              string
	Phone              string
	Birthday           string
	Address            string
	HowWeMet           string
	FoodPreference     string
	WorkInformation    string
	ContactInformation string
	Circles            []string
	CustomFields       map[string]string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Relationships      []Relationship
}

type Activity struct {
	ID           uint
	Title        string
	Description  string
	Location     string
	Date         time.Time
	EndDate      *time.Time
	ContactNames []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Note struct {
	ID          uint
	ContactID   *uint
	ContactName string
	Content     string
	Date        time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Reminder struct {
	ID                    uint
	ContactID             *uint
	ContactName           string
	Message               string
	RemindAt              time.Time
	Recurrence            string
	ByMail                *bool
	ReoccurFromCompletion *bool
	Completed             bool
	LastSent              *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Dataset is everything of one user that goes into the CSV export.
type Dataset struct {
	CustomFieldNames []string
	Contacts         []Contact
	Activities       []Activity
	Notes            []Note
	Reminders        []Reminder
}

func Filename(prefix, extension string, now time.Time) string {
	return fmt.Sprintf("%s-%s.%s", prefix, now.Format("2006-01-02"), extension)
}

func ContactName(first, last string) string { return first + " " + last }

var ErrContactNotFound = errors.New("contact not found")

const maxNameBytes = 200

// ContactFilename is the file name for one contact's vCard: their name, without control
// characters and cut to a length every file system takes.
func ContactFilename(first, last string) string {
	name := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, first+" "+last))
	for len(name) > maxNameBytes {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	if name = strings.TrimSpace(name); name == "" {
		name = "contact"
	}
	return name + ".vcf"
}

func id(v *uint) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%d", *v)
}

func stamp(t time.Time) string { return t.Format(time.RFC3339) }

func optionalStamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return stamp(*t)
}

// RenderCSV writes the dataset as one file of sections, each introduced by a "=== NAME ===" line.
func RenderCSV(d Dataset) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	section := func(title string, header []string) error {
		w.Flush()
		buf.WriteString(title)
		return w.Write(header)
	}

	headers := append([]string{
		"ID", "Firstname", "Lastname", "Nickname", "Gender", "Email", "Phone",
		"Birthday", "Address", "How We Met", "Food Preference", "Work Information",
		"Contact Information", "Circles", "Created At", "Updated At",
	}, d.CustomFieldNames...)
	if err := section("=== CONTACTS ===\n", headers); err != nil {
		return nil, err
	}
	for _, c := range d.Contacts {
		record := []string{
			fmt.Sprintf("%d", c.ID), c.Firstname, c.Lastname, c.Nickname, c.Gender, c.Email, c.Phone, c.Birthday, c.Address,
			c.HowWeMet, c.FoodPreference, c.WorkInformation, c.ContactInformation, strings.Join(c.Circles, "; "),
			stamp(c.CreatedAt), stamp(c.UpdatedAt),
		}
		for _, name := range d.CustomFieldNames {
			record = append(record, c.CustomFields[name])
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}

	if err := section("\n=== RELATIONSHIPS ===\n", []string{
		"ID", "Contact ID", "Contact Name", "Name", "Type", "Gender", "Birthday",
		"Related Contact ID", "Related Contact Name", "Created At", "Updated At",
	}); err != nil {
		return nil, err
	}
	for _, c := range d.Contacts {
		for _, r := range c.Relationships {
			if err := w.Write([]string{
				fmt.Sprintf("%d", r.ID), fmt.Sprintf("%d", c.ID), ContactName(c.Firstname, c.Lastname), r.Name, r.Type, r.Gender,
				r.Birthday, id(r.RelatedContactID), r.RelatedContactName, stamp(r.CreatedAt), stamp(r.UpdatedAt),
			}); err != nil {
				return nil, err
			}
		}
	}

	if err := section("\n=== ACTIVITIES ===\n", []string{
		"ID", "Title", "Description", "Location", "Date", "End Date", "Contact Names", "Created At", "Updated At",
	}); err != nil {
		return nil, err
	}
	for _, a := range d.Activities {
		if err := w.Write([]string{
			fmt.Sprintf("%d", a.ID), a.Title, a.Description, a.Location, stamp(a.Date), optionalStamp(a.EndDate), strings.Join(a.ContactNames, "; "),
			stamp(a.CreatedAt), stamp(a.UpdatedAt),
		}); err != nil {
			return nil, err
		}
	}

	if err := section("\n=== NOTES ===\n", []string{
		"ID", "Contact ID", "Contact Name", "Content", "Date", "Created At", "Updated At",
	}); err != nil {
		return nil, err
	}
	for _, n := range d.Notes {
		if err := w.Write([]string{
			fmt.Sprintf("%d", n.ID), id(n.ContactID), n.ContactName, n.Content, stamp(n.Date), stamp(n.CreatedAt), stamp(n.UpdatedAt),
		}); err != nil {
			return nil, err
		}
	}

	if err := section("\n=== REMINDERS ===\n", []string{
		"ID", "Contact ID", "Contact Name", "Message", "Remind At", "Recurrence",
		"By Mail", "Reoccur From Completion", "Completed", "Last Sent", "Created At", "Updated At",
	}); err != nil {
		return nil, err
	}
	for _, r := range d.Reminders {
		lastSent := ""
		if r.LastSent != nil {
			lastSent = stamp(*r.LastSent)
		}
		if err := w.Write([]string{
			fmt.Sprintf("%d", r.ID), id(r.ContactID), r.ContactName, r.Message, stamp(r.RemindAt), r.Recurrence,
			fmt.Sprintf("%t", r.ByMail != nil && *r.ByMail), fmt.Sprintf("%t", r.ReoccurFromCompletion == nil || *r.ReoccurFromCompletion),
			fmt.Sprintf("%t", r.Completed), lastSent, stamp(r.CreatedAt), stamp(r.UpdatedAt),
		}); err != nil {
			return nil, err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
