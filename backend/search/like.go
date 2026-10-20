package search

import (
	"strings"
	"unicode"
)

// LikePattern escapes what LIKE treats as a wildcard; use it with ESCAPE '\'.
func LikePattern(term string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.ToLower(term)) + "%"
}

// ContactNameLike is the substring search over a contact's full name: a WHERE condition and
// its arguments.
func ContactNameLike(term string) (string, []any) {
	return "LOWER(TRIM(REPLACE(contacts.firstname || ' ' || contacts.nickname || ' ' || contacts.lastname, '  ', ' '))) LIKE ? ESCAPE '\\'",
		[]any{LikePattern(term)}
}

// ContactAnyFieldLike is the substring search over every field the contacts_fts index covers,
// for input the index cannot match: a WHERE condition and its arguments.
func ContactAnyFieldLike(term string) (string, []any) {
	like := LikePattern(term)
	name, args := ContactNameLike(term)
	values := "CASE WHEN json_valid(contacts.%s) THEN contacts.%s ELSE '[]' END"
	emails := "json_each(" + strings.ReplaceAll(values, "%s", "emails") + ") AS j"
	phones := "json_each(" + strings.ReplaceAll(values, "%s", "phones") + ") AS j"
	cond := "(" + name + ") OR LOWER(contacts.email) LIKE ? ESCAPE '\\' OR LOWER(contacts.phone) LIKE ? ESCAPE '\\'" +
		" OR EXISTS (SELECT 1 FROM " + emails + " WHERE LOWER(json_extract(j.value, '$.value')) LIKE ? ESCAPE '\\')" +
		" OR EXISTS (SELECT 1 FROM " + phones + " WHERE LOWER(json_extract(j.value, '$.value')) LIKE ? ESCAPE '\\')"
	args = append(args, like, like, like, like)
	for _, col := range []string{"middle_name", "organization", "department", "job_title", "role", "how_we_met", "food_preference", "work_information", "contact_information", "note", "address"} {
		cond += " OR LOWER(contacts." + col + ") LIKE ? ESCAPE '\\'"
		args = append(args, like)
	}
	cond += " OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(contacts.addresses) AND json_type(contacts.addresses) = 'array' THEN contacts.addresses ELSE '[]' END) AS j WHERE LOWER(COALESCE(json_extract(j.value, '$.street'), '') || ' ' || COALESCE(json_extract(j.value, '$.city'), '') || ' ' || COALESCE(json_extract(j.value, '$.region'), '') || ' ' || COALESCE(json_extract(j.value, '$.postal'), '') || ' ' || COALESCE(json_extract(j.value, '$.country'), '')) LIKE ? ESCAPE '\\')" +
		" OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(contacts.circles) AND json_type(contacts.circles) = 'array' THEN contacts.circles ELSE '[]' END) AS j WHERE LOWER(j.value) LIKE ? ESCAPE '\\')" +
		" OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(contacts.custom_fields) AND json_type(contacts.custom_fields) = 'object' THEN contacts.custom_fields ELSE '{}' END) AS j WHERE LOWER(j.value) LIKE ? ESCAPE '\\')"
	args = append(args, like, like, like)
	if phoneLike.MatchString(term) {
		digits := "%" + digitsOf(term) + "%"
		cond += " OR " + withoutPunctuation("contacts.phone") + " LIKE ?" +
			" OR EXISTS (SELECT 1 FROM " + phones + " WHERE " + withoutPunctuation("json_extract(j.value, '$.value')") + " LIKE ?)"
		args = append(args, digits, digits)
	}
	return cond, args
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// withoutPunctuation is a SQL expression that drops the characters a phone number is
// written with, so that digits match across them.
func withoutPunctuation(col string) string {
	e := col
	for _, c := range []string{" ", "-", "(", ")", ".", "/", "+"} {
		e = "replace(" + e + ", '" + c + "', '')"
	}
	return e
}
