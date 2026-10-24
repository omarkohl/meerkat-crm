// Package files keeps the image files of the images module on disk, one directory per user.
package files

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"meerkat/imageproc"
	"meerkat/images/internal/app"
	"meerkat/images/internal/domain"
	"meerkat/photofile"
	"meerkat/services"
)

const root = "images"

type Directory struct{ dir string }

func New(dir string) *Directory { return &Directory{dir: dir} }

var _ app.Files = (*Directory)(nil)

func (d *Directory) Save(ctx context.Context, actor domain.UserID, data []byte) (domain.Stored, error) {
	img, err := services.StoreImage(ctx, d.dir, uint(actor), data)
	if err != nil {
		if errors.Is(err, imageproc.ErrUnsupported) || errors.Is(err, imageproc.ErrInvalid) || errors.Is(err, imageproc.ErrTooLarge) {
			return domain.Stored{}, errors.Join(domain.ErrInvalidImage, err)
		}
		return domain.Stored{}, err
	}
	return domain.Stored{
		File: img.File, ThumbFile: img.ThumbFile, Mime: img.Mime, Width: img.Width, Height: img.Height,
		Bytes: img.Bytes, TakenAt: img.TakenAt,
	}, nil
}

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

func (d *Directory) Remove(names ...string) { photofile.Remove(d.dir, names...) }

func (d *Directory) Sweep(keep map[string]bool, t time.Time) int {
	if d.dir == "" {
		return 0
	}
	var stale []string
	_ = filepath.WalkDir(filepath.Join(d.dir, root), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return nil //nolint:nilerr // best effort: an unreadable entry is skipped
		}
		rel, err := filepath.Rel(d.dir, path)
		if err != nil || keep[rel] {
			return nil
		}
		if info, err := entry.Info(); err == nil && info.ModTime().Before(t) {
			stale = append(stale, rel)
		}
		return nil
	})
	base, err := os.OpenRoot(d.dir)
	if err != nil {
		return 0
	}
	defer func() { _ = base.Close() }()
	removed := 0
	for _, rel := range stale {
		if base.Remove(rel) == nil {
			removed++
		}
	}
	return removed
}
