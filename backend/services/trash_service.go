package services

import (
	"errors"
	"fmt"
	"time"

	"meerkat/models"
	"meerkat/photofile"

	"gorm.io/gorm"
)

// ErrTrashedContactNotFound means the contact does not exist, is not in the
// trash, or belongs to another user.
var ErrTrashedContactNotFound = errors.New("contact not in trash")

// dependantWindow is how far before a contact's own deletion its dependants
// (notes, reminders, relationships) may have been soft-deleted and still count
// as deleted with it. DeleteContact removes them inside one transaction, so the
// gap is milliseconds; a note the user deleted earlier stays deleted on restore.
const dependantWindow = 10 * time.Second

// purgeChunk bounds how many contacts one purge transaction touches.
const purgeChunk = 200

// ListTrashedContacts returns the user's soft-deleted contacts, newest deletion first.
func ListTrashedContacts(db *gorm.DB, userID uint) ([]models.Contact, error) {
	var contacts []models.Contact
	err := db.Unscoped().
		Where("user_id = ? AND deleted_at IS NOT NULL", userID).
		Order("deleted_at DESC, id DESC").
		Find(&contacts).Error
	return contacts, err
}

func findTrashed(tx *gorm.DB, userID, id uint) (models.Contact, error) {
	var contact models.Contact
	err := tx.Unscoped().Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", id, userID).First(&contact).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return contact, ErrTrashedContactNotFound
	}
	return contact, err
}

