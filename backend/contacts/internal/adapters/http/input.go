package httpadapter

import (
	"meerkat/api"
	"meerkat/contacts/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// EmailInput, PhoneInput, URLInput, IMPPInput and AddressInput are the entries of a
// contact's lists in a request. The validate tags are the rules of ContactBody.
type EmailInput struct {
	Type  string `json:"type" validate:"max=30"`
	Value string `json:"value" validate:"required,email"`
}

type PhoneInput struct {
	Type  string `json:"type" validate:"max=30"`
	Value string `json:"value" validate:"required,phone"`
}

type URLInput struct {
	Type  string `json:"type" validate:"max=30"`
	Value string `json:"value" validate:"required,max=500,safeurl"`
}

type IMPPInput struct {
	Type  string `json:"type" validate:"max=30"`
	Value string `json:"value" validate:"required,max=200,safeurl"`
}

type AddressInput struct {
	Type    string `json:"type" validate:"max=30"`
	Street  string `json:"street" validate:"max=500"`
	City    string `json:"city" validate:"max=200"`
	Region  string `json:"region" validate:"max=200"`
	Postal  string `json:"postal" validate:"max=30"`
	Country string `json:"country" validate:"max=100"`
}

// ContactBody is what the user edits of a contact, on create and on update.
type ContactBody struct {
	Firstname          string            `json:"firstname" validate:"required,min=1,max=100"`
	Lastname           string            `json:"lastname" validate:"max=100"`
	Nickname           string            `json:"nickname" validate:"max=50"`
	Gender             string            `json:"gender" validate:"omitempty,oneof=male female other prefer_not_to_say"`
	Email              string            `json:"email" validate:"omitempty,email"`
	Phone              string            `json:"phone" validate:"omitempty,phone"`
	Birthday           string            `json:"birthday" validate:"omitempty,birthday"`
	Address            string            `json:"address" validate:"max=500"`
	HowWeMet           string            `json:"how_we_met" validate:"max=1000"`
	FoodPreference     string            `json:"food_preference" validate:"max=500"`
	WorkInformation    string            `json:"work_information" validate:"max=1000"`
	ContactInformation string            `json:"contact_information" validate:"max=1000"`
	Circles            []string          `json:"circles" validate:"unique_circles"`
	CustomFields       map[string]string `json:"custom_fields"`

	Emails    []EmailInput   `json:"emails" validate:"omitempty,max=25,dive"`
	Phones    []PhoneInput   `json:"phones" validate:"omitempty,max=25,dive"`
	Addresses []AddressInput `json:"addresses" validate:"omitempty,max=25,dive"`
	URLs      []URLInput     `json:"urls" validate:"omitempty,max=25,dive"`
	IMPPs     []IMPPInput    `json:"impps" validate:"omitempty,max=25,dive"`

	Prefix     string `json:"prefix" validate:"max=50"`
	MiddleName string `json:"middle_name" validate:"max=100"`
	Suffix     string `json:"suffix" validate:"max=50"`

	Organization string `json:"organization" validate:"max=200"`
	Department   string `json:"department" validate:"max=200"`
	JobTitle     string `json:"job_title" validate:"max=200"`
	Role         string `json:"role" validate:"max=200"`

	Anniversary string `json:"anniversary" validate:"omitempty,birthday"`

	Note string `json:"note" validate:"max=5000"`

	IsDeceased   bool   `json:"is_deceased"`
	DeceasedDate string `json:"deceased_date" validate:"omitempty,deceased_date"`
}

// CreateInput is the request of POST /contacts.
type CreateInput struct{ Body ContactBody }

// Resolve implements huma.Resolver.
func (i *CreateInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

// UpdateInput is the request of PUT /contacts/{id}.
type UpdateInput struct {
	ID   uint `path:"id" minimum:"1" doc:"The contact's ID"`
	Body ContactBody
}

// Resolve implements huma.Resolver.
func (i *UpdateInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

func (b ContactBody) draft() domain.Contact {
	c := domain.Contact{
		Firstname: b.Firstname, Lastname: b.Lastname, Nickname: b.Nickname, Gender: b.Gender,
		Email: b.Email, Phone: b.Phone, Birthday: b.Birthday, Address: b.Address,
		HowWeMet: b.HowWeMet, FoodPreference: b.FoodPreference, WorkInformation: b.WorkInformation,
		ContactInformation: b.ContactInformation, Circles: b.Circles, CustomFields: b.CustomFields,
		Prefix: b.Prefix, MiddleName: b.MiddleName, Suffix: b.Suffix,
		Organization: b.Organization, Department: b.Department, JobTitle: b.JobTitle, Role: b.Role,
		Anniversary: b.Anniversary, Note: b.Note,
		IsDeceased: b.IsDeceased, DeceasedDate: b.DeceasedDate,
	}
	for _, e := range b.Emails {
		c.Emails = append(c.Emails, domain.Value{Type: e.Type, Value: e.Value})
	}
	for _, p := range b.Phones {
		c.Phones = append(c.Phones, domain.Value{Type: p.Type, Value: p.Value})
	}
	for _, u := range b.URLs {
		c.URLs = append(c.URLs, domain.Value{Type: u.Type, Value: u.Value})
	}
	for _, i := range b.IMPPs {
		c.IMPPs = append(c.IMPPs, domain.Value{Type: i.Type, Value: i.Value})
	}
	for _, a := range b.Addresses {
		c.Addresses = append(c.Addresses, domain.Address{Type: a.Type, Street: a.Street, City: a.City, Region: a.Region, Postal: a.Postal, Country: a.Country})
	}
	return c
}
