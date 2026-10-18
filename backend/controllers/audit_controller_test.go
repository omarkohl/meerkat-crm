package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"meerkat/middleware"
	"meerkat/models"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func auditEnv(t *testing.T) (*backupEnv, func(uid uint) *gin.Engine) {
	t.Helper()
	e := newBackupEnv(t)
	require.NoError(t, e.db.AutoMigrate(&models.AuditLog{}, &models.Session{}, &models.ApiToken{}, &models.IcsFeed{}))
	mk := func(uid uint) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("db", e.db); c.Set("userID", uid); c.Next() })
		r.GET("/admin/audit-log", middleware.AdminMiddleware(), ListAuditLog)
		return r
	}
	return e, mk
}

type auditPage struct {
	Entries []models.AuditLog `json:"entries"`
	Total   int               `json:"total"`
	Events  []string          `json:"events"`
}

func TestAuditLogIsAdminOnly(t *testing.T) {
	e, mk := auditEnv(t)
	var user models.User
	require.NoError(t, e.db.Where("is_admin = ?", false).First(&user).Error)
	services.WriteAudit(e.db, models.AuditLog{Event: models.AuditLogin, Username: "secret-name"})
	w := e.do(mk(user.ID), "GET", "/admin/audit-log")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NotContains(t, w.Body.String(), "secret-name")
}

func TestAuditLogListFiltersAndPagesNewestFirst(t *testing.T) {
	e, mk := auditEnv(t)
	var admin models.User
	require.NoError(t, e.db.Where("is_admin = ?", true).First(&admin).Error)
	for i := 0; i < 5; i++ {
		services.WriteAudit(e.db, models.AuditLog{Event: models.AuditLogin, Username: "Alice", Detail: fmt.Sprint(i), CreatedAt: time.Now()})
	}
	services.WriteAudit(e.db, models.AuditLog{Event: models.AuditLoginFailed, Username: "bob"})
	services.WriteAudit(e.db, models.AuditLog{Event: models.AuditExport, Username: "alice_b"})

	get := func(q string) auditPage {
		w := e.do(mk(admin.ID), "GET", "/admin/audit-log"+q)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var p auditPage
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
		return p
	}
	all := get("")
	assert.Equal(t, 7, all.Total)
	assert.Equal(t, models.AuditExport, all.Entries[0].Event, "newest first")
	assert.Equal(t, models.AuditEvents, all.Events)

	assert.Equal(t, 1, get("?event=login_failed").Total)
	assert.Equal(t, 6, get("?event=login").Total+get("?event=export").Total, "exact event match")
	assert.Equal(t, 6, get("?username=ALI").Total, "case-insensitive substring")
	assert.Equal(t, 5, get("?event=login&username=alice").Total)
	assert.Equal(t, 0, get("?event=login&username=bob").Total)
	assert.Equal(t, 0, get("?username=%25").Total, "wildcards are literal")

	page := get("?limit=3&page=2")
	assert.Len(t, page.Entries, 3)
	assert.Equal(t, 7, page.Total)
}

func TestPurgedAuditEntriesKeepOtherYears(t *testing.T) {
	e, _ := auditEnv(t)
	old := time.Now().AddDate(-2, 0, 0)
	services.WriteAudit(e.db, models.AuditLog{Event: models.AuditLogin, CreatedAt: old})
	services.WriteAudit(e.db, models.AuditLog{Event: models.AuditLogin})
	n, err := services.PurgeAuditLog(e.db, 365, time.Now())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}
