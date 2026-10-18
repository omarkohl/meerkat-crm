package routes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"meerkat/config"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proof names a Go test, in a file relative to backend/, that shows the behaviour. The tests
// below check that the file holds a function of that name.
type proof struct{ file, test string }

// publicRoutes are the mutating routes that answer without a login. Every entry needs a reason.
var publicRoutes = map[string]string{
	"POST /api/v1/check-password-strength": "scores a password typed into the registration form",
	"POST /api/v1/login":                   "starts a session",
	"POST /api/v1/logout":                  "ends the session of the cookie it is given, if any",
	"POST /api/v1/password-reset/request":  "asks for a reset e-mail without a login",
	"POST /api/v1/password-reset/confirm":  "the reset token is the credential",
	"POST /api/v1/register":                "creates an account",
}

// isolation maps every mutating route that needs a login to the test that shows another user
// (or a non-admin, for the admin routes) cannot use it on someone else's data.
var isolation = map[string]proof{
	"POST /api/v1/activities":                        proof{"activities/activities_test.go", "TestCreateWithoutContactsAndWithUnknownOnes"},
	"PUT /api/v1/activities/:id":                     proof{"activities/activities_test.go", "TestUpdateRefusesOtherUsersActivitiesAndContactsAndBadInput"},
	"DELETE /api/v1/activities/:id":                  proof{"activities/activities_test.go", "TestDeleteRemovesTheActivityAndAnnouncesIt"},
	"POST /api/v1/notes":                             proof{"notes/notes_test.go", "TestCreateForAnotherUsersContactIsNotFound"},
	"POST /api/v1/contacts/:id/notes":                proof{"notes/notes_test.go", "TestCreateForAnotherUsersContactIsNotFound"},
	"PUT /api/v1/notes/:id":                          proof{"notes/notes_test.go", "TestUpdateRefusesOtherUsersNotesAndBadInput"},
	"DELETE /api/v1/notes/:id":                       proof{"notes/notes_test.go", "TestDeleteSoftDeletesAndAnnounces"},
	"POST /api/v1/contacts/:id/reminders":            proof{"reminders/reminders_test.go", "TestCreateRefusesOtherUsersContactsAndBadInput"},
	"PUT /api/v1/reminders/:id":                      proof{"reminders/reminders_test.go", "TestUpdateRefusesOtherUsersRemindersContactsAndBadInput"},
	"POST /api/v1/reminders/:id/complete":            proof{"reminders/reminders_test.go", "TestCompleteRefusesOtherUsers"},
	"DELETE /api/v1/reminders/:id":                   proof{"reminders/reminders_test.go", "TestDeleteRemovesOwnReminder"},
	"DELETE /api/v1/reminder-completions/:id":        proof{"reminders/reminders_test.go", "TestCompletionsAreListedNewestFirstAndDeletedByTheirOwner"},
	"POST /api/v1/contacts/:id/relationships":        proof{"relationships/relationships_test.go", "TestCreateRefusesOtherUsersContactsRelatedContactsAndBadInput"},
	"PUT /api/v1/contacts/:id/relationships/:rid":    proof{"relationships/relationships_test.go", "TestUpdateRefusesOtherUsersRelationshipsRelatedContactsAndBadInput"},
	"DELETE /api/v1/contacts/:id/relationships/:rid": proof{"relationships/relationships_test.go", "TestDeleteRemovesOwnRelationshipOnly"},
	"POST /api/v1/contacts":                          proof{"contacts/contacts_test.go", "TestGetUpdateAndCreateRefuseOtherUsersAndAnonymous"},
	"PUT /api/v1/contacts/:id":                       proof{"contacts/contacts_test.go", "TestGetUpdateAndCreateRefuseOtherUsersAndAnonymous"},
	"DELETE /api/v1/contacts/:id":                    proof{"contacts/contacts_test.go", "TestOtherUsersContactsAreNotFound"},
	"POST /api/v1/contacts/:id/archive":              proof{"contacts/contacts_test.go", "TestOtherUsersContactsAreNotFound"},
	"POST /api/v1/contacts/:id/unarchive":            proof{"contacts/contacts_test.go", "TestOtherUsersContactsAreNotFound"},
	"POST /api/v1/contacts/:id/profile_picture":      proof{"photos/photos_test.go", "TestUploadRefusesWhatTheUserCanFixAndWritesNothing"},
	"POST /api/v1/trash/contacts/:id/restore":        proof{"trash/trash_test.go", "TestPurgeAndRestoreRefuseLiveForeignAndMalformedContacts"},
	"DELETE /api/v1/trash/contacts/:id":              proof{"trash/trash_test.go", "TestPurgeAndRestoreRefuseLiveForeignAndMalformedContacts"},
	"DELETE /api/v1/trash/contacts":                  proof{"trash/trash_test.go", "TestEmptyingTheTrashPurgesOnlyTheActorsDeletedContacts"},
	"POST /api/v1/api-tokens":                        proof{"tokens/tokens_test.go", "TestListAPITokensIsScopedNewestFirstAndIncludesRevoked"},
	"DELETE /api/v1/api-tokens/:id":                  proof{"tokens/tokens_test.go", "TestRevokeAPITokenOfAnotherUserIs404AndChangesNothing"},
	"POST /api/v1/ics-feed":                          proof{"tokens/tokens_test.go", "TestFeedsAreIsolatedBetweenUsers"},
	"DELETE /api/v1/ics-feed":                        proof{"tokens/tokens_test.go", "TestFeedsAreIsolatedBetweenUsers"},
	"POST /api/v1/webhooks":                          proof{"webhooks/webhooks_test.go", "TestListWebhooksIsScopedNewestFirstAndAnArray"},
	"PUT /api/v1/webhooks/:id":                       proof{"webhooks/webhooks_test.go", "TestAnotherUsersWebhookIs404ForEveryOperationAndStaysUnchanged"},
	"DELETE /api/v1/webhooks/:id":                    proof{"webhooks/webhooks_test.go", "TestAnotherUsersWebhookIs404ForEveryOperationAndStaysUnchanged"},
	"POST /api/v1/webhooks/:id/test":                 proof{"webhooks/webhooks_test.go", "TestAnotherUsersWebhookIs404ForEveryOperationAndStaysUnchanged"},
	"POST /api/v1/calendars":                         proof{"calendars/calendars_test.go", "TestListIsScopedOldestFirstAndAnArray"},
	"PUT /api/v1/calendars/:id":                      proof{"calendars/calendars_test.go", "TestAnotherUsersSubscriptionIs404ForEveryOperation"},
	"DELETE /api/v1/calendars/:id":                   proof{"calendars/calendars_test.go", "TestAnotherUsersSubscriptionIs404ForEveryOperation"},
	"POST /api/v1/calendars/:id/sync":                proof{"calendars/calendars_test.go", "TestAnotherUsersSubscriptionIs404ForEveryOperation"},
	"PUT /api/v1/carddav/connection":                 proof{"contactsync/contactsync_test.go", "TestNoConnectionIs404UntilOneIsSaved"},
	"POST /api/v1/carddav/connection/sync":           {"contactsync/contactsync_test.go", "TestAnotherUsersConnectionCannotBeSyncedAndKeepsItsLinks"},
	"DELETE /api/v1/carddav/connection":              proof{"contactsync/contactsync_test.go", "TestDeleteRemovesTheConnectionAndLinksButNotContacts"},
	"POST /api/v1/carddav/discover":                  proof{"contactsync/contactsync_test.go", "TestDiscoverReusesTheStoredPasswordOnlyForTheStoredServer"},
	"POST /api/v1/contacts/import/upload":            proof{"imports/imports_test.go", "TestImportSessionsBelongToTheirUser"},
	"POST /api/v1/contacts/import/preview":           proof{"imports/imports_test.go", "TestImportSessionsBelongToTheirUser"},
	"POST /api/v1/contacts/import/confirm":           proof{"imports/imports_test.go", "TestImportSessionsBelongToTheirUser"},
	"POST /api/v1/contacts/import/vcf/upload":        {"imports/imports_test.go", "TestAVCFImportSessionBelongsToItsUploader"},
	"POST /api/v1/contacts/import/vcf/confirm":       {"imports/imports_test.go", "TestAVCFImportSessionBelongsToItsUploader"},
	"POST /api/v1/contacts/import/monica/connect":    proof{"monicaimport/monicaimport_test.go", "TestSessionsBelongToTheirUser"},
	"POST /api/v1/contacts/import/monica/fetch":      proof{"monicaimport/monicaimport_test.go", "TestSessionsBelongToTheirUser"},
	"POST /api/v1/contacts/import/monica/confirm":    proof{"monicaimport/monicaimport_test.go", "TestSessionsBelongToTheirUser"},
	"DELETE /api/v1/contacts/import/monica/session":  proof{"monicaimport/monicaimport_test.go", "TestSessionsBelongToTheirUser"},
	"PATCH /api/v1/users/language":                   proof{"users/users_test.go", "TestLanguageIsChangedForTheActorOnly"},
	"PATCH /api/v1/users/date-format":                proof{"users/users_test.go", "TestDateFormatIsChangedForTheActorOnly"},
	"PATCH /api/v1/users/custom-fields":              proof{"users/users_test.go", "TestCustomFieldNamesDefaultToAnEmptyListAndAreKeptPerUser"},
	"PATCH /api/v1/users/enabled-contact-fields":     proof{"users/users_test.go", "TestEnabledContactFieldsKeepNullApartFromEmpty"},
	"DELETE /api/v1/sessions/:id":                    proof{"users/sessions_test.go", "TestOnlyALiveSessionOfTheActorCanBeRevoked"},
	"POST /api/v1/sessions/revoke-all":               proof{"controllers/session_controller_test.go", "TestSessionsAreIsolatedBetweenUsers"},
	"POST /api/v1/users/change-password":             {"users/sessions_test.go", "TestChangingOnePasswordLeavesTheOtherUsersPasswordAndSessionsAlone"},
	"POST /api/v1/users/me/2fa/setup":                {"users/totp_test.go", "TestOneUsersTwoFactorCannotBeSeenOrChangedByAnotherUser"},
	"POST /api/v1/users/me/2fa/enable":               {"users/totp_test.go", "TestOneUsersTwoFactorCannotBeSeenOrChangedByAnotherUser"},
	"POST /api/v1/users/me/2fa/disable":              {"users/totp_test.go", "TestOneUsersTwoFactorCannotBeSeenOrChangedByAnotherUser"},
	"POST /api/v1/users/me/2fa/recovery-codes":       {"users/totp_test.go", "TestOneUsersTwoFactorCannotBeSeenOrChangedByAnotherUser"},
	"DELETE /api/v1/admin/backups/:name":             proof{"admin/admin_test.go", "TestEveryRouteRefusesAPlainUserAndTouchesNothing"},
	"POST /api/v1/admin/backups":                     proof{"admin/admin_test.go", "TestEveryRouteRefusesAPlainUserAndTouchesNothing"},
	"PATCH /api/v1/admin/users/:id":                  proof{"admin/users_test.go", "TestUserEndpointsRefuseAPlainUser"},
	"DELETE /api/v1/admin/users/:id":                 proof{"admin/users_test.go", "TestUserEndpointsRefuseAPlainUser"},
	"POST /api/v1/admin/users/:id/2fa/reset":         proof{"admin/users_test.go", "TestUserEndpointsRefuseAPlainUser"},
	"POST /api/v1/admin/trigger-reminders":           proof{"admin/users_test.go", "TestUserEndpointsRefuseAPlainUser"},
	"COPY /carddav/*path":                            {"carddav/backend_test.go", "TestOtherCardDAVMethodsCannotReachAnotherUsersContactEither"},
	"DELETE /carddav/*path":                          {"carddav/backend_test.go", "TestACardDAVUserCannotReadOrDeleteAnotherUsersContact"},
	"MKCOL /carddav/*path":                           {"carddav/backend_test.go", "TestOtherCardDAVMethodsCannotReachAnotherUsersContactEither"},
	"MOVE /carddav/*path":                            {"carddav/backend_test.go", "TestOtherCardDAVMethodsCannotReachAnotherUsersContactEither"},
	"PATCH /carddav/*path":                           {"carddav/backend_test.go", "TestOtherCardDAVMethodsCannotReachAnotherUsersContactEither"},
	"PUT /carddav/*path":                             {"carddav/backend_test.go", "TestACardDAVPutWithAnotherUsersUIDCreatesTheCallersOwnContact"},
	"POST /carddav/*path":                            {"carddav/backend_test.go", "TestOtherCardDAVMethodsCannotReachAnotherUsersContactEither"},
}

