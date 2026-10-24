package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"meerkat/carddav"
	"meerkat/clock"
	"meerkat/config"
	apperrors "meerkat/errors"
	"meerkat/models"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

// sessionExpiry is how long an in-progress import wizard session is kept server-side.
const sessionExpiry = 15 * time.Minute

// importSessionData holds the server-side state for an in-progress import wizard.
// Sessions are kept in memory only (not persisted) and are lost on restart.
type importSessionData struct {
	session     models.ImportSession
	rows        [][]string       // CSV rows (nil for VCF imports)
	importType  string           // "csv" or "vcf"
	vcfContacts []VCFContactData // VCF parsed contacts (nil for CSV imports)
	csvContacts []models.Contact // CSV contacts built during preview (nil for VCF imports)
	photos      *models.ImportPhotoStatus
	confirmed   bool
}

// ImportSessionManager owns the lifecycle of in-progress import sessions: creation,
// retrieval/validation, preview generation, confirmation, and expiry. It is the single
// owner of import state so controllers only need to validate input and delegate.
type ImportSessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*importSessionData
	clock    atomic.Pointer[clock.Clock]
}

// NewImportSessionManager creates an empty session manager.
func NewImportSessionManager() *ImportSessionManager {
	m := &ImportSessionManager{sessions: make(map[string]*importSessionData)}
	m.SetClock(clock.Real())
	return m
}

// SetClock replaces the clock the manager reads for session expiry (set once at startup).
func (m *ImportSessionManager) SetClock(c clock.Clock) { m.clock.Store(&c) }

func (m *ImportSessionManager) now() time.Time { return (*m.clock.Load()).Now() }

// generateSessionID creates a random session ID.
func generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// avoidVCardUIDCollision mints a fresh VCardUID for contact if its current one is
// already taken by another row for this user (soft-deleted tombstone or live
// contact). Called before creating a contact via the "add" action: choosing "add"
// is an explicit request for a separate contact, so it must not collide with the
// unique (user_id, vcard_uid) index the deleted/live row is still holding.
func avoidVCardUIDCollision(tx *gorm.DB, userID uint, contact *models.Contact, log *zerolog.Logger) {
	if contact.VCardUID == "" {
		return
	}
	var collision models.Contact
	if err := tx.Unscoped().
		Where("user_id = ? AND vcard_uid = ?", userID, contact.VCardUID).
		First(&collision).Error; err == nil {
		originalUID := contact.VCardUID
		contact.VCardUID = uuid.New().String()
		log.Debug().
			Str("original_vcard_uid", originalUID).
			Str("new_vcard_uid", contact.VCardUID).
			Uint("held_by_contact_id", collision.ID).
			Bool("held_by_deleted_contact", collision.DeletedAt.Valid).
			Msg("Imported vCard UID already in use, minted a fresh one for the new contact")
	}
}

// CleanupExpired removes expired import sessions. Safe to call from a goroutine.
func (m *ImportSessionManager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	for id, data := range m.sessions {
		if now.After(data.session.ExpiresAt) {
			delete(m.sessions, id)
		}
	}
}

// get retrieves and validates an import session, enforcing ownership and expiry.
func (m *ImportSessionManager) get(sessionID string, userID uint) (*importSessionData, *apperrors.AppError) {
	m.mu.RLock()
	sessionData, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return nil, apperrors.ErrNotFound("Import session expired or not found")
	}

	if sessionData.session.UserID != userID {
		return nil, apperrors.ErrNotFound("Import session expired or not found")
	}

	m.mu.RLock()
	expiresAt := sessionData.session.ExpiresAt
	m.mu.RUnlock()
	if m.now().After(expiresAt) {
		m.mu.Lock()
		delete(m.sessions, sessionID)
		m.mu.Unlock()
		return nil, apperrors.ErrNotFound("Import session expired")
	}

	return sessionData, nil
}

// claim marks a session as confirmed, so a repeated or concurrent confirm cannot import it twice.
func (m *ImportSessionManager) claim(sd *importSessionData) *apperrors.AppError {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sd.confirmed {
		return apperrors.ErrInvalidInput("session", "This import was already confirmed")
	}
	sd.confirmed = true
	return nil
}

func (m *ImportSessionManager) unclaim(sd *importSessionData) {
	m.mu.Lock()
	sd.confirmed = false
	m.mu.Unlock()
}

// Delete removes a session, typically after a completed import.
func (m *ImportSessionManager) Delete(sessionID string) {
	m.mu.Lock()
	delete(m.sessions, sessionID)
	m.mu.Unlock()
}

