package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func marked(segs []Segment) []string {
	var out []string
	for _, s := range segs {
		if s.Match {
			out = append(out, s.Text)
		}
	}
	return out
}

func joined(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestWordsMatchAsPrefixesIgnoringCaseAndAccents(t *testing.T) {
	segs := Highlight("Zoë Müllerová met Jo", "ZOE mulle", false)

	assert.Equal(t, []string{"Zoë", "Mülle"}, marked(segs))
	assert.Equal(t, "Zoë Müllerová met Jo", joined(segs), "the text is never changed")
}

func TestOnlyTheStartOfAWordMatches(t *testing.T) {
	assert.Empty(t, marked(Highlight("Alice Walker", "lice", false)))
	assert.Equal(t, []string{"Wal"}, marked(Highlight("Alice Walker", "wal", false)))
}

func TestSubstringModeMatchesAnywhereInTheText(t *testing.T) {
	assert.Equal(t, []string{"京都"}, marked(Highlight("東京都の友達", "京都", true)))
	assert.Equal(t, []string{"ller"}, marked(Highlight("Müller", "LLER", true)))
}

func TestPhoneDigitsMatchAcrossSpacesAndPunctuation(t *testing.T) {
	segs := Highlight("+41 79 5550101", "795550", true)

	assert.Equal(t, []string{"79 5550"}, marked(segs))
	assert.Equal(t, "+41 79 5550101", joined(segs))
}

func TestNoTermNoMark(t *testing.T) {
	assert.Equal(t, []Segment{{Text: "plain"}}, Highlight("plain", "", false))
	assert.Empty(t, Highlight("", "x", false))
}

func TestAnExcerptKeepsTheFirstMatchInViewAndSaysItIsCut(t *testing.T) {
	text := strings.Repeat("lorem ipsum ", 30) + "the Needle is here " + strings.Repeat("dolor sit ", 30)

	segs := Excerpt(text, "needle", false)

	assert.Equal(t, []string{"Needle"}, marked(segs))
	got := joined(segs)
	assert.True(t, strings.HasPrefix(got, "…"))
	assert.True(t, strings.HasSuffix(got, "…"))
	assert.Less(t, len([]rune(got)), 200)
}

func TestAShortTextIsNotCutAndNewlinesBecomeSpaces(t *testing.T) {
	assert.Equal(t, "line one line two", joined(Excerpt("line one\nline two", "two", false)))
}

func TestAnExcerptWithoutAMatchStartsAtTheBeginning(t *testing.T) {
	segs := Excerpt(strings.Repeat("abc ", 100), "zzz", false)

	assert.Empty(t, marked(segs))
	assert.True(t, strings.HasPrefix(joined(segs), "abc abc"))
}
