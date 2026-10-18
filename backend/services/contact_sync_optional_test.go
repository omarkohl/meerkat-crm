package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRadicaleIsOptionalLocallyButNeverSilentlySkippedInCI(t *testing.T) {
	use, problem := radicaleMode("", "")
	assert.False(t, use)
	assert.Empty(t, problem, "locally the harness is opt-in")

	use, problem = radicaleMode("", "1")
	assert.True(t, use)
	assert.Empty(t, problem)

	use, problem = radicaleMode("true", "1")
	assert.True(t, use)
	assert.Empty(t, problem)

	use, problem = radicaleMode("true", "off")
	assert.False(t, use)
	assert.Empty(t, problem, "CI can switch it off explicitly")

	use, problem = radicaleMode("true", "")
	assert.False(t, use)
	assert.Contains(t, problem, "MEERKAT_CARDDAV_IT")
}