// CreateCSVSession stores a freshly parsed CSV upload and returns its session ID.
func (m *ImportSessionManager) CreateCSVSession(userID uint, headers []string, rows [][]string) string {
	sessionID := generateSessionID()
	now := m.now()

	m.mu.Lock()
	m.sessions[sessionID] = &importSessionData{
		session: models.ImportSession{
			ID:        sessionID,
			UserID:    userID,
			Headers:   headers,
			Rows:      rows,
			CreatedAt: now,
			ExpiresAt: now.Add(sessionExpiry),
		},
		rows:       rows,
		importType: "csv",
	}
	m.mu.Unlock()

	return sessionID
}

// CreateVCFSession stores a freshly parsed VCF upload (whose preview is computed at
// upload time) and returns its session ID.
func (m *ImportSessionManager) CreateVCFSession(userID uint, vcfContacts []VCFContactData, previews []models.ImportRowPreview) string {
	sessionID := generateSessionID()
	now := m.now()

	m.mu.Lock()
	m.sessions[sessionID] = &importSessionData{
		session: models.ImportSession{
			ID:            sessionID,
			UserID:        userID,
			CreatedAt:     now,
			ExpiresAt:     now.Add(sessionExpiry),
			PreviewRows:   previews,
			PreviewCached: true,
		},
		importType:  "vcf",
		vcfContacts: vcfContacts,
	}
	m.mu.Unlock()

	return sessionID
}

// PreviewCSV applies mappings to a CSV session, caches the built contacts and preview
// rows for the confirm step, and returns the preview response.
func (m *ImportSessionManager) PreviewCSV(db *gorm.DB, userID uint, req models.ImportPreviewRequest) (*models.ImportPreviewResponse, *apperrors.AppError) {
	sessionData, err := m.get(req.SessionID, userID)
	if err != nil {
		return nil, err
	}

	contacts, previews, stats := GenerateCSVPreview(db, userID, sessionData.rows, sessionData.session.Headers, req.Mappings)

	// Cache preview data and built contacts in the session for the confirm step.
	m.mu.Lock()
	if sd, exists := m.sessions[req.SessionID]; exists {
		sd.session.Mappings = req.Mappings
		sd.session.PreviewRows = previews
		sd.session.PreviewCached = true
		sd.csvContacts = contacts
	}
	m.mu.Unlock()

	return &models.ImportPreviewResponse{
		SessionID:      req.SessionID,
		Rows:           previews,
		TotalRows:      len(previews),
		ValidRows:      stats.ValidCount,
		DuplicateCount: stats.DuplicateCount,
		ErrorCount:     stats.ErrorCount,
	}, nil
}

// Confirm executes a CSV or VCF import using the per-row actions in req, then deletes
// the session. Photo processing is handled separately by ConfirmVCF.
func (m *ImportSessionManager) Confirm(db *gorm.DB, userID uint, req models.ImportConfirmRequest, log *zerolog.Logger) (*models.ImportResult, *apperrors.AppError) {
	sessionData, sessErr := m.get(req.SessionID, userID)
	if sessErr != nil {
		return nil, sessErr
	}

	if !sessionData.session.PreviewCached {
		return nil, apperrors.ErrInvalidInput("session", "Please generate a preview first")
	}
	if err := m.claim(sessionData); err != nil {
		return nil, err
	}

	actionMap := buildActionMap(req.Actions)
	result := models.ImportResult{Errors: []string{}, SkippedRows: []models.SkippedRow{}}
	isVCFImport := sessionData.importType == "vcf"

	txErr := db.Transaction(func(tx *gorm.DB) error {
		for _, preview := range sessionData.session.PreviewRows {
			action := actionMap[preview.RowIndex]
			if action == "" || len(preview.ValidationErrors) > 0 {
				action = "skip"
			}

			result.TotalProcessed++

			switch action {
			case "skip":
				result.SkipChosenRow(&preview)

			case "add":
				var contact models.Contact
				if isVCFImport {
					contact = *sessionData.vcfContacts[preview.RowIndex].Contact
				} else {
					contact = sessionData.csvContacts[preview.RowIndex]
				}
				contact.UserID = userID
				avoidVCardUIDCollision(tx, userID, &contact, log)

				if err := tx.Create(&contact).Error; err != nil {
					result.FailRow(&preview, fmt.Sprintf("Failed to create contact: %v", err))
				} else {
					result.Created++
					if isVCFImport {
						sessionData.vcfContacts[preview.RowIndex].Contact.ID = contact.ID
					}
				}

			case "update":
				if preview.DuplicateMatch == nil {
					result.FailRow(&preview, "Cannot update - no existing contact found")
					continue
				}

				var existing models.Contact
				if err := tx.Unscoped().First(&existing, preview.DuplicateMatch.ExistingContactID).Error; err != nil {
					result.FailRow(&preview, fmt.Sprintf("Failed to fetch existing contact: %v", err))
					continue
				}

				importType := "CSV"
				if isVCFImport {
					importType = "VCF"
				}
				if err := CreateMergeNote(tx, userID, existing.ID, &existing, preview.ParsedContact, importType); err != nil {
					log.Warn().Err(err).Uint("contact_id", existing.ID).Msg("Failed to create merge note")
				}

				if isVCFImport {
					MergeImportedContact(&existing, sessionData.vcfContacts[preview.RowIndex].Contact)
				} else {
					csvContact := sessionData.csvContacts[preview.RowIndex]
					MergeImportedContact(&existing, &csvContact)
				}

				// A matched vcard_uid can point at a soft-deleted contact (re-importing
				// a previously deleted vCard); restore it instead of leaving it deleted.
				wasDeleted := existing.DeletedAt.Valid
				existing.DeletedAt = gorm.DeletedAt{}
				if wasDeleted {
					existing.Photo = ""
					existing.PhotoThumbnailFile = ""
					existing.LegacyThumbnail = ""
				}

				saveTx := tx
				if wasDeleted {
					saveTx = tx.Unscoped()
				}
				if err := saveTx.Save(&existing).Error; err != nil {
					result.FailRow(&preview, fmt.Sprintf("Failed to update contact: %v", err))
				} else {
					result.Updated++
					if isVCFImport {
						sessionData.vcfContacts[preview.RowIndex].Contact.ID = existing.ID
					}
				}
			}
		}

		return nil
	})

	if txErr != nil {
		m.unclaim(sessionData)
		return nil, apperrors.ErrDatabase("Import failed").WithError(txErr)
	}

	m.Delete(req.SessionID)

	log.Info().
		Str("session_id", req.SessionID).
		Str("import_type", sessionData.importType).
		Int("created", result.Created).
		Int("updated", result.Updated).
		Int("skipped", result.Skipped).
		Int("errors", len(result.Errors)).
		Msg("Import completed")

	return &result, nil
}