// RestoreTrashedContact un-deletes a contact together with the notes, reminders
// and relationships that were deleted with it. Its activity links were never
// removed, so activities show it again. The unique (user_id, vcard_uid) index
// still covers soft-deleted rows, so a restore cannot collide with another contact.
func RestoreTrashedContact(db *gorm.DB, userID, id uint) (*models.Contact, error) {
	var restored *models.Contact
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		restored, err = restoreTrashedContactTx(tx, userID, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return restored, nil
}

// restoreTrashedContactTx is RestoreTrashedContact inside a caller's transaction;
// the CardDAV sync reuses it when a remote edit revives a trashed contact.
func restoreTrashedContactTx(tx *gorm.DB, userID, id uint) (*models.Contact, error) {
	var restored models.Contact
	contact, err := findTrashed(tx, userID, id)
	if err != nil {
		return nil, err
	}
	deletedAt := contact.DeletedAt.Time

	restoreDependants := func(dest any) error {
		return tx.Unscoped().Where("contact_id = ? AND user_id = ? AND deleted_at IS NOT NULL", id, userID).
			Find(dest).Error
	}

	var notes []models.Note
	if err := restoreDependants(&notes); err != nil {
		return nil, err
	}
	var reminders []models.Reminder
	if err := restoreDependants(&reminders); err != nil {
		return nil, err
	}
	var rels []models.Relationship
	if err := restoreDependants(&rels); err != nil {
		return nil, err
	}

	revive := func(model any, ids []uint) error {
		if len(ids) == 0 {
			return nil
		}
		return tx.Unscoped().Model(model).Where("id IN ?", ids).UpdateColumn("deleted_at", nil).Error
	}
	var noteIDs, reminderIDs, relIDs []uint
	for _, n := range notes {
		if deletedWith(n.DeletedAt, deletedAt) {
			noteIDs = append(noteIDs, n.ID)
		}
	}
	for _, r := range reminders {
		if deletedWith(r.DeletedAt, deletedAt) {
			reminderIDs = append(reminderIDs, r.ID)
		}
	}
	for _, r := range rels {
		if deletedWith(r.DeletedAt, deletedAt) {
			relIDs = append(relIDs, r.ID)
		}
	}
	if err := revive(&models.Note{}, noteIDs); err != nil {
		return nil, err
	}
	if err := revive(&models.Reminder{}, reminderIDs); err != nil {
		return nil, err
	}
	if err := revive(&models.Relationship{}, relIDs); err != nil {
		return nil, err
	}

	// UpdateColumn skips the Contact hooks, which would rewrite fields.
	if err := tx.Unscoped().Model(&models.Contact{}).Where("id = ? AND user_id = ?", id, userID).
		UpdateColumn("deleted_at", nil).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("id = ? AND user_id = ?", id, userID).First(&restored).Error; err != nil {
		return nil, err
	}
	return &restored, nil
}

func deletedWith(dependant gorm.DeletedAt, contactDeletedAt time.Time) bool {
	if !dependant.Valid {
		return false
	}
	d := contactDeletedAt.Sub(dependant.Time)
	return d >= -dependantWindow && d <= dependantWindow
}

// PurgeTrashedContacts permanently deletes trashed contacts of one user (ids
// nil or empty means the whole trash) with their dependants, then removes their
// photo files from photoDir. Live contacts and other users' contacts are never
// touched. It returns how many contacts were removed.
func PurgeTrashedContacts(db *gorm.DB, userID uint, ids []uint, photoDir string, now time.Time) (int, error) {
	q := db.Unscoped().Select("id", "user_id", "photo", "photo_thumbnail", "photo_thumbnail_file", "deleted_at").
		Where("user_id = ? AND deleted_at IS NOT NULL", userID)
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var contacts []models.Contact
	if err := q.Find(&contacts).Error; err != nil {
		return 0, err
	}
	return purgeContacts(db, contacts, photoDir, now)
}

// PurgeExpiredTrash permanently deletes, for every user, contacts deleted more
// than retentionDays before now. A retention below one day purges nothing.
func PurgeExpiredTrash(db *gorm.DB, retentionDays int, photoDir string, now time.Time) (int, error) {
	if retentionDays < 1 {
		return 0, nil
	}
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	var trashed []models.Contact
	if err := db.Unscoped().Select("id", "user_id", "photo", "photo_thumbnail", "photo_thumbnail_file", "deleted_at").
		Where("deleted_at IS NOT NULL").Find(&trashed).Error; err != nil {
		return 0, err
	}
	// Compared in Go: SQLite holds timestamps as text, which only sorts
	// correctly when every value has the same UTC offset.
	var expired []models.Contact
	for _, c := range trashed {
		if c.DeletedAt.Time.Before(cutoff) {
			expired = append(expired, c)
		}
	}
	return purgeContacts(db, expired, photoDir, now)
}

// purgeContacts purges the candidates that are still in the trash exactly as
// listed. The caller read them outside any transaction, so each chunk is
// re-checked inside its own: a contact restored (or restored and deleted again)
// in between keeps its dependants and photo files.
func purgeContacts(db *gorm.DB, candidates []models.Contact, photoDir string, now time.Time) (int, error) {
	purged := 0
	for start := 0; start < len(candidates); start += purgeChunk {
		end := min(start+purgeChunk, len(candidates))
		var confirmed []models.Contact
		var imageFiles []string
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			confirmed, imageFiles, err = purgeChunkTx(tx, candidates[start:end], now)
			return err
		})
		if err != nil {
			return purged, fmt.Errorf("failed to purge contacts: %w", err)
		}
		purged += len(confirmed)
		// Files go after the commit: a failed transaction must not lose photos.
		for _, c := range confirmed {
			RemoveContactPhotoFiles(photoDir, c)
		}
		photofile.Remove(photoDir, imageFiles...)
	}
	return purged, nil
}

