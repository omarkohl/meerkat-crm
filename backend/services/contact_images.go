package services

import (
	"context"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"meerkat/imageproc"
	"meerkat/logger"
	"meerkat/models"
	"meerkat/photofile"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrContactNotFound = errors.New("contact not found")
	ErrInvalidCrop     = errors.New("crop is not a square inside the image")
)

// ImageCropInput is the face square of a contact's image, in pixels of the stored image.
type ImageCropInput struct{ X, Y, Size int }

// ImageEffects are the files a database change made or made unused. Call Finish after the
// transaction: a rollback removes what was created, a commit removes what is no longer used.
type ImageEffects struct{ Created, Removed []string }

func (e *ImageEffects) Finish(dir string, err error) {
	if err != nil {
		photofile.Remove(dir, e.Created...)
		return
	}
	photofile.Remove(dir, e.Removed...)
}

// StoreImage turns an uploaded picture into the stored variants on disk. The returned row is not saved.
func StoreImage(ctx context.Context, dir string, userID uint, data []byte) (models.Image, error) {
	res, err := imageproc.Prepare(ctx, data)
	if err != nil {
		return models.Image{}, err
	}
	return storePrepared(dir, userID, res)
}

func storePrepared(dir string, userID uint, res *imageproc.Result) (models.Image, error) {
	userDir := filepath.Join("images", "u"+strconv.FormatUint(uint64(userID), 10))
	if err := photofile.EnsureDir(filepath.Join(dir, userDir)); err != nil {
		return models.Image{}, fmt.Errorf("create image directory: %w", err)
	}
	base := uuid.New().String()
	file, thumb := filepath.Join(userDir, base+"_image.webp"), filepath.Join(userDir, base+"_thumb.webp")
	if err := photofile.WriteFile(filepath.Join(dir, file), res.Image); err != nil {
		return models.Image{}, err
	}
	if err := photofile.WriteFile(filepath.Join(dir, thumb), res.Thumb); err != nil {
		_ = os.Remove(filepath.Join(dir, file))
		return models.Image{}, err
	}
	return models.Image{
		UserID: userID, File: file, ThumbFile: thumb, Mime: res.Mime,
		Width: res.Width, Height: res.Height, Bytes: int64(len(res.Image)), TakenAt: res.TakenAt,
	}, nil
}

func loadContact(tx *gorm.DB, userID, contactID uint) (models.Contact, error) {
	var c models.Contact
	err := tx.Select("id", "photo", "photo_thumbnail_file", "photo_dhash", "main_image_id").Where("user_id = ?", userID).First(&c, contactID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c, ErrContactNotFound
	}
	return c, err
}

func contactImages(tx *gorm.DB, userID, contactID uint) ([]models.Image, error) {
	var images []models.Image
	err := tx.Where("user_id = ? AND contact_id = ?", userID, contactID).Order("position, id").Find(&images).Error
	return images, err
}

// SetContactImages makes ids the ordered images of the contact (see SetOwnerImages) and keeps its
// main image and avatar right: a removed main image gives way to the next one, and a contact
// that loses an image to this one is repaired the same way.
func SetContactImages(tx *gorm.DB, dir string, userID, contactID uint, ids []uint, eff *ImageEffects) error {
	contact, err := loadContact(tx, userID, contactID)
	if err != nil {
		return err
	}
	var leaving []uint
	if len(ids) > 0 {
		err := tx.Model(&models.Contact{}).Where("user_id = ? AND id <> ? AND main_image_id IN ?", userID, contactID, ids).Pluck("id", &leaving).Error
		if err != nil {
			return err
		}
	}
	removed, err := SetOwnerImages(tx, userID, ImageOwner{Kind: ImageOfContact, ID: contactID}, ids)
	if err != nil {
		return err
	}
	eff.Removed = append(eff.Removed, removed...)
	if err := repairMain(tx, dir, userID, contact, eff); err != nil {
		return err
	}
	for _, id := range leaving {
		other, err := loadContact(tx, userID, id)
		if err != nil {
			return err
		}
		if err := repairMain(tx, dir, userID, other, eff); err != nil {
			return err
		}
	}
	return nil
}