// photoTask queues photo processing for a contact, deferred until after the import
// transaction commits since it involves file I/O and network requests.
type photoTask struct {
	contactID uint
	photoData []byte
	photoURL  string // URL to fetch photo from (if not embedded)
}

// ConfirmVCF executes a VCF import with photo processing, then deletes the session.
func (m *ImportSessionManager) ConfirmVCF(db *gorm.DB, userID uint, req models.ImportConfirmRequest, cfg *config.Config, log *zerolog.Logger) (*models.ImportResult, *apperrors.AppError) {
	sessionData, sessErr := m.get(req.SessionID, userID)
	if sessErr != nil {
		return nil, sessErr
	}

	if sessionData.importType != "vcf" {
		return nil, apperrors.ErrInvalidInput("session", "This endpoint is only for VCF imports")
	}

	if !sessionData.session.PreviewCached {
		return nil, apperrors.ErrInvalidInput("session", "Please generate a preview first")
	}

	if err := m.claim(sessionData); err != nil {
		return nil, err
	}

	actionMap := buildActionMap(req.Actions)
	result := models.ImportResult{Errors: []string{}, SkippedRows: []models.SkippedRow{}}
	var photoTasks []photoTask

	txErr := db.Transaction(func(tx *gorm.DB) error {
		for _, preview := range sessionData.session.PreviewRows {
			action := actionMap[preview.RowIndex]
			if action == "" || len(preview.ValidationErrors) > 0 {
				action = "skip"
			}

			result.TotalProcessed++
			vcfData := sessionData.vcfContacts[preview.RowIndex]

			switch action {
			case "skip":
				result.SkipChosenRow(&preview)

			case "add":
				contact := *vcfData.Contact
				contact.UserID = userID
				avoidVCardUIDCollision(tx, userID, &contact, log)

				if err := tx.Create(&contact).Error; err != nil {
					result.FailRow(&preview, fmt.Sprintf("Failed to create contact: %v", err))
				} else {
					result.Created++
					// Queue photo processing (either embedded data or URL)
					if len(vcfData.PhotoData) > 0 || vcfData.PhotoURL != "" {
						photoTasks = append(photoTasks, photoTask{
							contactID: contact.ID,
							photoData: vcfData.PhotoData,
							photoURL:  vcfData.PhotoURL,
						})
					}
				}

			case "update":
				if preview.DuplicateMatch == nil {
					result.FailRow(&preview, "Cannot update - no existing contact found")
					continue
				}

				var existing models.Contact
				if err := tx.Unscoped().First(&existing, preview.DuplicateMatch.ExistingContactID).Error; err != nil {
					result.FailRow(&preview, fmt.Sprintf("Failed to fetch existing contact: %v", err))
					continue
				}

				if err := CreateMergeNote(tx, userID, existing.ID, &existing, preview.ParsedContact, "VCF"); err != nil {
					log.Warn().Err(err).Uint("contact_id", existing.ID).Msg("Failed to create merge note")
				}

				MergeImportedContact(&existing, vcfData.Contact)

				// A matched vcard_uid can point at a soft-deleted contact (re-importing
				// a previously deleted vCard); restore it instead of leaving it deleted.
				wasDeleted := existing.DeletedAt.Valid
				existing.DeletedAt = gorm.DeletedAt{}
				if wasDeleted {
					existing.Photo = ""
					existing.PhotoThumbnailFile = ""
					existing.LegacyThumbnail = ""
				}

				saveTx := tx
				if wasDeleted {
					saveTx = tx.Unscoped()
				}
				if err := saveTx.Save(&existing).Error; err != nil {
					result.FailRow(&preview, fmt.Sprintf("Failed to update contact: %v", err))
				} else {
					result.Updated++
					// Queue photo processing only if contact doesn't already have a photo
					if existing.Photo == "" && (len(vcfData.PhotoData) > 0 || vcfData.PhotoURL != "") {
						photoTasks = append(photoTasks, photoTask{
							contactID: existing.ID,
							photoData: vcfData.PhotoData,
							photoURL:  vcfData.PhotoURL,
						})
					}
				}
			}
		}

		return nil
	})

	if txErr != nil {
		m.unclaim(sessionData)
		return nil, apperrors.ErrDatabase("Import failed").WithError(txErr)
	}

	result.PhotosQueued = len(photoTasks)
	log.Info().
		Str("session_id", req.SessionID).
		Int("created", result.Created).
		Int("updated", result.Updated).
		Int("skipped", result.Skipped).
		Int("photos_queued", len(photoTasks)).
		Int("errors", len(result.Errors)).
		Msg("VCF import completed")

	if len(photoTasks) == 0 {
		m.Delete(req.SessionID)
		return &result, nil
	}

	m.mu.Lock()
	sessionData.photos = &models.ImportPhotoStatus{Phase: "importing_photos", Total: len(photoTasks)}
	sessionData.vcfContacts = nil
	sessionData.session.PreviewRows = nil
	sessionData.session.ExpiresAt = m.now().Add(sessionExpiry)
	m.mu.Unlock()
	go m.processPhotos(db.WithContext(context.Background()), userID, sessionData, photoTasks, cfg, log)

	return &result, nil
}