// purgeChunkTx hard-deletes the candidates that are still trashed with the same
// deletion time, and everything that belongs to them. It returns those contacts.
// CardDAV links are kept on purpose and marked purged (a tombstone): the sync
// engine uses them to delete the remote object (two-way, push) or to never import
// it again (pull-only), and drops the link itself when it is done. The image rows of the
// contacts and of their notes are deleted with them; their files are returned.
func purgeChunkTx(tx *gorm.DB, chunk []models.Contact, now time.Time) ([]models.Contact, []string, error) {
	var imageFiles []string
	byUser := map[uint][]models.Contact{}
	for _, c := range chunk {
		byUser[c.UserID] = append(byUser[c.UserID], c)
	}
	var confirmed []models.Contact
	for userID, candidates := range byUser {
		candidateIDs := make([]uint, 0, len(candidates))
		listedAt := make(map[uint]time.Time, len(candidates))
		for _, c := range candidates {
			candidateIDs = append(candidateIDs, c.ID)
			listedAt[c.ID] = c.DeletedAt.Time
		}
		var current []models.Contact
		if err := tx.Unscoped().Select("id", "user_id", "photo", "photo_thumbnail", "photo_thumbnail_file", "deleted_at").
			Where("id IN ? AND user_id = ? AND deleted_at IS NOT NULL", candidateIDs, userID).
			Find(&current).Error; err != nil {
			return nil, nil, err
		}
		var ids []uint
		for _, c := range current {
			if c.DeletedAt.Time.Equal(listedAt[c.ID]) {
				ids = append(ids, c.ID)
				confirmed = append(confirmed, c)
			}
		}
		if len(ids) == 0 {
			continue
		}
		var images []models.Image
		if err := tx.Unscoped().Select("file", "thumb_file").
			Where("user_id = ? AND (contact_id IN ? OR note_id IN (SELECT id FROM notes WHERE contact_id IN ? AND user_id = ?))", userID, ids, ids, userID).
			Find(&images).Error; err != nil {
			return nil, nil, err
		}
		for _, img := range images {
			imageFiles = append(imageFiles, img.File, img.ThumbFile)
		}
		if len(images) > 0 {
			if err := tx.Unscoped().
				Where("user_id = ? AND (contact_id IN ? OR note_id IN (SELECT id FROM notes WHERE contact_id IN ? AND user_id = ?))", userID, ids, ids, userID).
				Delete(&models.Image{}).Error; err != nil {
				return nil, nil, err
			}
		}
		if err := tx.Exec("DELETE FROM activity_contacts WHERE contact_id IN ? AND activity_id IN (SELECT id FROM activities WHERE user_id = ?)", ids, userID).Error; err != nil {
			return nil, nil, err
		}
		for _, model := range []any{&models.Note{}, &models.Reminder{}, &models.ReminderCompletion{}, &models.Relationship{}} {
			if err := tx.Unscoped().Where("contact_id IN ? AND user_id = ?", ids, userID).Delete(model).Error; err != nil {
				return nil, nil, err
			}
		}
		// Other contacts' relationships that point here become plain name entries.
		if err := tx.Unscoped().Model(&models.Relationship{}).
			Where("related_contact_id IN ? AND user_id = ?", ids, userID).
			UpdateColumn("related_contact_id", nil).Error; err != nil {
			return nil, nil, err
		}
		if err := tx.Model(&models.CardDAVContactLink{}).
			Where("contact_id IN ? AND user_id = ? AND purged_at IS NULL", ids, userID).
			UpdateColumn("purged_at", now).Error; err != nil {
			return nil, nil, err
		}
		if err := tx.Unscoped().Where("id IN ? AND user_id = ? AND deleted_at IS NOT NULL", ids, userID).
			Delete(&models.Contact{}).Error; err != nil {
			return nil, nil, err
		}
	}
	return confirmed, imageFiles, nil
}

// RemoveContactPhotoFiles deletes the profile photo and the thumbnail file of a contact
// (and a legacy file-named thumbnail; base64 data URLs have no file).
// Failures are logged, not returned: the database change already happened.
func RemoveContactPhotoFiles(photoDir string, contact models.Contact) {
	photofile.Remove(photoDir, contact.Photo, contact.PhotoThumbnailFile, contact.LegacyThumbnail)
}
