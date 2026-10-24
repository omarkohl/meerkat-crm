// Package files reads the picture files of the photos module from disk.
package files

import (
	"os"
	"path/filepath"

	"meerkat/photos/internal/app"
	"meerkat/photos/internal/domain"
)

type Directory struct{ dir string }

func New(dir string) *Directory { return &Directory{dir: dir} }

var _ app.Files = (*Directory)(nil)

func (d *Directory) Open(name string) (domain.File, error) {
	file, err := os.Open(filepath.Join(d.dir, name)) //nolint:gosec // name checked by the domain, directory from config
	if err != nil {
		return domain.File{}, domain.ErrFileMissing
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return domain.File{}, domain.ErrFileMissing
	}
	return domain.File{ReadSeekCloser: file, ModTime: info.ModTime(), Size: info.Size()}, nil
}
