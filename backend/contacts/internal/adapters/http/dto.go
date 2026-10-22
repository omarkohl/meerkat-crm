package httpadapter

import (
	"time"

	"meerkat/contacts/internal/domain"
)

// ContactValue is a typed email address, phone number, website or messaging handle.
type ContactValue struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ContactAddress is a structured postal address.
type ContactAddress struct {
	Type    string `json:"type"`
	Street  string `json:"street"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Postal  string `json:"postal"`
	Country string `json:"country"`
}

// ContactResponse is a contact as the API returns it. The JSON names (ID, CreatedAt, ... next
// to snake_case ones) are those of the GORM model it replaces, because clients depend on them.
type ContactResponse struct {
	ID        uint       `json:"ID"`
	CreatedAt time.Time  `json:"CreatedAt"`
	UpdatedAt time.Time  `json:"UpdatedAt"`
	DeletedAt *time.Time `json:"DeletedAt" nullable:"true"`

	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Nickname  string `json:"nickname"`
	Gender    string `json:"gender"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Birthday  string `json:"birthday"`
	Photo     string `json:"photo"`

	// Relationships is null unless the endpoint loads them.
	Relationships []ContactRelationship `json:"relationships" nullable:"true"`
	Notes         []ContactNote         `json:"notes,omitempty"`
	Activities    []ContactActivity     `json:"activities,omitempty"`
	Reminders     []ContactReminder     `json:"reminders,omitempty"`

	Address            string   `json:"address"`
	HowWeMet           string   `json:"how_we_met"`
	FoodPreference     string   `json:"food_preference"`
	WorkInformation    string   `json:"work_information"`
	ContactInformation string   `json:"contact_information"`
	Circles            []string `json:"circles" nullable:"true"`

	Emails    []ContactValue   `json:"emails" nullable:"true"`
	Phones    []ContactValue   `json:"phones" nullable:"true"`
	Addresses []ContactAddress `json:"addresses" nullable:"true"`
	URLs      []ContactValue   `json:"urls" nullable:"true"`
	IMPPs     []ContactValue   `json:"impps" nullable:"true"`

	Prefix     string `json:"prefix"`
	MiddleName string `json:"middle_name"`
	Suffix     string `json:"suffix"`

	Organization string `json:"organization"`
	Department   string `json:"department"`
	JobTitle     string `json:"job_title"`
	Role         string `json:"role"`

	Anniversary string `json:"anniversary"`
	Note        string `json:"note"`

	CustomFields map[string]string `json:"custom_fields" nullable:"true"`

	Archived bool `json:"archived"`
}

// ContactRelationship is a relationship between contacts, as embedded in a contact when
// the endpoint loads them.
type ContactRelationship struct {
	ID               uint       `json:"ID"`
	CreatedAt        time.Time  `json:"CreatedAt"`
	UpdatedAt        time.Time  `json:"UpdatedAt"`
	DeletedAt        *time.Time `json:"DeletedAt" nullable:"true"`
	ContactID        uint       `json:"contact_id"`
	RelatedContactID *uint      `json:"related_contact_id" nullable:"true"`
	Name             string     `json:"name"`
	Type             string     `json:"type"`
	Birthday         string     `json:"birthday"`
	Gender           string     `json:"gender"`
}

// NewContactResponse maps a contact to the JSON shape of the API, also used for webhooks.
type ContactNote struct {
	ID        uint       `json:"ID"`
	CreatedAt time.Time  `json:"CreatedAt"`
	UpdatedAt time.Time  `json:"UpdatedAt"`
	DeletedAt *time.Time `json:"DeletedAt" nullable:"true"`
	Content   string     `json:"content"`
	Date      time.Time  `json:"date"`
	ContactID *uint      `json:"contact_id" nullable:"true"`
}