var mutating = map[string]bool{"POST": true, "PUT": true, "PATCH": true, "DELETE": true, "PROPPATCH": true, "MKCOL": true, "COPY": true, "MOVE": true}

var routeParam = regexp.MustCompile(`:[A-Za-z_]+|\*[A-Za-z_]+`)

func engine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", CardDAVEnabled: true, MetricsEnabled: true}
	cfg.OIDC.Enabled = true
	r := gin.New()
	RegisterRoutes(r, cfg, nil, NewModules(nil, cfg, &services.OIDCProvider{}))
	return r
}

func mutatingRoutes(r *gin.Engine) []string {
	var out []string
	for _, route := range r.Routes() {
		if mutating[route.Method] {
			out = append(out, route.Method+" "+route.Path)
		}
	}
	sort.Strings(out)
	return out
}

func TestEveryMutatingRouteNeedsALoginUnlessItIsListedAsPublic(t *testing.T) {
	r := engine(t)
	var open []string
	for _, key := range mutatingRoutes(r) {
		if _, ok := publicRoutes[key]; ok {
			continue
		}
		method, path, _ := strings.Cut(key, " ")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, routeParam.ReplaceAllString(path, "1"), nil))
		if w.Code != http.StatusUnauthorized {
			open = append(open, key+" answered "+http.StatusText(w.Code))
		}
	}
	assert.Empty(t, open, "these routes answer an anonymous request: protect them, or list them in publicRoutes with the reason")
}

