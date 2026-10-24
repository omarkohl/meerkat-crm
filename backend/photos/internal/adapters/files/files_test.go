package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"meerkat/photos/internal/adapters/files"
	"meerkat/photos/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRefusesWhatIsNotAFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o750))
	d := files.New(dir)
	_, err := d.Open("sub")
	assert.ErrorIs(t, err, domain.ErrFileMissing)
	_, err = d.Open("nothing.jpg")
	assert.ErrorIs(t, err, domain.ErrFileMissing)
}
