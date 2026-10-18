package domain

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	excerptBefore = 50
	excerptAfter  = 110
)

// fold lower-cases a rune and drops its accents, the way the search index does, so that "zoe"
// finds "Zoë". A rune folds to at most a few runes; most fold to one.
func fold(r rune) []rune {
	var out []rune
	for _, d := range norm.NFD.String(string(r)) {
		if !unicode.Is(unicode.Mn, d) {
			out = append(out, unicode.ToLower(d))
		}
	}
	return out
}

func foldString(s string) []rune {
	var out []rune
	for _, r := range s {
		out = append(out, fold(r)...)
	}
	return out
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func isDigitsAndPunctuation(s string) bool {
	digit := false
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			digit = true
		case r == ' ' || r == '+' || r == '(' || r == ')' || r == '.' || r == '/' || r == '-':
		default:
			return false
		}
	}
	return digit
}

// Highlight splits text into segments and marks where term was found. With substring false
// every word of the term matches the start of a word of the text ("mül" in "Müller"), the way
// the index finds it; with substring true the term matches anywhere (Han, Kana and Hangul text,
// and phone digits, which match across spaces and punctuation).
func Highlight(text, term string, substring bool) []Segment {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	marks := make([]bool, len(runes))
	term = strings.TrimSpace(term)
	switch {
	case term == "":
	case substring && isDigitsAndPunctuation(term):
		markDigits(runes, marks, term)
	case substring:
		markSubstring(runes, marks, term)
	default:
		markWords(runes, marks, term)
	}
	var segs []Segment
	for i, r := range runes {
		if n := len(segs); n > 0 && segs[n-1].Match == marks[i] {
			segs[n-1].Text += string(r)
		} else {
			segs = append(segs, Segment{Text: string(r), Match: marks[i]})
		}
	}
	return segs
}

func markWords(runes []rune, marks []bool, term string) {
	var terms [][]rune
	for _, w := range strings.FieldsFunc(term, func(r rune) bool { return !isWordRune(r) }) {
		terms = append(terms, foldString(w))
	}
	for i := 0; i < len(runes); {
		if !isWordRune(runes[i]) {
			i++
			continue
		}
		j := i
		for j < len(runes) && isWordRune(runes[j]) {
			j++
		}
		// ends[k] is how many folded runes the first k+1 runes of the word give.
		var folded []rune
		var ends []int
		for _, r := range runes[i:j] {
			folded = append(folded, fold(r)...)
			ends = append(ends, len(folded))
		}
		best := 0
		for _, t := range terms {
			if len(t) == 0 || len(t) > len(folded) || string(folded[:len(t)]) != string(t) {
				continue
			}
			for k, e := range ends {
				if e >= len(t) {
					best = max(best, k+1)
					break
				}
			}
		}
		for k := 0; k < best; k++ {
			marks[i+k] = true
		}
		i = j
	}
}

func markSubstring(runes []rune, marks []bool, term string) {
	needle := []rune(strings.ToLower(term))
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	for i := 0; i+len(needle) <= len(lower); {
		if string(lower[i:i+len(needle)]) == string(needle) {
			for k := range needle {
				marks[i+k] = true
			}
			i += len(needle)
		} else {
			i++
		}
	}
}

func markDigits(runes []rune, marks []bool, term string) {
	var digits []rune
	var at []int
	for i, r := range runes {
		if unicode.IsDigit(r) {
			digits = append(digits, r)
			at = append(at, i)
		}
	}
	var needle []rune
	for _, r := range term {
		if unicode.IsDigit(r) {
			needle = append(needle, r)
		}
	}
	for i := 0; i+len(needle) <= len(digits); {
		if string(digits[i:i+len(needle)]) == string(needle) {
			for k := at[i]; k <= at[i+len(needle)-1]; k++ {
				marks[k] = true
			}
			i += len(needle)
		} else {
			i++
		}
	}
}

// Excerpt is Highlight on a window of text around the first match, for a snippet: a short
// text is kept whole, a cut one starts and ends with an ellipsis. Line breaks become spaces.
// Without a match the window starts at the beginning.
func Excerpt(text, term string, substring bool) []Segment {
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= excerptBefore+excerptAfter {
		return Highlight(text, term, substring)
	}
	start := 0
	for i, s := 0, Highlight(text, term, substring); ; {
		if len(s) == 0 {
			break
		}
		seg := s[0]
		s = s[1:]
		if seg.Match {
			start = max(0, i-excerptBefore)
			break
		}
		i += len([]rune(seg.Text))
	}
	end := min(len(runes), start+excerptBefore+excerptAfter)
	segs := Highlight(string(runes[start:end]), term, substring)
	if start > 0 {
		segs = append([]Segment{{Text: "…"}}, segs...)
	}
	if end < len(runes) {
		segs = append(segs, Segment{Text: "…"})
	}
	return segs
}