func TestEveryMutatingRouteHasIsolationCoverage(t *testing.T) {
	r := engine(t)
	routes := map[string]bool{}
	var uncovered []string
	for _, key := range mutatingRoutes(r) {
		routes[key] = true
		_, public := publicRoutes[key]
		if _, ok := isolation[key]; !ok && !public {
			uncovered = append(uncovered, key)
		}
	}
	assert.Empty(t, uncovered, "add these routes to isolation (with the test that shows another user cannot use them) or to publicRoutes")
	for key := range isolation {
		assert.True(t, routes[key], "isolation lists a route that does not exist: %s", key)
	}
	for key := range publicRoutes {
		assert.True(t, routes[key], "publicRoutes lists a route that does not exist: %s", key)
	}
	for key, p := range isolation {
		requireTest(t, key, p)
	}
}

func requireTest(t *testing.T, what string, p proof) {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", p.file), nil, 0)
	if !assert.NoError(t, err, "%s: %s", what, p.file) {
		return
	}
	for _, decl := range parsed.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == p.test && strings.HasPrefix(p.test, "Test") {
			return
		}
	}
	t.Errorf("%s: %s has no top-level test function %s", what, p.file, p.test)
}

// covered maps every model that has a UserID to the test that shows one user cannot read or
// change another's rows. A proof in frontend/e2e/isolation.spec.ts has an empty file and the
// test title in test.
// noAPI lists the models with a UserID that no endpoint reads or writes, so there is no
// cross-user access to test. Every entry needs a reason.
var noAPI = map[string]string{
	"EmailOutbox": "a queue only the mail engine reads and sends; users see no rows of it",
}

