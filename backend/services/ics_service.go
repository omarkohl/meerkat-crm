package services

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"meerkat/i18n"
	"meerkat/models"

	"gorm.io/gorm"
)

// yearlessBirthdayYear is the DTSTART year for birthdays stored as --MM-DD. It is a
// leap year after 1970 so that 29 February is valid and clients that ignore earlier
// dates still show the event.
const yearlessBirthdayYear = 1972

const icsLineLimit = 75 // octets per line, excluding CRLF (RFC 5545 3.1)

// BuildICSFeed renders the user's birthdays (contacts that are neither archived nor deceased) and open
// reminders as an iCalendar document. Every query is scoped to userID. The output
// is deterministic for unchanged data (DTSTAMP is the row's update time), so callers
// can use a hash of it as an ETag.
func BuildICSFeed(db *gorm.DB, userID uint, lang string) (string, error) {
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND archived = ? AND is_deceased = ? AND birthday IS NOT NULL AND birthday != ''", userID, false, false).
		Order("id ASC").Find(&contacts).Error; err != nil {
		return "", fmt.Errorf("failed to load birthdays: %w", err)
	}

	var reminders []models.Reminder
	if err := db.Preload("Contact").Where("user_id = ? AND completed = ?", userID, false).
		Order("id ASC").Find(&reminders).Error; err != nil {
		return "", fmt.Errorf("failed to load reminders: %w", err)
	}

	var b strings.Builder
	line := func(s string) { b.WriteString(foldICSLine(s)) }

	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Meerkat CRM//ICS Feed//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:" + escapeICSText(i18n.T(lang, "ics.calendarName")))
	line("REFRESH-INTERVAL;VALUE=DURATION:PT12H")
	line("X-PUBLISHED-TTL:PT12H")

	for _, c := range contacts {
		start, ok := icsBirthdayStart(c.Birthday)
		if !ok {
			continue
		}
		name := icsContactName(c)
		line("BEGIN:VEVENT")
		line(fmt.Sprintf("UID:contact-birthday-%d@meerkat-crm", c.ID))
		line("DTSTAMP:" + icsStamp(c.UpdatedAt))
		line("DTSTART;VALUE=DATE:" + start.Format("20060102"))
		line("RRULE:" + icsRecurrenceRule("yearly", start))
		line("SUMMARY:" + escapeICSText(i18n.T(lang, "ics.birthday", map[string]string{"name": name})))
		line("TRANSP:TRANSPARENT")
		line("END:VEVENT")
	}

	for _, r := range reminders {
		start := r.RemindAt.UTC()
		year, month, day := start.Date()
		line("BEGIN:VEVENT")
		line(fmt.Sprintf("UID:reminder-%d@meerkat-crm", r.ID))
		line("DTSTAMP:" + icsStamp(r.UpdatedAt))
		line(fmt.Sprintf("DTSTART;VALUE=DATE:%04d%02d%02d", year, int(month), day))
		if rule := icsRecurrenceRule(r.Recurrence, start); rule != "" {
			line("RRULE:" + rule)
		}
		line("SUMMARY:" + escapeICSText(r.Message))
		if r.Contact.ID != 0 {
			line("DESCRIPTION:" + escapeICSText(i18n.T(lang, "ics.contact", map[string]string{"name": icsContactName(r.Contact)})))
		}
		line("TRANSP:TRANSPARENT")
		line("END:VEVENT")
	}

	line("END:VCALENDAR")
	return b.String(), nil
}

func icsContactName(c models.Contact) string {
	name := c.Firstname
	if c.Nickname != "" {
		name = c.Nickname
	}
	if c.Lastname != "" {
		name += " " + c.Lastname
	}
	return strings.TrimSpace(name)
}

// icsBirthdayStart parses YYYY-MM-DD or --MM-DD. Impossible dates are rejected.
func icsBirthdayStart(birthday string) (time.Time, bool) {
	if strings.HasPrefix(birthday, "--") {
		t, err := time.Parse("2006-01-02", fmt.Sprintf("%04d-%s", yearlessBirthdayYear, birthday[2:]))
		return t, err == nil
	}
	t, err := time.Parse("2006-01-02", birthday)
	if err != nil {
		return time.Time{}, false
	}
	if t.Year() < 1000 {
		t, err = time.Parse("2006-01-02", fmt.Sprintf("%04d-%s", yearlessBirthdayYear, birthday[5:]))
		return t, err == nil
	}
	return t, true
}

// icsRecurrenceRule maps a reminder recurrence to an RRULE for an event starting on
// start. Like the app (addMonths/addYears), an occurrence that would fall on a
// missing day moves to the last day of the month instead of being skipped, which
// plain FREQ=MONTHLY/YEARLY would do (31 January has no 31 February):
//   - start on day 29-31: BYMONTHDAY=28..D with BYSETPOS=-1 picks the last candidate
//     day that exists in the month, i.e. min(D, last day of month);
//   - start on 29 February: the last day of February (29th in leap years).
func icsRecurrenceRule(recurrence string, start time.Time) string {
	monthly := func(interval int) string {
		rule := "FREQ=MONTHLY"
		if interval > 1 {
			rule += fmt.Sprintf(";INTERVAL=%d", interval)
		}
		if day := start.Day(); day > 28 {
			days := make([]string, 0, 4)
			for d := 28; d <= day; d++ {
				days = append(days, fmt.Sprintf("%d", d))
			}
			rule += ";BYMONTHDAY=" + strings.Join(days, ",") + ";BYSETPOS=-1"
		}
		return rule
	}
	switch recurrence {
	case "weekly":
		return "FREQ=WEEKLY"
	case "monthly":
		return monthly(1)
	case "quarterly":
		return monthly(3)
	case "six-months":
		return monthly(6)
	case "yearly":
		if start.Month() == time.February && start.Day() == 29 {
			return "FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=-1"
		}
		return "FREQ=YEARLY"
	}
	return ""
}

func icsStamp(t time.Time) string {
	if t.IsZero() {
		t = time.Unix(0, 0)
	}
	return t.UTC().Format("20060102T150405Z")
}

// escapeICSText escapes a TEXT value (RFC 5545 3.3.11) and drops control characters.
func escapeICSText(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == ';':
			b.WriteString(`\;`)
		case r == ',':
			b.WriteString(`\,`)
		case r == '\r':
			if i+1 < len(runes) && runes[i+1] == '\n' {
				i++
			}
			b.WriteString(`\n`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// foldICSLine folds a content line at 75 octets (never inside a UTF-8 sequence) and
// terminates it with CRLF (RFC 5545 3.1). Continuation lines start with one space.
func foldICSLine(s string) string {
	var b strings.Builder
	limit := icsLineLimit
	for len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		if cut == 0 {
			// Hardening: no rune start within the limit (not valid UTF-8); cut anywhere instead of looping forever.
			cut = limit
		}
		b.WriteString(s[:cut])
		b.WriteString("\r\n ")
		s = s[cut:]
		limit = icsLineLimit - 1
	}
	b.WriteString(s)
	b.WriteString("\r\n")
	return b.String()
}