// repairMain keeps the main image if it still belongs to the contact, else takes the first image
// with a centred crop, else clears the avatar. A contact with a photo but no main image
// (made by older code) is left alone.
func repairMain(tx *gorm.DB, dir string, userID uint, c models.Contact, eff *ImageEffects) error {
	images, err := contactImages(tx, userID, c.ID)
	if err != nil {
		return err
	}
	if c.MainImageID != nil {
		for _, img := range images {
			if img.ID == *c.MainImageID {
				return nil
			}
		}
	}
	if len(images) > 0 {
		next := images[0]
		if next.CropSize == nil {
			x, y, size := imageproc.CenteredCrop(next.Width, next.Height)
			next.CropX, next.CropY, next.CropSize = &x, &y, &size
		}
		return makeMain(tx, dir, userID, c, next, eff)
	}
	if c.MainImageID == nil {
		return nil
	}
	eff.Removed = append(eff.Removed, c.Photo, c.PhotoThumbnailFile)
	return tx.Model(&models.Contact{Model: gorm.Model{ID: c.ID}}).Where("user_id = ?", userID).
		Updates(map[string]any{"main_image_id": nil, "photo": "", "photo_thumbnail_file": "", "photo_thumbnail": ""}).Error
}

// makeMain saves the crop of img, points the contact at it and cuts a new avatar from the crop.
func makeMain(tx *gorm.DB, dir string, userID uint, c models.Contact, img models.Image, eff *ImageEffects) error {
	if !strings.HasPrefix(img.File, "images/") || strings.Contains(img.File, "..") {
		return fmt.Errorf("unsafe image path %q", img.File)
	}
	ctx := tx.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := imageproc.Acquire(ctx)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, img.File)) //nolint:gosec // checked above, directory from config
	if err != nil {
		release()
		return err
	}
	src, err := imageproc.Decode(data)
	if err != nil {
		release()
		return err
	}
	photo, thumb := imageproc.Avatar(src.Image, *img.CropX, *img.CropY, *img.CropSize)
	release()
	dhash := int64(imageproc.DHash(photo, 0, 0, photo.Bounds().Dx())) //nolint:gosec // the bits are kept, not the value

	if err := photofile.EnsureDir(dir); err != nil {
		return err
	}
	base := uuid.New().String()
	photoName, thumbName := base+"_photo.jpg", base+"_thumb.jpg"
	if err := photofile.SaveJPEG(filepath.Join(dir, photoName), photo); err != nil {
		return err
	}
	eff.Created = append(eff.Created, photoName)
	if err := photofile.SaveJPEG(filepath.Join(dir, thumbName), thumb); err != nil {
		return err
	}
	eff.Created = append(eff.Created, thumbName)
	eff.Removed = append(eff.Removed, c.Photo, c.PhotoThumbnailFile)

	if err := tx.Model(&models.Image{}).Where("user_id = ? AND id = ?", userID, img.ID).
		Updates(map[string]any{"crop_x": *img.CropX, "crop_y": *img.CropY, "crop_size": *img.CropSize}).Error; err != nil {
		return err
	}
	return tx.Model(&models.Contact{Model: gorm.Model{ID: c.ID}}).Where("user_id = ?", userID).
		Updates(map[string]any{"main_image_id": img.ID, "photo": photoName, "photo_thumbnail_file": thumbName, "photo_thumbnail": "", "photo_dhash": dhash}).Error
}