var covered = map[string]proof{
	"Activity":             {"activities/activities_test.go", "TestUpdateRefusesOtherUsersActivitiesAndContactsAndBadInput"},
	"ApiToken":             {"tokens/tokens_test.go", "TestRevokeAPITokenOfAnotherUserIs404AndChangesNothing"},
	"AuditLog":             {"admin/admin_test.go", "TestEveryRouteRefusesAPlainUserAndTouchesNothing"},
	"CalendarEventLink":    {"calendars/calendars_test.go", "TestDeletingOneUsersSubscriptionKeepsTheOtherUsersEventLinks"},
	"CalendarSubscription": {"calendars/calendars_test.go", "TestAnotherUsersSubscriptionIs404ForEveryOperation"},
	"CardDAVConnection":    {"contactsync/contactsync_test.go", "TestNoConnectionIs404UntilOneIsSaved"},
	"CardDAVContactLink":   {"contactsync/contactsync_test.go", "TestAnotherUsersConnectionCannotBeSyncedAndKeepsItsLinks"},
	"Contact":              {"", "a user cannot see another user's contacts"},
	"IcsFeed":              {"", "a calendar feed only contains its owner's birthdays"},
	"ImportSession":        {"imports/imports_test.go", "TestImportSessionsBelongToTheirUser"},
	"Note":                 {"notes/notes_test.go", "TestUpdateRefusesOtherUsersNotesAndBadInput"},
	"RecoveryCode":         {"users/totp_test.go", "TestOneUsersTwoFactorCannotBeSeenOrChangedByAnotherUser"},
	"Relationship":         {"relationships/relationships_test.go", "TestUpdateRefusesOtherUsersRelationshipsRelatedContactsAndBadInput"},
	"Reminder":             {"reminders/reminders_test.go", "TestUpdateRefusesOtherUsersRemindersContactsAndBadInput"},
	"ReminderCompletion":   {"reminders/reminders_test.go", "TestCompletionsAreListedNewestFirstAndDeletedByTheirOwner"},
	"Session":              {"users/sessions_test.go", "TestOnlyALiveSessionOfTheActorCanBeRevoked"},
	"Webhook":              {"webhooks/webhooks_test.go", "TestAnotherUsersWebhookIs404ForEveryOperationAndStaysUnchanged"},
}