// ContactActivity is an activity of a contact, embedded when the contact is loaded in full.
type ContactActivity struct {
	ID          uint       `json:"ID"`
	CreatedAt   time.Time  `json:"CreatedAt"`
	UpdatedAt   time.Time  `json:"UpdatedAt"`
	DeletedAt   *time.Time `json:"DeletedAt" nullable:"true"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Location    string     `json:"location"`
	Date        time.Time  `json:"date"`
	EndDate     *time.Time `json:"end_date" nullable:"true"`
}

type ContactReminder struct {
	ID                    uint       `json:"ID"`
	CreatedAt             time.Time  `json:"CreatedAt"`
	UpdatedAt             time.Time  `json:"UpdatedAt"`
	DeletedAt             *time.Time `json:"DeletedAt" nullable:"true"`
	Message               string     `json:"message"`
	ByMail                *bool      `json:"by_mail" nullable:"true"`
	RemindAt              time.Time  `json:"remind_at"`
	Recurrence            string     `json:"recurrence"`
	ReoccurFromCompletion *bool      `json:"reoccur_from_completion" nullable:"true"`
	Completed             bool       `json:"completed"`
	EmailSent             bool       `json:"email_sent"`
	LastSent              *time.Time `json:"last_sent" nullable:"true"`
	ContactID             *uint      `json:"contact_id" nullable:"true"`
}

func NewContactResponse(c domain.Contact) ContactResponse {
	r := ContactResponse{
		ID: uint(c.ID), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, DeletedAt: c.DeletedAt,
		Firstname: c.Firstname, Lastname: c.Lastname, Nickname: c.Nickname, Gender: c.Gender,
		Email: c.Email, Phone: c.Phone, Birthday: c.Birthday, Photo: c.Photo,
		Address: c.Address, HowWeMet: c.HowWeMet, FoodPreference: c.FoodPreference,
		WorkInformation: c.WorkInformation, ContactInformation: c.ContactInformation,
		Circles: c.Circles,
		Prefix:  c.Prefix, MiddleName: c.MiddleName, Suffix: c.Suffix,
		Organization: c.Organization, Department: c.Department, JobTitle: c.JobTitle, Role: c.Role,
		Anniversary: c.Anniversary, Note: c.Note,
		CustomFields: c.CustomFields, Archived: c.Archived,
	}
	if c.Relationships != nil {
		r.Relationships = []ContactRelationship{}
	}
	for _, n := range c.Notes {
		r.Notes = append(r.Notes, ContactNote{ID: n.ID, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt, DeletedAt: n.DeletedAt,
			Content: n.Content, Date: n.Date, ContactID: contactIDPtr(n.ContactID)})
	}
	for _, a := range c.Activities {
		r.Activities = append(r.Activities, ContactActivity{ID: a.ID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, DeletedAt: a.DeletedAt,
			Title: a.Title, Description: a.Description, Location: a.Location, Date: a.Date, EndDate: a.EndDate})
	}
	for _, m := range c.Reminders {
		r.Reminders = append(r.Reminders, ContactReminder{ID: m.ID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, DeletedAt: m.DeletedAt,
			Message: m.Message, ByMail: m.ByMail, RemindAt: m.RemindAt, Recurrence: m.Recurrence,
			ReoccurFromCompletion: m.ReoccurFromCompletion, Completed: m.Completed, EmailSent: m.EmailSent,
			LastSent: m.LastSent, ContactID: contactIDPtr(m.ContactID)})
	}
	for _, rel := range c.Relationships {
		var related *uint
		if rel.RelatedContactID != nil {
			v := uint(*rel.RelatedContactID)
			related = &v
		}
		r.Relationships = append(r.Relationships, ContactRelationship{
			ID: rel.ID, CreatedAt: rel.CreatedAt, UpdatedAt: rel.UpdatedAt, DeletedAt: rel.DeletedAt,
			ContactID: uint(rel.ContactID), RelatedContactID: related,
			Name: rel.Name, Type: rel.Type, Birthday: rel.Birthday, Gender: rel.Gender,
		})
	}
	r.Emails = values(c.Emails)
	r.Phones = values(c.Phones)
	r.URLs = values(c.URLs)
	r.IMPPs = values(c.IMPPs)
	for _, a := range c.Addresses {
		r.Addresses = append(r.Addresses, ContactAddress{Type: a.Type, Street: a.Street, City: a.City, Region: a.Region, Postal: a.Postal, Country: a.Country})
	}
	return r
}

func values(in []domain.Value) []ContactValue {
	var out []ContactValue
	for _, v := range in {
		out = append(out, ContactValue{Type: v.Type, Value: v.Value})
	}
	return out
}

func contactIDPtr(id *domain.ContactID) *uint {
	if id == nil {
		return nil
	}
	v := uint(*id)
	return &v
}