// processPhotos stores the photos of a confirmed vCard import (fetching photo URLs) and
// reports progress through the session for status polling.
func (m *ImportSessionManager) processPhotos(db *gorm.DB, userID uint, sessionData *importSessionData, tasks []photoTask, cfg *config.Config, log *zerolog.Logger) {
	for _, task := range tasks {
		saved := m.savePhoto(db, userID, task, cfg, log)
		m.mu.Lock()
		p := sessionData.photos
		p.Done++
		if saved {
			p.Saved++
		} else {
			p.Failed++
		}
		if p.Done == p.Total {
			p.Phase = "done"
		}
		sessionData.session.ExpiresAt = m.now().Add(sessionExpiry)
		m.mu.Unlock()
	}
}

func (m *ImportSessionManager) savePhoto(db *gorm.DB, userID uint, task photoTask, cfg *config.Config, log *zerolog.Logger) bool {
	photoData := task.photoData
	if len(photoData) == 0 {
		var err error
		photoData, _, err = carddav.FetchPhotoFromURL(context.Background(), task.photoURL)
		if err != nil {
			log.Warn().Err(err).Uint("contact_id", task.contactID).Str("photo_url", task.photoURL).Msg("Failed to fetch photo from URL")
			return false
		}
	}
	if len(photoData) == 0 {
		return false
	}

	if _, err := ImportContactPhotoAt(context.Background(), db, cfg.ProfilePhotoDir, userID, task.contactID, photoData); err != nil {
		log.Warn().Err(err).Uint("contact_id", task.contactID).Msg("Failed to save imported photo")
		return false
	}
	return true
}

// PhotoStatus returns the photo progress of a confirmed vCard import.
func (m *ImportSessionManager) PhotoStatus(userID uint, sessionID string) (models.ImportPhotoStatus, *apperrors.AppError) {
	sessionData, err := m.get(sessionID, userID)
	if err != nil {
		return models.ImportPhotoStatus{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if sessionData.photos == nil {
		return models.ImportPhotoStatus{}, apperrors.ErrNotFound("This import has no photos to load")
	}
	return *sessionData.photos, nil
}

// buildActionMap indexes per-row import actions by row index.
func buildActionMap(actions []models.RowImportAction) map[int]string {
	actionMap := make(map[int]string, len(actions))
	for _, action := range actions {
		actionMap[action.RowIndex] = action.Action
	}
	return actionMap
}
