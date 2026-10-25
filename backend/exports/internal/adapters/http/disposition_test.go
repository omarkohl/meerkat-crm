package httpadapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// @016-US1
func TestContentDispositionKeepsAFileNameThatIsAlreadyASCII(t *testing.T) {
	assert.Equal(t, "attachment; filename=meerkat-export-2026-03-10.csv", contentDisposition("meerkat-export-2026-03-10.csv"))
}

// @016-US1
func TestContentDispositionFoldsLettersForTheFallbackAndEncodesTheRealName(t *testing.T) {
	assert.Equal(t, `attachment; filename=zoe-muller.vcf; filename*=UTF-8''Zo%C3%AB%20M%C3%BCller.vcf`, contentDisposition("Zoë Müller.vcf"))
	assert.Equal(t, `attachment; filename=soren-lukasz-strasse.vcf; filename*=UTF-8''S%C3%B8ren%20%C5%81ukasz%20Stra%C3%9Fe.vcf`, contentDisposition("Søren Łukasz Straße.vcf"))
	assert.Equal(t, `attachment; filename=alice-johnson.vcf; filename*=UTF-8''Alice%20Johnson.vcf`, contentDisposition("Alice Johnson.vcf"))
}

// @016-US1
func TestContentDispositionNormalisesTheNameBeforeEncodingIt(t *testing.T) {
	decomposed := "Zoë.vcf"
	assert.Equal(t, `attachment; filename=zoe.vcf; filename*=UTF-8''Zo%C3%AB.vcf`, contentDisposition(decomposed))
}

// @016-US1
func TestContentDispositionFallsBackForANameInAnotherScript(t *testing.T) {
	assert.Equal(t, `attachment; filename=contact.vcf; filename*=UTF-8''%D0%98%D0%B2%D0%B0%D0%BD.vcf`, contentDisposition("Иван.vcf"))
}

// @016-US1
func TestContentDispositionLetsNothingButTokenCharactersIntoTheHeader(t *testing.T) {
	h := contentDisposition("A\"b  Set-Cookie: x=1 O'Brien;.vcf")
	assert.Equal(t, `attachment; filename=a-b-set-cookie-x-1-o-brien.vcf; filename*=UTF-8''A%22b%20%20Set-Cookie%3A%20x%3D1%20O%27Brien%3B.vcf`, h)
}
