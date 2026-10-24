package services

import (
	"errors"

	"meerkat/models"

	"gorm.io/gorm"
)

type ImageOwnerKind string

const (
	ImageOfContact  ImageOwnerKind = "contact"
	ImageOfActivity ImageOwnerKind = "activity"
	ImageOfNote     ImageOwnerKind = "note"
)

const (
	MaxImagesPerNote     = 20
	MaxImagesPerActivity = 20
	MaxImagesPerContact  = 50
)

var (
	// ErrImageNotFound means an image is not the user's, is listed twice, or belongs to something else.
	ErrImageNotFound = errors.New("image not found")
	ErrTooManyImages = errors.New("too many images")
)

type ImageOwner struct {
	Kind ImageOwnerKind
	ID   uint
}

func (k ImageOwnerKind) column() string { return string(k) + "_id" }

func (k ImageOwnerKind) limit() int {
	switch k {
	case ImageOfContact:
		return MaxImagesPerContact
	case ImageOfActivity:
		return MaxImagesPerActivity
	}
	return MaxImagesPerNote
}

// SetOwnerImages makes ids the ordered images of owner. Each must be the user's and pending, or
// already on this owner; a contact may also take images of another contact of the user (a
// move). Images of the owner that are not listed are deleted; their file names are returned
// for the caller to remove after the commit.
func SetOwnerImages(tx *gorm.DB, userID uint, owner ImageOwner, ids []uint) (removedFiles []string, err error) {
	if len(ids) > owner.Kind.limit() {
		return nil, ErrTooManyImages
	}
	listed := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if listed[id] {
			return nil, ErrImageNotFound
		}
		listed[id] = true
	}
	var wanted []models.Image
	if len(ids) > 0 {
		if err := tx.Where("user_id = ? AND id IN ?", userID, ids).Find(&wanted).Error; err != nil {
			return nil, err
		}
	}
	if len(wanted) != len(ids) {
		return nil, ErrImageNotFound
	}
	for _, img := range wanted {
		if !canTake(img, owner) {
			return nil, ErrImageNotFound
		}
	}

	var current []models.Image
	if err := tx.Where("user_id = ? AND "+owner.Kind.column()+" = ?", userID, owner.ID).Find(&current).Error; err != nil {
		return nil, err
	}
	var drop []uint
	for _, img := range current {
		if !listed[img.ID] {
			drop = append(drop, img.ID)
			removedFiles = append(removedFiles, img.File, img.ThumbFile)
		}
	}
	if len(drop) > 0 {
		if err := tx.Where("user_id = ? AND id IN ?", userID, drop).Delete(&models.Image{}).Error; err != nil {
			return nil, err
		}
	}
	for position, id := range ids {
		update := map[string]any{"contact_id": nil, "activity_id": nil, "note_id": nil, "position": position}
		update[owner.Kind.column()] = owner.ID
		if err := tx.Model(&models.Image{}).Where("user_id = ? AND id = ?", userID, id).Updates(update).Error; err != nil {
			return nil, err
		}
	}
	return removedFiles, nil
}

func canTake(img models.Image, owner ImageOwner) bool {
	pending := img.ContactID == nil && img.ActivityID == nil && img.NoteID == nil
	switch owner.Kind {
	case ImageOfContact:
		return pending || img.ContactID != nil
	case ImageOfActivity:
		return pending || (img.ActivityID != nil && *img.ActivityID == owner.ID)
	}
	return pending || (img.NoteID != nil && *img.NoteID == owner.ID)
}

// DeleteOwnerImages deletes the images of owner for good and returns their file names.
func DeleteOwnerImages(tx *gorm.DB, userID uint, owner ImageOwner) (files []string, err error) {
	var rows []models.Image
	if err := tx.Where("user_id = ? AND "+owner.Kind.column()+" = ?", userID, owner.ID).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, img := range rows {
		files = append(files, img.File, img.ThumbFile)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return files, tx.Where("user_id = ? AND "+owner.Kind.column()+" = ?", userID, owner.ID).Delete(&models.Image{}).Error
}

// ImagesFor returns the images of the given owners of one kind in one query, in order.
func ImagesFor(db *gorm.DB, userID uint, kind ImageOwnerKind, ownerIDs []uint) (map[uint][]models.ImageRef, error) {
	out := map[uint][]models.ImageRef{}
	if len(ownerIDs) == 0 {
		return out, nil
	}
	var rows []models.Image
	col := kind.column()
	if err := db.Where("user_id = ? AND "+col+" IN ?", userID, ownerIDs).Order("position, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, img := range rows {
		var owner uint
		switch kind {
		case ImageOfContact:
			owner = *img.ContactID
		case ImageOfActivity:
			owner = *img.ActivityID
		default:
			owner = *img.NoteID
		}
		out[owner] = append(out[owner], img.Ref())
	}
	return out, nil
}
