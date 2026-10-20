// Package search builds SQLite FTS5 queries for contacts, notes and activities. The indexes
// are created and kept in sync by migrations 29 and 34; when they are missing (a database that was
// not migrated, or an SQLite build without FTS5) callers fall back to LIKE.
package search

import (
	"regexp"
	"strings"
	"sync"
	"unicode"

	"gorm.io/gorm"
)

const (
	// MaxTermRunes and MaxTermWords bound what one search can ask the index for.
	MaxTermRunes = 200
	MaxTermWords = 32
)

// phoneLike is input that is only digits and phone punctuation: "795550" has to find the
// middle of "+41 79 5550101", which a word-prefix index cannot do.
var phoneLike = regexp.MustCompile(`^[\d\s+()./-]*\d[\d\s+()./-]*$`)

// needsSubstring reports input the word-prefix index handles badly: phone-like digits, and
// scripts written without spaces between words (Han, Kana, Hangul), where one "word" is a
// whole sentence and a typed part of it must match anywhere.
func needsSubstring(term string) bool {
	if phoneLike.MatchString(term) {
		return true
	}
	for _, r := range term {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
			return true
		}
	}
	return false
}

// MatchQuery turns what the user typed into an FTS5 MATCH expression: every word must
// match as a prefix ("mül" finds "Müller"), in any order. Each word is quoted, so FTS5
// operators and punctuation in the input are plain text. Input is cut to MaxTermRunes
// characters and MaxTermWords words. ok is false when the index is the wrong tool and the
// caller should use a LIKE search: no letters or digits at all, phone-like digits, or
// Han/Kana/Hangul text.
func MatchQuery(term string) (string, bool) {
	if r := []rune(term); len(r) > MaxTermRunes {
		term = string(r[:MaxTermRunes])
	}
	if needsSubstring(term) {
		return "", false
	}
	words := strings.FieldsFunc(term, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) == 0 {
		return "", false
	}
	if len(words) > MaxTermWords {
		words = words[:MaxTermWords]
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = `"` + w + `"*`
	}
	return strings.Join(quoted, " "), true
}

var availability sync.Map // availKey -> bool

type availKey struct {
	cfg   *gorm.Config
	table string
}

// Available reports whether the FTS index with this name exists. The answer is looked up
// once per database and remembered: the indexes are created by a migration at startup.
func Available(db *gorm.DB, table string) bool {
	key := availKey{db.Config, table}
	if v, ok := availability.Load(key); ok {
		return v.(bool)
	}
	var n int64
	err := db.Session(&gorm.Session{NewDB: true}).Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n).Error
	if err != nil {
		return false // do not remember a failed lookup
	}
	availability.Store(key, n > 0)
	return n > 0
}

// Hits is a subquery of the matching ids with their rank (smaller is better), for joining.
// The columns are named hit_id and hit_rank so they never clash with the joined table's:
//
//	Joins("JOIN "+search.Hits("contacts_fts")+" AS hits ON hits.hit_id = contacts.id", match)
func Hits(table string) string {
	return "(SELECT rowid AS hit_id, rank AS hit_rank FROM " + table + " WHERE " + table + " MATCH ?)"
}

// ContactNames limits a contacts_fts expression to the name columns.
func ContactNames(match string) string {
	return "{firstname lastname nickname} : (" + match + ")"
}

// ContactBasics limits a contacts_fts expression to names, e-mail addresses and phone numbers
// (the pickers); the other columns are for the global search and the contact list.
func ContactBasics(match string) string {
	return "{firstname lastname nickname email phone emails phones} : (" + match + ")"
}
