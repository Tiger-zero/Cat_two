package configuration

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/version"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db/sqlite"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	instanceProfile := &profile.Profile{Data: t.TempDir(), Driver: "sqlite", Version: version.GetCurrentVersion()}
	require.NoError(t, instanceProfile.Validate())
	driver, err := sqlite.NewDB(instanceProfile)
	require.NoError(t, err)
	s := store.New(driver, instanceProfile)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.Migrate(context.Background()))
	return s
}

func applyFor(t *testing.T, desired *Desired, s *store.Store) (*Plan, *storepb.InstanceConfigurationState) {
	t.Helper()
	plan, state, err := Apply(context.Background(), desired, s, ApplyMetadata{AppliedBy: "test host", Version: "test", Now: time.Unix(1_800_000_000, 0)})
	require.NoError(t, err)
	return plan, state
}

func requirePassword(t *testing.T, s *store.Store, username, password string) {
	t.Helper()
	user, err := s.GetUser(context.Background(), &store.FindUser{Username: &username})
	require.NoError(t, err)
	require.NotNil(t, user)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)))
}

func instanceFiles(title, password string, version string) map[string]string {

	return map[string]string{
		"general.json": generalDocument(title),
		"users.json": `{"users":[{"name":"users/admin","username":"admin","role":"ADMIN","password":"` + password + `"}],
			"lifecycle":[{"resource":"users/admin","writeOnlyVersion":` + version + `}]}`,
	}
}

func TestApplyLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	plan, state := applyFor(t, loadDesired(t, instanceFiles("Company", "first-password", "1")), s)
	require.True(t, plan.Valid())
	require.True(t, plan.HasChanges())
	require.Equal(t, int64(1), state.Serial)
	require.Equal(t, "test host", state.AppliedBy)
	require.Len(t, state.Resources, 2)
	for _, resource := range state.Resources {
		require.Equal(t, int64(1_800_000_000), resource.AppliedTs, resource.Name)
	}

	general, err := s.GetInstanceGeneralSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, "Company", general.GetCustomProfile().GetTitle())
	requirePassword(t, s, "admin", "first-password")

	// Applying the same files again changes nothing.
	again, state := applyFor(t, loadDesired(t, instanceFiles("Company", "first-password", "1")), s)
	require.False(t, again.HasChanges())
	require.Equal(t, int64(1), state.Serial)

	// An edit outside the configuration is reverted as drift.
	_, err = s.UpsertInstanceSetting(ctx, generalSetting("Edited in the UI"))
	require.NoError(t, err)
	repaired, state := applyFor(t, loadDesired(t, instanceFiles("Company", "first-password", "1")), s)
	require.True(t, changeFor(t, repaired, "instance/settings/GENERAL").Drift)
	require.Equal(t, int64(2), state.Serial)
	general, err = s.GetInstanceGeneralSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, "Company", general.GetCustomProfile().GetTitle())

	// A new password is sent only when writeOnlyVersion goes up.
	_, _ = applyFor(t, loadDesired(t, instanceFiles("Company", "second-password", "1")), s)
	requirePassword(t, s, "admin", "first-password")
	_, state = applyFor(t, loadDesired(t, instanceFiles("Company", "second-password", "2")), s)
	requirePassword(t, s, "admin", "second-password")
	for _, resource := range state.Resources {
		if resource.Name == "users/admin" {
			require.Equal(t, int32(2), resource.WriteOnlyVersion)
		}
	}
}

func TestApplyInvalidPlanWritesNothing(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	desired := loadDesired(t, map[string]string{
		"general.json": generalDocument("Should not be written"),
		"admin.json":   `{"users":[{"name":"users/admin","username":"admin","role":"ADMIN"}]}`,
	})
	plan, state := applyFor(t, desired, s)
	require.False(t, plan.Valid())
	require.Nil(t, state)
	general, err := s.GetInstanceGeneralSetting(ctx)
	require.NoError(t, err)
	require.NotEqual(t, "Should not be written", general.GetCustomProfile().GetTitle())
	record, err := s.GetConfigurationState(ctx)
	require.NoError(t, err)
	require.Zero(t, record.Serial)
}

func TestApplyKeepsIgnoredFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, err := s.UpsertInstanceSetting(ctx, generalSetting("Chosen in the UI"))
	require.NoError(t, err)

	desired := loadDesired(t, map[string]string{"general.json": `{"instanceSettings":[{"name":"instance/settings/GENERAL","generalSetting":{
		"weekStartDayOffset":1,"customProfile":{"title":"From the file"}}}],
		"lifecycle":[{"resource":"instance/settings/GENERAL","ignoreChanges":["generalSetting.customProfile"]}]}`})
	_, state := applyFor(t, desired, s)
	require.Equal(t, int64(1), state.Serial)
	general, err := s.GetInstanceGeneralSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), general.WeekStartDayOffset, "managed fields are written")
	require.Equal(t, "Chosen in the UI", general.GetCustomProfile().GetTitle(), "ignored fields keep their stored value")
}

func TestApplyRemovals(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, state := applyFor(t, loadDesired(t, map[string]string{"general.json": generalDocument("Company")}), s)
	require.Len(t, state.Resources, 1)

	_, state = applyFor(t, loadDesired(t, map[string]string{"removed.json": `{"removed":[
		{"resource":"instance/settings/GENERAL","destroy":true}]}`}), s)
	require.Empty(t, state.Resources)
	general, err := s.GetUncachedInstanceSetting(ctx, &store.FindInstanceSetting{Name: "GENERAL"})
	require.NoError(t, err)
	require.Nil(t, general, "an explicitly removed active setting is reset")
}

