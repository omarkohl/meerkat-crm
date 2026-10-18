package app_test

import (
	"context"
	"testing"
	"time"

	"meerkat/admin/internal/app"
	"meerkat/admin/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeUsers struct {
	app.Users
	users  map[domain.UserID]domain.User
	admins int64
	saved  bool
	gone   bool
}

func (f *fakeUsers) Get(_ context.Context, id domain.UserID) (domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}
func (f *fakeUsers) CountAdmins(context.Context) (int64, error) { return f.admins, nil }
func (f *fakeUsers) Save(_ context.Context, u domain.User, _ string, _ time.Time) (domain.User, error) {
	f.saved = true
	return u, nil
}
func (f *fakeUsers) Delete(context.Context, domain.User) error { f.gone = true; return nil }

type noAudit struct{ app.Audit }

func (noAudit) RecordUser(context.Context, domain.UserID, domain.UserAction, domain.User, []string, domain.Client) {
}

func TestTheLastAdminIsNeitherDemotedNorDeleted(t *testing.T) {
	users := &fakeUsers{users: map[domain.UserID]domain.User{2: {ID: 2, IsAdmin: true}}, admins: 1}
	uc := app.New(nil, nil, noAudit{}, users, nil, nil, domain.SystemSettings{})
	no := false

	_, err := uc.UpdateUser(t.Context(), 1, 2, domain.UserChanges{IsAdmin: &no}, time.Time{}, domain.Client{})
	require.ErrorIs(t, err, domain.ErrLastAdmin)
	assert.False(t, users.saved)

	require.ErrorIs(t, uc.DeleteUser(t.Context(), 1, 2, domain.Client{}), domain.ErrLastAdmin)
	assert.False(t, users.gone)

	users.admins = 2
	_, err = uc.UpdateUser(t.Context(), 1, 2, domain.UserChanges{IsAdmin: &no}, time.Time{}, domain.Client{})
	require.NoError(t, err)
	require.NoError(t, uc.DeleteUser(t.Context(), 1, 2, domain.Client{}))
}