// SetMainImage makes an image of the contact its main image with the given face crop.
func SetMainImage(tx *gorm.DB, dir string, userID, contactID, imageID uint, crop ImageCropInput, eff *ImageEffects) error {
	contact, err := loadContact(tx, userID, contactID)
	if err != nil {
		return err
	}
	var img models.Image
	err = tx.Where("user_id = ? AND contact_id = ?", userID, contactID).First(&img, imageID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrImageNotFound
	}
	if err != nil {
		return err
	}
	if crop.Size < 1 || crop.X < 0 || crop.Y < 0 || crop.X+crop.Size > img.Width || crop.Y+crop.Size > img.Height {
		return ErrInvalidCrop
	}
	img.CropX, img.CropY, img.CropSize = &crop.X, &crop.Y, &crop.Size
	return makeMain(tx, dir, userID, contact, img, eff)
}

// AddContactImage stores data as a new image of the contact and makes it the main image with a
// centred crop. front puts it first in the gallery, else it goes last.
func AddContactImage(ctx context.Context, tx *gorm.DB, dir string, userID, contactID uint, data []byte, front bool, eff *ImageEffects) (models.Image, error) {
	img, err := StoreImage(ctx, dir, userID, data)
	if err != nil {
		return models.Image{}, err
	}
	return AttachContactImage(tx, dir, userID, contactID, img, front, eff)
}

// AttachContactImage saves an image made by StoreImage as the main image of the contact. The
// files of img count as created: they go if the transaction fails.
func AttachContactImage(tx *gorm.DB, dir string, userID, contactID uint, img models.Image, front bool, eff *ImageEffects) (models.Image, error) {
	eff.Created = append(eff.Created, img.File, img.ThumbFile)
	contact, err := loadContact(tx, userID, contactID)
	if err != nil {
		return models.Image{}, err
	}
	existing, err := contactImages(tx, userID, contactID)
	if err != nil {
		return models.Image{}, err
	}
	if len(existing) >= MaxImagesPerContact {
		return models.Image{}, ErrTooManyImages
	}
	img.ContactID = &contactID
	if front {
		if err := tx.Model(&models.Image{}).Where("user_id = ? AND contact_id = ?", userID, contactID).
			Update("position", gorm.Expr("position + 1")).Error; err != nil {
			return models.Image{}, err
		}
	} else if len(existing) > 0 {
		img.Position = existing[len(existing)-1].Position + 1
	}
	x, y, size := imageproc.CenteredCrop(img.Width, img.Height)
	img.CropX, img.CropY, img.CropSize = &x, &y, &size
	if err := tx.Create(&img).Error; err != nil {
		return models.Image{}, err
	}
	return img, makeMain(tx, dir, userID, contact, img, eff)
}

// MigrateContactPhotos gives every contact that has a profile photo and no images one image made
// from that photo. The avatar files are not touched, so the vCard of the contact stays the same.
// A contact whose photo cannot be read is logged and skipped. It returns how many it migrated.
func MigrateContactPhotos(ctx context.Context, db *gorm.DB, dir string) (int, error) {
	var contacts []models.Contact
	err := db.WithContext(ctx).Unscoped().Select("id", "user_id", "photo").
		Where("photo <> '' AND main_image_id IS NULL AND NOT EXISTS (SELECT 1 FROM images WHERE images.contact_id = contacts.id)").
		Find(&contacts).Error
	if err != nil {
		return 0, err
	}
	migrated := 0
	for _, c := range contacts {
		if err := migrateContactPhoto(ctx, db, dir, c); err != nil {
			logger.Warn().Err(err).Uint("contact_id", c.ID).Msg("Skipping a profile photo that cannot be turned into an image")
			continue
		}
		migrated++
	}
	return migrated, nil
}

var errPhotoChanged = errors.New("the profile photo changed during the migration")

