package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckFileRefusesOversizedAndWrongTypeFiles(t *testing.T) {
	assert.NoError(t, CheckFile(CSV, "C.CSV", 10, 100))
	assert.ErrorAs(t, CheckFile(CSV, "c.csv", 101, 100), &TooLargeError{})
	assert.ErrorAs(t, CheckFile(CSV, "c.exe", 10, 100), &WrongTypeError{})
	assert.ErrorAs(t, CheckFile(VCF, "c.csv", 10, 100), &WrongTypeError{})
}

func TestErrorMessagesMatchWhatUsersAlwaysSaw(t *testing.T) {
	assert.Equal(t, "File too large. Maximum size is 5 MB", TooLargeError{MaxMB: 5}.Error())
	assert.Equal(t, "File must be a CSV file", WrongTypeError{Kind: CSV}.Error())
	assert.Equal(t, "File must be a VCF file", WrongTypeError{Kind: VCF}.Error())
}