func TestApplyLegacyConfigurationPreservesData(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	admin, err := s.CreateUser(ctx, &store.User{Username: "admin", Role: store.RoleAdmin, PasswordHash: "old-hash"})
	require.NoError(t, err)
	legacy, err := s.CreateUser(ctx, &store.User{Username: "legacy", Role: store.RoleUser, PasswordHash: "legacy-hash"})
	require.NoError(t, err)
	legacyMemo, err := s.CreateMemo(ctx, &store.Memo{UID: "legacy-note", CreatorID: legacy.ID, Content: "old private note", Visibility: store.Private})
	require.NoError(t, err)
	db, err := sql.Open("sqlite", filepath.Join(s.GetDataDir(), "memos_prod.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	const providerConfig = `{"clientId":"old-client","clientSecret":"old-secret"}`
	_, err = db.ExecContext(ctx, `INSERT INTO idp (uid, name, type, config) VALUES ('sso', 'Legacy SSO', 'OAUTH2', ?)`, providerConfig)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO user_identity (user_id, provider, extern_uid) VALUES (?, 'sso', 'legacy-subject')`, legacy.ID)
	require.NoError(t, err)
	legacySetting := &storepb.InstanceSetting{Key: storepb.InstanceSettingKey_AI,
		Value: &storepb.InstanceSetting_AiSetting{AiSetting: &storepb.InstanceAISetting{Providers: []*storepb.AIProviderConfig{{Id: "old-ai", ApiKey: "old-key"}}}}}
	_, err = s.UpsertInstanceSetting(ctx, legacySetting)
	require.NoError(t, err)
	_, err = s.RunConfigurationTransaction(ctx, func(context.Context, *store.ConfigurationTransaction) (*storepb.InstanceConfigurationState, error) {
		return &storepb.InstanceConfigurationState{Resources: []*storepb.InstanceConfigurationState_ManagedResource{
			{Name: "identity-providers/sso", Kind: "IdentityProvider", AppliedTs: 123},
			{Name: "users/legacy/linkedIdentities/sso", Kind: "LinkedIdentity", AppliedTs: 123},
			{Name: "users/legacy", Kind: "User", AppliedTs: 123},
			{Name: "instance/settings/AI", Kind: "InstanceSetting", AppliedTs: 123},
		}}, nil
	})
	require.NoError(t, err)

	files := map[string]string{"legacy.json": `{
		"identityProviders":[{"name":"identity-providers/sso","config":"${env:RETIRED_SECRET}"}],
		"linkedIdentities":[{"name":"users/legacy/linkedIdentities/sso","externalUid":"${file:/missing/legacy}"}],
		"instanceSettings":[{"name":"instance/settings/AI","aiSetting":{"providers":[{"apiKey":"${env:RETIRED_AI_KEY}"}]}}],
		"users":[{"name":"users/admin","username":"admin","role":"ADMIN","password":"new-password"},
			{"name":"users/legacy","username":"legacy","role":"USER","password":"${env:RETIRED_USER_PASSWORD}"},
			{"name":"users/extra","username":"extra","role":"ADMIN","password":"ignored"}],
		"lifecycle":[{"resource":"users/admin","writeOnlyVersion":1},{"resource":"users/legacy"}],
		"removed":[{"resource":"users/legacy","destroy":true},{"resource":"instance/settings/AI","destroy":true},
			{"resource":"identity-providers/sso","destroy":true},{"resource":"users/legacy/linkedIdentities/sso","destroy":true}]
	}`}
	desired, err := Load(writeFiles(t, files), Resolver{})
	require.NoError(t, err, "retired declarations must not resolve their secrets")
	plan, state := applyFor(t, desired, s)
	require.True(t, plan.Valid(), plan.Message.Errors)
	require.Len(t, plan.Message.ResourceChanges, 1)
	require.Len(t, state.Resources, 5)
	users, err := s.ListUsers(ctx, &store.FindUser{})
	require.NoError(t, err)
	require.Len(t, users, 2)
	requirePassword(t, s, admin.Username, "new-password")
	unchangedUser, err := s.GetUser(ctx, &store.FindUser{ID: &legacy.ID})
	require.NoError(t, err)
	require.Equal(t, legacy.PasswordHash, unchangedUser.PasswordHash)
	unchangedMemo, err := s.GetMemo(ctx, &store.FindMemo{ID: &legacyMemo.ID})
	require.NoError(t, err)
	require.Equal(t, legacyMemo.Content, unchangedMemo.Content)
	var unchangedConfig, unchangedSubject string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT config FROM idp WHERE uid = 'sso'`).Scan(&unchangedConfig))
	require.Equal(t, providerConfig, unchangedConfig)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT extern_uid FROM user_identity WHERE user_id = ? AND provider = 'sso'`, legacy.ID).Scan(&unchangedSubject))
	require.Equal(t, "legacy-subject", unchangedSubject)
	unchangedSetting, err := s.GetUncachedInstanceSetting(ctx, &store.FindInstanceSetting{Name: "AI"})
	require.NoError(t, err)
	require.Equal(t, "old-key", unchangedSetting.GetAiSetting().Providers[0].ApiKey)
	again, _ := applyFor(t, desired, s)
	require.False(t, again.HasChanges())
}

func TestApplyFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	desired := loadDesired(t, instanceFiles("Should roll back", strings.Repeat("a", 73), "1"))
	_, _, err := Apply(ctx, desired, s, ApplyMetadata{Now: time.Now()})
	require.ErrorIs(t, err, bcrypt.ErrPasswordTooLong)
	general, err := s.GetUncachedInstanceSetting(ctx, &store.FindInstanceSetting{Name: "GENERAL"})
	require.NoError(t, err)
	require.Nil(t, general, "the setting written before the failure rolled back")
	record, err := s.GetConfigurationState(ctx)
	require.NoError(t, err)
	require.Zero(t, record.Serial)
}
