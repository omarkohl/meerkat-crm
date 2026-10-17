package controllers

import (
	"errors"
	"meerkat/config"
	"meerkat/middleware"
	"net/http"
	"os"
	"time"

	apperrors "meerkat/errors"
	"meerkat/logger"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Backups hold the data of every user, so all of these handlers are mounted
// behind AdminMiddleware (which also refuses API tokens). The file name comes
// from the URL and only ever reaches the file system through
// services.BackupPath, which accepts nothing but names the service creates.

// liftWriteDeadline removes the server's write timeout for this request. Writing
// or sending a big backup can take longer than HTTP_WRITE_TIMEOUT. Writers that
// do not support deadlines (tests) are left alone.
func liftWriteDeadline(c *gin.Context) {
	if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		logger.FromContext(c).Warn().Err(err).Msg("Failed to lift the write deadline for a backup")
	}
}

func backupResponse(b services.BackupInfo) models.BackupResponse {
	return models.BackupResponse{Name: b.Name, Size: b.Size, CreatedAt: b.CreatedAt}
}

// ListBackups returns the stored backups, newest first, and the schedule settings.
func ListBackups(c *gin.Context) {
	cfg := currentConfig(c)
	backups, err := services.ListBackups(cfg.BackupDir)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to list backups").WithError(err))
		return
	}
	resp := models.BackupListResponse{
		Backups:        make([]models.BackupResponse, len(backups)),
		Enabled:        cfg.BackupEnabled,
		Time:           cfg.BackupTime,
		RetentionCount: cfg.BackupRetentionCount,
		Directory:      cfg.BackupDir,
	}
	for i, b := range backups {
		resp.Backups[i] = backupResponse(b)
	}
	c.JSON(http.StatusOK, resp)
}

// CreateBackup makes a backup now and prunes old ones down to the retention count.
func CreateBackup(c *gin.Context) {
	cfg := currentConfig(c)
	db := c.MustGet("db").(*gorm.DB)
	log := logger.FromContext(c)
	liftWriteDeadline(c)

	b, err := services.CreateBackup(db, cfg.BackupDir, photoDir(cfg), middleware.RequestNow(c))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create backup")
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to create backup").WithError(err))
		return
	}
	if _, err := services.PruneBackups(cfg.BackupDir, cfg.BackupRetentionCount); err != nil {
		log.Warn().Err(err).Msg("Failed to prune old backups")
	}
	log.Info().Str("backup", b.Name).Int64("bytes", b.Size).Msg("Backup created")
	audit(c, db, models.AuditBackupCreated, "backup:"+auditValue(b.Name))
	c.JSON(http.StatusCreated, backupResponse(b))
}

func backupFile(c *gin.Context) (string, bool) {
	path, err := services.BackupPath(currentConfig(c).BackupDir, c.Param("name"))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Backup"))
		return "", false
	}
	return path, true
}

// DownloadBackup streams one backup archive.
func DownloadBackup(c *gin.Context) {
	path, ok := backupFile(c)
	if !ok {
		return
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Backup"))
		return
	}
	liftWriteDeadline(c)
	audit(c, c.MustGet("db").(*gorm.DB), models.AuditBackupDownloaded, "backup:"+auditValue(c.Param("name")))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(path, c.Param("name"))
}

// DeleteBackup removes one backup.
func DeleteBackup(c *gin.Context) {
	if _, ok := backupFile(c); !ok {
		return
	}
	err := services.DeleteBackup(currentConfig(c).BackupDir, c.Param("name"))
	switch {
	case errors.Is(err, services.ErrBackupNotFound), errors.Is(err, services.ErrInvalidBackupName):
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Backup"))
	case err != nil:
		apperrors.AbortWithError(c, apperrors.ErrInternal("Failed to delete backup").WithError(err))
	default:
		audit(c, c.MustGet("db").(*gorm.DB), models.AuditBackupDeleted, "backup:"+auditValue(c.Param("name")))
		logger.FromContext(c).Info().Str("backup", c.Param("name")).Msg("Backup deleted")
		c.JSON(http.StatusOK, gin.H{"message": "Backup deleted"})
	}
}

func photoDir(cfg config.Config) string {
	if cfg.ProfilePhotoDir != "" {
		return cfg.ProfilePhotoDir
	}
	return os.Getenv("PROFILE_PHOTO_DIR")
}
