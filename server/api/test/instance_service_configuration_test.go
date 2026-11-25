package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/usememos/memos/core/configuration"
	apipb "github.com/usememos/memos/proto/gen/api"
)

// applyConfiguration applies files to the test instance as `memos config apply` would.
func applyConfiguration(ctx context.Context, t *testing.T, ts *TestService, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	desired, err := configuration.Load(dir, configuration.OSResolver())
	require.NoError(t, err)
	plan, _, err := configuration.Apply(ctx, desired, ts.Store, configuration.ApplyMetadata{AppliedBy: "test", Version: "test", Now: time.Unix(1_800_000_000, 0)})
	require.NoError(t, err)
	require.True(t, plan.Valid(), plan.Message.Errors)
}

func requireManaged(t *testing.T, err error, resource string) map[string]string {
	t.Helper()
	st := status.Convert(err)
	require.Equal(t, codes.FailedPrecondition, st.Code(), "got %v", err)
	for _, detail := range st.Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok {
			require.Equal(t, "MANAGED_BY_CONFIGURATION", info.Reason)
			require.Equal(t, "memos.api", info.Domain)
			require.Equal(t, resource, info.Metadata["resource"])
			return info.Metadata
		}
	}
	require.Fail(t, "no ErrorInfo detail", "%v", err)
	return nil
}

func TestConfigurationGuard(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	admin, err := ts.CreateHostUser(ctx, "admin")
	require.NoError(t, err)
	adminCtx := ts.CreateUserContext(ctx, admin.ID)
	applyConfiguration(ctx, t, ts, map[string]string{
		"general.json": `{"instanceSettings":[{"name":"instance/settings/GENERAL","generalSetting":{"customProfile":{"title":"Company"}}}],
			"lifecycle":[{"resource":"instance/settings/GENERAL","ignoreChanges":["generalSetting.weekStartDayOffset"]}]}`,
		"users.json": `{"users":[{"name":"users/admin","username":"admin","role":"ADMIN","password":"pw"}]}`,
	})

	// Instance settings: a managed change is refused, with the file and the field.
	general := func(title string, weekStart int32) error {
		_, err := ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: &apipb.InstanceSetting{
			Name: "instance/settings/GENERAL",
			Value: &apipb.InstanceSetting_GeneralSetting_{GeneralSetting: &apipb.InstanceSetting_GeneralSetting{
				CustomProfile:      &apipb.InstanceSetting_GeneralSetting_CustomProfile{Title: title},
				WeekStartDayOffset: weekStart,
			}},
		}})
		return err
	}
	metadata := requireManaged(t, general("Renamed", 0), "instance/settings/GENERAL")
	require.Equal(t, "general.json", metadata["source"])
	require.Equal(t, "generalSetting.customProfile.title", metadata["fields"])
	require.NoError(t, general("Company", 0), "resending the managed values passes")
	require.NoError(t, general("Company", 1), "an ignored field is the API's to change")

	// Users: username and role are managed; the profile and the password are not.
	update := func(user *apipb.User, paths ...string) error {
		user.Name = "users/admin"
		_, err := ts.Service.UpdateUser(adminCtx, &apipb.UpdateUserRequest{User: user, UpdateMask: &fieldmaskpb.FieldMask{Paths: paths}})
		return err
	}
	require.Equal(t, "role", requireManaged(t, update(&apipb.User{Role: apipb.User_USER}, "role"), "users/admin")["fields"])
	require.NoError(t, update(&apipb.User{DisplayName: "Admin"}, "display_name"))
	require.NoError(t, update(&apipb.User{Password: "changed in the UI"}, "password"), "the password is write-only and never guarded")
	_, err = ts.Service.DeleteUser(adminCtx, &apipb.DeleteUserRequest{Name: "users/admin"})
	requireManaged(t, err, "users/admin")

	// Unmanaged resources are not guarded.
	_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: &apipb.InstanceSetting{
		Name:  "instance/settings/TAGS",
		Value: &apipb.InstanceSetting_TagsSetting_{TagsSetting: &apipb.InstanceSetting_TagsSetting{}},
	}})
	require.NoError(t, err)
}