func migrateContactPhoto(ctx context.Context, db *gorm.DB, dir string, c models.Contact) error {
	if strings.Contains(c.Photo, "..") || filepath.IsAbs(c.Photo) {
		return fmt.Errorf("unsafe photo path %q", c.Photo)
	}
	data, err := os.ReadFile(filepath.Join(dir, c.Photo)) //nolint:gosec // checked above, directory from config
	if err != nil {
		return err
	}
	img, err := StoreImage(ctx, dir, c.UserID, data)
	if err != nil {
		return err
	}
	contactID := c.ID
	size := min(img.Width, img.Height)
	zero := 0
	img.ContactID, img.CropX, img.CropY, img.CropSize = &contactID, &zero, &zero, &size
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&img).Error; err != nil {
			return err
		}
		res := tx.Unscoped().Model(&models.Contact{}).
			Where("id = ? AND user_id = ? AND photo = ? AND main_image_id IS NULL", c.ID, c.UserID, c.Photo).
			Update("main_image_id", img.ID)
		if res.Error == nil && res.RowsAffected == 0 {
			return errPhotoChanged
		}
		return res.Error
	})
	if err != nil {
		photofile.Remove(dir, img.File, img.ThumbFile)
	}
	return err
}

// maxSamePictureDistance is the Hamming distance up to which two difference hashes show one picture
// that was only re-encoded or resized.
const maxSamePictureDistance = 8

// ImportContactPhoto adds a photo that comes from outside (CardDAV, vCard, Monica) as the first and
// main image of the contact, unless it shows the picture the contact has as its avatar already: a
// phone that re-encodes the photo must not add an image on every sync. It reports whether it added one.
func ImportContactPhoto(ctx context.Context, tx *gorm.DB, dir string, userID, contactID uint, data []byte, eff *ImageEffects) (bool, error) {
	res, err := imageproc.Prepare(ctx, data)
	if err != nil {
		return false, err
	}
	return importPrepared(tx, dir, userID, contactID, res, eff)
}

// ImportContactPhotoAt is ImportContactPhoto in a transaction of its own. The picture is decoded
// before the transaction starts, so the database is not locked meanwhile.
func ImportContactPhotoAt(ctx context.Context, db *gorm.DB, dir string, userID, contactID uint, data []byte) (bool, error) {
	res, err := imageproc.Prepare(ctx, data)
	if err != nil {
		return false, err
	}
	var eff ImageEffects
	var added bool
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		added, err = importPrepared(tx, dir, userID, contactID, res, &eff)
		return err
	})
	eff.Finish(dir, err)
	return added && err == nil, err
}

func importPrepared(tx *gorm.DB, dir string, userID, contactID uint, res *imageproc.Result, eff *ImageEffects) (bool, error) {
	contact, err := loadContact(tx, userID, contactID)
	if err != nil {
		return false, err
	}
	incoming, err := centredHash(res.Thumb)
	if err != nil {
		return false, err
	}
	if current, ok := avatarHash(contact, dir); ok && bits.OnesCount64(current^incoming) <= maxSamePictureDistance {
		return false, nil
	}
	img, err := storePrepared(dir, userID, res)
	if err != nil {
		return false, err
	}
	_, err = AttachContactImage(tx, dir, userID, contactID, img, true, eff)
	return err == nil, err
}

func centredHash(webp []byte) (uint64, error) {
	src, err := imageproc.Decode(webp)
	if err != nil {
		return 0, err
	}
	b := src.Image.Bounds()
	x, y, size := imageproc.CenteredCrop(b.Dx(), b.Dy())
	return imageproc.DHash(src.Image, x, y, size), nil
}

// avatarHash is the hash of the contact's avatar: stored, or else read from the file for avatars
// that were made before hashes were kept.
func avatarHash(c models.Contact, dir string) (uint64, bool) {
	if c.PhotoDHash != nil {
		return uint64(*c.PhotoDHash), true //nolint:gosec // the bits were stored as they are
	}
	if c.Photo == "" || strings.Contains(c.Photo, "..") || strings.ContainsAny(c.Photo, "/\\") {
		return 0, false
	}
	data, err := os.ReadFile(filepath.Join(dir, c.Photo)) //nolint:gosec // a plain file name, directory from config
	if err != nil {
		return 0, false
	}
	src, err := imageproc.Decode(data)
	if err != nil {
		return 0, false
	}
	return imageproc.DHash(src.Image, 0, 0, min(src.Image.Bounds().Dx(), src.Image.Bounds().Dy())), true
}