func modelsWithUserID(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "models", "*.go"))
	require.NoError(t, err)
	var out []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		require.NoError(t, err)
		ast.Inspect(parsed, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if st, ok := spec.Type.(*ast.StructType); ok {
				for _, f := range st.Fields.List {
					for _, name := range f.Names {
						if name.Name == "UserID" {
							out = append(out, spec.Name.Name)
						}
					}
				}
			}
			return true
		})
	}
	sort.Strings(out)
	return out
}

func TestEveryModelWithAUserIDHasIsolationCoverage(t *testing.T) {
	models := modelsWithUserID(t)
	require.NotEmpty(t, models)
	var missing []string
	have := map[string]bool{}
	for _, m := range models {
		have[m] = true
		_, isCovered := covered[m]
		_, exempt := noAPI[m]
		if !isCovered && !exempt {
			missing = append(missing, m)
		}
		assert.False(t, isCovered && exempt, "%s is both covered and exempt", m)
	}
	assert.Empty(t, missing, "these models have a UserID: add them to covered with the test that shows another user cannot reach their rows")
	for m := range noAPI {
		assert.True(t, have[m], "noAPI lists a model without a UserID: %s", m)
	}
	for m, p := range covered {
		assert.True(t, have[m], "covered lists a model without a UserID: %s", m)
		if p.file == "" {
			spec, err := os.ReadFile(filepath.Join("..", "..", "frontend", "e2e", "isolation.spec.ts"))
			require.NoError(t, err)
			assert.Regexp(t, `(?m)^\s*test\(\s*["'`+"`"+`]`+regexp.QuoteMeta(p.test)+`(?: @[\w-]+)*["'`+"`"+`]`, string(spec), "%s: isolation.spec.ts has no running test %q", m, p.test)
			continue
		}
		requireTest(t, m, p)
	}
}

// readIsolation maps the read routes that search across a user's rows to the test that shows
// another user's rows are never found. (Routes that take an ID are covered by their module's
// tests; see the audit in docs/development/testing.md.)
var readIsolation = map[string]proof{
	"GET /api/v1/search": {"globalsearch/globalsearch_test.go", "TestOneUserNeverFindsAnotherUsersData"},
}

func TestEverySearchRouteHasIsolationCoverage(t *testing.T) {
	r := engine(t)
	routes := map[string]bool{}
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for key, p := range readIsolation {
		assert.True(t, routes[key], "readIsolation lists a route that does not exist: %s", key)
		requireTest(t, key, p)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/search?q=x", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code, "the search needs a login")
}
