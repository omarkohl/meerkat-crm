package httpadapter

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	unsafeFilenameChars = regexp.MustCompile(`[^a-z0-9]+`)
	letterFolds         = strings.NewReplacer("ø", "o", "Ø", "O", "ł", "l", "Ł", "L", "ß", "ss", "æ", "ae", "Æ", "AE",
		"œ", "oe", "Œ", "OE", "đ", "d", "Đ", "D", "ð", "d", "Ð", "D", "þ", "th", "Þ", "TH")
)

// contentDisposition names the download. The ASCII filename is the fallback (accents folded, other
// scripts dropped); a name that differs also goes, percent-encoded, in filename* (RFC 6266), the
// only form that carries bytes beyond [A-Za-z0-9-._].
func contentDisposition(filename string) string {
	filename = norm.NFC.String(filename)
	stem, ext := filename, ""
	if i := strings.LastIndexByte(filename, '.'); i > 0 {
		stem, ext = filename[:i], filename[i:]
	}
	fallback := asciiSlug(stem)
	if ext != "" {
		fallback += "." + asciiSlug(ext)
	}
	header := "attachment; filename=" + fallback
	if fallback != filename {
		header += "; filename*=UTF-8''" + percentEncode(filename)
	}
	return header
}

func asciiSlug(s string) string {
	var folded strings.Builder
	for _, r := range norm.NFD.String(letterFolds.Replace(s)) {
		if !unicode.Is(unicode.Mn, r) {
			folded.WriteRune(r)
		}
	}
	slug := strings.Trim(unsafeFilenameChars.ReplaceAllString(strings.ToLower(folded.String()), "-"), "-")
	if slug == "" {
		return "contact"
	}
	return slug
}

func percentEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.IndexByte("!#$&+-.^_`|~", c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
