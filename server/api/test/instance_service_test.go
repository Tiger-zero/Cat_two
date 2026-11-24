package test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	colorpb "google.golang.org/genproto/googleapis/type/color"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func TestGetInstanceProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("GetInstanceProfile returns instance profile", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Call GetInstanceProfile directly
		req := &apipb.GetInstanceProfileRequest{}
		resp, err := ts.Service.GetInstanceProfile(ctx, req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify the response contains expected data
		require.Equal(t, "test-1.0.0", resp.Version)
		require.Equal(t, "test-commit", resp.Commit)
		require.True(t, resp.Demo)
		require.Equal(t, "http://localhost:8080", resp.InstanceUrl)
		require.Equal(t, apipb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC, resp.AccessMode)

		// Instance should not be initialized since no users exist at all.
		require.Nil(t, resp.Admin)
		require.True(t, resp.NeedsSetup)
	})

	t.Run("GetInstanceProfile with initialized instance", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a host user in the store
		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		require.NotNil(t, hostUser)

		// Call GetInstanceProfile as the signed-in admin
		req := &apipb.GetInstanceProfileRequest{}
		resp, err := ts.Service.GetInstanceProfile(ts.CreateUserContext(ctx, hostUser.ID), req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify the response contains expected data with initialized flag
		require.Equal(t, "test-1.0.0", resp.Version)
		require.Equal(t, "test-commit", resp.Commit)
		require.True(t, resp.Demo)
		require.Equal(t, "http://localhost:8080", resp.InstanceUrl)

		// Instance should be initialized since an admin user exists
		require.NotNil(t, resp.Admin)
		require.Equal(t, hostUser.Username, resp.Admin.Username)
		require.False(t, resp.NeedsSetup)
	})

	t.Run("GetInstanceProfile hides the admin from anonymous callers", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)

		// The endpoint is public so the sign-in page can render, but an
		// unauthenticated visitor must not learn the admin's username.
		resp, err := ts.Service.GetInstanceProfile(ctx, &apipb.GetInstanceProfileRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Nil(t, resp.Admin)
		require.False(t, resp.NeedsSetup)

		// Any signed-in user, not only the admin, may see it.
		regularUser, err := ts.CreateRegularUser(ctx, "alice")
		require.NoError(t, err)
		resp, err = ts.Service.GetInstanceProfile(ts.CreateUserContext(ctx, regularUser.ID), &apipb.GetInstanceProfileRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp.Admin)
		require.Equal(t, hostUser.Username, resp.Admin.Username)
	})

	t.Run("GetInstanceProfile with users but no admin", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a regular user but no admin. This mirrors an instance that has
		// lost all of its admins: admin is nil, but the instance is NOT a fresh
		// install and must not be flagged for first-run setup.
		regularUser, err := ts.CreateRegularUser(ctx, "alice")
		require.NoError(t, err)
		require.NotNil(t, regularUser)

		req := &apipb.GetInstanceProfileRequest{}
		resp, err := ts.Service.GetInstanceProfile(ts.CreateUserContext(ctx, regularUser.ID), req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// No admin to display, but setup is already done because a user exists.
		require.Nil(t, resp.Admin)
		require.False(t, resp.NeedsSetup)
	})
}

func TestGetInstanceProfile_Concurrency(t *testing.T) {
	ctx := context.Background()

	t.Run("Concurrent access to service", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a host user
		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, hostUser.ID)

		// Make concurrent requests
		numGoroutines := 10
		results := make(chan *apipb.InstanceProfile, numGoroutines)
		errors := make(chan error, numGoroutines)

		for range numGoroutines {
			go func() {
				req := &apipb.GetInstanceProfileRequest{}
				resp, err := ts.Service.GetInstanceProfile(userCtx, req)
				if err != nil {
					errors <- err
					return
				}
				results <- resp
			}()
		}

		// Collect all results
		for range numGoroutines {
			select {
			case err := <-errors:
				t.Fatalf("Goroutine returned error: %v", err)
			case resp := <-results:
				require.NotNil(t, resp)
				require.Equal(t, "test-1.0.0", resp.Version)
				require.Equal(t, "test-commit", resp.Commit)
				require.True(t, resp.Demo)
				require.Equal(t, "http://localhost:8080", resp.InstanceUrl)
				require.NotNil(t, resp.Admin)
			}
		}
	})
}

func TestGetInstanceSetting(t *testing.T) {
	ctx := context.Background()

	t.Run("GetInstanceSetting - general setting", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Call GetInstanceSetting for general setting
		req := &apipb.GetInstanceSettingRequest{
			Name: "instance/settings/GENERAL",
		}
		resp, err := ts.Service.GetInstanceSetting(ctx, req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "instance/settings/GENERAL", resp.Name)

		// The general setting should have a general_setting field
		generalSetting := resp.GetGeneralSetting()
		require.NotNil(t, generalSetting)

		// General setting should have default values
		require.False(t, generalSetting.DisallowUserRegistration)
		require.False(t, generalSetting.DisallowPasswordAuth)
		require.Empty(t, generalSetting.AdditionalScript)
	})

	t.Run("GetInstanceSetting - storage setting", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a host user for storage setting access
		hostUser, err := ts.CreateHostUser(ctx, "testhost")
		require.NoError(t, err)

		// Add user to context
		userCtx := ts.CreateUserContext(ctx, hostUser.ID)

		// Call GetInstanceSetting for storage setting
		req := &apipb.GetInstanceSettingRequest{
			Name: "instance/settings/STORAGE",
		}
		resp, err := ts.Service.GetInstanceSetting(userCtx, req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "instance/settings/STORAGE", resp.Name)

		// The storage setting should have a storage_setting field
		storageSetting := resp.GetStorageSetting()
		require.NotNil(t, storageSetting)
	})

	t.Run("GetInstanceSetting - retired reaction setting", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		_, err := ts.Service.GetInstanceSetting(ctx, &apipb.GetInstanceSettingRequest{
			Name: "instance/settings/MEMO_RELATED",
		})
		require.Error(t, err)
	})

	t.Run("GetInstanceSetting - tags setting", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		req := &apipb.GetInstanceSettingRequest{
			Name: "instance/settings/TAGS",
		}
		resp, err := ts.Service.GetInstanceSetting(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "instance/settings/TAGS", resp.Name)
		require.NotNil(t, resp.GetTagsSetting())
		require.Empty(t, resp.GetTagsSetting().GetTags())
	})

	t.Run("GetInstanceSetting - access setting", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		resp, err := ts.Service.GetInstanceSetting(ctx, &apipb.GetInstanceSettingRequest{
			Name: "instance/settings/ACCESS",
		})

		require.NoError(t, err)
		require.Equal(t, "instance/settings/ACCESS", resp.Name)
		require.Equal(t, apipb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC, resp.GetAccessSetting().GetAccessMode())
	})

	t.Run("GetInstanceSetting - AI setting requires authenticated user", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		admin, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		adminCtx := ts.CreateUserContext(ctx, admin.ID)

		regularUser, err := ts.CreateRegularUser(ctx, "user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, regularUser.ID)

		req := &apipb.GetInstanceSettingRequest{Name: "instance/settings/AI"}

		_, err = ts.Service.GetInstanceSetting(ctx, req)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not authenticated")

		resp, err := ts.Service.GetInstanceSetting(userCtx, req)
		require.NoError(t, err)
		require.NotNil(t, resp.GetAiSetting())
		require.Empty(t, resp.GetAiSetting().GetProviders())

		resp, err = ts.Service.GetInstanceSetting(adminCtx, req)
		require.NoError(t, err)
		require.NotNil(t, resp.GetAiSetting())
		require.Empty(t, resp.GetAiSetting().GetProviders())
	})

	t.Run("GetInstanceSetting - invalid setting name", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Call GetInstanceSetting with invalid name
		req := &apipb.GetInstanceSettingRequest{
			Name: "invalid/setting/name",
		}
		_, err := ts.Service.GetInstanceSetting(ctx, req)

		// Should return an error
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid instance setting name")
	})
}

func TestBatchGetInstanceSettings(t *testing.T) {
	ctx := context.Background()

	t.Run("BatchGetInstanceSettings - returns settings in request order", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		resp, err := ts.Service.BatchGetInstanceSettings(ctx, &apipb.BatchGetInstanceSettingsRequest{
			Names: []string{
				"instance/settings/TAGS",
				"instance/settings/GENERAL",
				"instance/settings/ACCESS",
			},
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.InstanceSettings, 3)
		require.Equal(t, "instance/settings/TAGS", resp.InstanceSettings[0].Name)
		require.NotNil(t, resp.InstanceSettings[0].GetTagsSetting())
		require.Equal(t, "instance/settings/GENERAL", resp.InstanceSettings[1].Name)
		require.NotNil(t, resp.InstanceSettings[1].GetGeneralSetting())
		require.Equal(t, "instance/settings/ACCESS", resp.InstanceSettings[2].Name)
		require.NotNil(t, resp.InstanceSettings[2].GetAccessSetting())
	})

	t.Run("BatchGetInstanceSettings - admin-only setting requires admin", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		regularUser, err := ts.CreateRegularUser(ctx, "batch-user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, regularUser.ID)

		_, err = ts.Service.BatchGetInstanceSettings(userCtx, &apipb.BatchGetInstanceSettingsRequest{
			Names: []string{"instance/settings/GENERAL", "instance/settings/USAGE_LIMITS"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")

		admin, err := ts.CreateHostUser(ctx, "batch-admin")
		require.NoError(t, err)
		adminCtx := ts.CreateUserContext(ctx, admin.ID)

		resp, err := ts.Service.BatchGetInstanceSettings(adminCtx, &apipb.BatchGetInstanceSettingsRequest{
			Names: []string{"instance/settings/GENERAL", "instance/settings/USAGE_LIMITS"},
		})
		require.NoError(t, err)
		require.Len(t, resp.InstanceSettings, 2)
		require.NotNil(t, resp.InstanceSettings[1].GetUsageLimitsSetting())
	})

	t.Run("BatchGetInstanceSettings - invalid setting name", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		_, err := ts.Service.BatchGetInstanceSettings(ctx, &apipb.BatchGetInstanceSettingsRequest{
			Names: []string{"instance/settings/GENERAL", "invalid/setting/name"},
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid instance setting name")
	})
}

func TestUpdateInstanceSetting(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateInstanceSetting - AI setting requires admin", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		regularUser, err := ts.CreateRegularUser(ctx, "user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, regularUser.ID)

		setting := &apipb.InstanceSetting{
			Name: "instance/settings/AI",
			Value: &apipb.InstanceSetting_AiSetting{
				AiSetting: &apipb.InstanceSetting_AISetting{
					Providers: []*apipb.InstanceSetting_AIProviderConfig{
						{
							Id:     "openai-main",
							Title:  "OpenAI",
							Type:   apipb.InstanceSetting_OPENAI,
							ApiKey: "sk-test",
						},
					},
				},
			},
		}

		_, err = ts.Service.UpdateInstanceSetting(ctx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: setting})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not authenticated")

		_, err = ts.Service.UpdateInstanceSetting(userCtx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: setting})
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")
	})

	t.Run("UpdateInstanceSetting - tags setting", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)

		resp, err := ts.Service.UpdateInstanceSetting(ts.CreateUserContext(ctx, hostUser.ID), &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/TAGS",
				Value: &apipb.InstanceSetting_TagsSetting_{
					TagsSetting: &apipb.InstanceSetting_TagsSetting{
						Tags: map[string]*apipb.InstanceSetting_TagMetadata{
							"bug": {
								BackgroundColor: &colorpb.Color{
									Red:   0.9,
									Green: 0.1,
									Blue:  0.1,
								},
							},
						},
					},
				},
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tags"}},
		})
		require.NoError(t, err)
		require.NotNil(t, resp.GetTagsSetting())
		require.Contains(t, resp.GetTagsSetting().GetTags(), "bug")
	})

	t.Run("UpdateInstanceSetting - invalid tags color", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)

		_, err = ts.Service.UpdateInstanceSetting(ts.CreateUserContext(ctx, hostUser.ID), &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/TAGS",
				Value: &apipb.InstanceSetting_TagsSetting_{
					TagsSetting: &apipb.InstanceSetting_TagsSetting{
						Tags: map[string]*apipb.InstanceSetting_TagMetadata{
							"bug": {
								BackgroundColor: &colorpb.Color{
									Red:   1.2,
									Green: 0.1,
									Blue:  0.1,
								},
							},
						},
					},
				},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid instance setting")
	})

	t.Run("UpdateInstanceSetting - tags setting without color", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)

		resp, err := ts.Service.UpdateInstanceSetting(ts.CreateUserContext(ctx, hostUser.ID), &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/TAGS",
				Value: &apipb.InstanceSetting_TagsSetting_{
					TagsSetting: &apipb.InstanceSetting_TagsSetting{
						Tags: map[string]*apipb.InstanceSetting_TagMetadata{
							"spoiler": {
								BlurContent: true,
							},
						},
					},
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, resp.GetTagsSetting())
		require.Contains(t, resp.GetTagsSetting().GetTags(), "spoiler")
		require.Nil(t, resp.GetTagsSetting().GetTags()["spoiler"].GetBackgroundColor())
		require.True(t, resp.GetTagsSetting().GetTags()["spoiler"].GetBlurContent())
	})

	t.Run("UpdateInstanceSetting rejects legacy storage changes", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		adminCtx := ts.CreateUserContext(ctx, hostUser.ID)
		_, err = ts.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
			Key: storepb.InstanceSettingKey_STORAGE,
			Value: &storepb.InstanceSetting_StorageSetting{StorageSetting: &storepb.InstanceStorageSetting{
				StorageType: storepb.InstanceStorageSetting_S3,
				S3Config: &storepb.StorageS3Config{
					AccessKeyId: "original-id", AccessKeySecret: "original-secret",
					Endpoint: "https://original.example.test", Region: "test", Bucket: "memos",
				},
			}},
		})
		require.NoError(t, err)

		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/STORAGE",
				Value: &apipb.InstanceSetting_StorageSetting_{
					StorageSetting: &apipb.InstanceSetting_StorageSetting{
						S3Config: &apipb.InstanceSetting_StorageSetting_S3Config{
							AccessKeyId: "replacement-id", AccessKeySecret: "replacement-secret",
							Endpoint: "https://replacement.example.test", Region: "test", Bucket: "memos",
						},
					},
				},
			},
		})
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		require.NotContains(t, err.Error(), "replacement-secret")

		resp, err := ts.Service.GetInstanceSetting(adminCtx, &apipb.GetInstanceSettingRequest{
			Name: "instance/settings/STORAGE",
		})
		require.NoError(t, err)
		setting := resp.GetStorageSetting()
		require.Equal(t, apipb.InstanceSetting_StorageSetting_LOCAL, setting.GetStorageType())
		require.Nil(t, setting.GetS3Config())
		require.Len(t, setting.GetStorages(), 1)
		require.Nil(t, setting.GetStorages()[0].GetS3Config())

		stored, err := ts.Store.GetInstanceStorageSetting(ctx)
		require.NoError(t, err)
		require.Equal(t, "original-secret", stored.GetS3Config().GetAccessKeySecret())
		require.Equal(t, "https://original.example.test", stored.GetS3Config().GetEndpoint())
	})

	t.Run("UpdateInstanceSetting - AI provider keys are write-only and preserved on empty", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		adminCtx := ts.CreateUserContext(ctx, hostUser.ID)

		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/AI",
				Value: &apipb.InstanceSetting_AiSetting{
					AiSetting: &apipb.InstanceSetting_AISetting{
						Providers: []*apipb.InstanceSetting_AIProviderConfig{
							{
								Id:     "openai-main",
								Title:  "OpenAI",
								Type:   apipb.InstanceSetting_OPENAI,
								ApiKey: "sk-original",
							},
						},
					},
				},
			},
		})
		require.NoError(t, err)

		resp, err := ts.Service.GetInstanceSetting(adminCtx, &apipb.GetInstanceSettingRequest{
			Name: "instance/settings/AI",
		})
		require.NoError(t, err)
		require.Len(t, resp.GetAiSetting().GetProviders(), 1)
		provider := resp.GetAiSetting().GetProviders()[0]
		require.Empty(t, provider.GetApiKey(), "AI provider API key must never be returned in responses")
		require.True(t, provider.GetApiKeySet())
		require.Equal(t, "sk-o...inal", provider.GetApiKeyHint())
		require.Equal(t, "https://api.openai.com/v1", provider.GetEndpoint())

		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/AI",
				Value: &apipb.InstanceSetting_AiSetting{
					AiSetting: &apipb.InstanceSetting_AISetting{
						Providers: []*apipb.InstanceSetting_AIProviderConfig{
							{
								Id:     "openai-main",
								Title:  "OpenAI primary",
								Type:   apipb.InstanceSetting_OPENAI,
								ApiKey: "",
							},
						},
					},
				},
			},
		})
		require.NoError(t, err)

		stored, err := ts.Store.GetInstanceAISetting(ctx)
		require.NoError(t, err)
		require.Len(t, stored.GetProviders(), 1)
		require.Equal(t, "sk-original", stored.GetProviders()[0].GetApiKey(),
			"existing AI provider API key must be preserved when an empty value is sent")
		require.Equal(t, "OpenAI primary", stored.GetProviders()[0].GetTitle())
	})

	t.Run("UpdateInstanceSetting - transcription provider_id must reference an existing provider", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		adminCtx := ts.CreateUserContext(ctx, hostUser.ID)

		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/AI",
				Value: &apipb.InstanceSetting_AiSetting{
					AiSetting: &apipb.InstanceSetting_AISetting{
						Providers: []*apipb.InstanceSetting_AIProviderConfig{
							{
								Id:     "openai-main",
								Title:  "OpenAI",
								Type:   apipb.InstanceSetting_OPENAI,
								ApiKey: "sk-test",
							},
						},
						Transcription: &apipb.InstanceSetting_TranscriptionConfig{
							ProviderId: "does-not-exist",
						},
					},
				},
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "transcription provider_id")
	})

	t.Run("UpdateInstanceSetting - transcription strings are length-capped", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		adminCtx := ts.CreateUserContext(ctx, hostUser.ID)

		base := &apipb.InstanceSetting{
			Name: "instance/settings/AI",
			Value: &apipb.InstanceSetting_AiSetting{
				AiSetting: &apipb.InstanceSetting_AISetting{
					Providers: []*apipb.InstanceSetting_AIProviderConfig{
						{
							Id:     "openai-main",
							Title:  "OpenAI",
							Type:   apipb.InstanceSetting_OPENAI,
							ApiKey: "sk-test",
						},
					},
				},
			},
		}

		oversizedModel := strings.Repeat("a", 257)
		base.GetAiSetting().Transcription = &apipb.InstanceSetting_TranscriptionConfig{
			ProviderId: "openai-main",
			Model:      oversizedModel,
		}
		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: base})
		require.Error(t, err)
		require.Contains(t, err.Error(), "transcription model")

		oversizedLanguage := strings.Repeat("a", 33)
		base.GetAiSetting().Transcription = &apipb.InstanceSetting_TranscriptionConfig{
			ProviderId: "openai-main",
			Language:   oversizedLanguage,
		}
		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: base})
		require.Error(t, err)
		require.Contains(t, err.Error(), "transcription language")

		oversizedPrompt := strings.Repeat("a", 4097)
		base.GetAiSetting().Transcription = &apipb.InstanceSetting_TranscriptionConfig{
			ProviderId: "openai-main",
			Prompt:     oversizedPrompt,
		}
		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: base})
		require.Error(t, err)
		require.Contains(t, err.Error(), "transcription prompt")
	})

	t.Run("UpdateInstanceSetting - transcription is preserved when omitted on update", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		adminCtx := ts.CreateUserContext(ctx, hostUser.ID)

		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/AI",
				Value: &apipb.InstanceSetting_AiSetting{
					AiSetting: &apipb.InstanceSetting_AISetting{
						Providers: []*apipb.InstanceSetting_AIProviderConfig{
							{
								Id:     "openai-main",
								Title:  "OpenAI",
								Type:   apipb.InstanceSetting_OPENAI,
								ApiKey: "sk-test",
							},
						},
						Transcription: &apipb.InstanceSetting_TranscriptionConfig{
							ProviderId: "openai-main",
							Model:      "whisper-1",
							Language:   "en",
							Prompt:     "names: Alice",
						},
					},
				},
			},
		})
		require.NoError(t, err)

		_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{
			InstanceSetting: &apipb.InstanceSetting{
				Name: "instance/settings/AI",
				Value: &apipb.InstanceSetting_AiSetting{
					AiSetting: &apipb.InstanceSetting_AISetting{
						Providers: []*apipb.InstanceSetting_AIProviderConfig{
							{
								Id:     "openai-main",
								Title:  "OpenAI",
								Type:   apipb.InstanceSetting_OPENAI,
								ApiKey: "",
							},
						},
					},
				},
			},
		})
		require.NoError(t, err)

		stored, err := ts.Store.GetInstanceAISetting(ctx)
		require.NoError(t, err)
		require.NotNil(t, stored.GetTranscription())
		require.Equal(t, "openai-main", stored.GetTranscription().GetProviderId())
		require.Equal(t, "whisper-1", stored.GetTranscription().GetModel())
		require.Equal(t, "en", stored.GetTranscription().GetLanguage())
		require.Equal(t, "names: Alice", stored.GetTranscription().GetPrompt())
	})
}

func TestConfigurationStateIsNotExposed(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	hostUser, err := ts.CreateHostUser(ctx, "admin")
	require.NoError(t, err)
	adminCtx := ts.CreateUserContext(ctx, hostUser.ID)

	_, err = ts.Store.RunConfigurationTransaction(ctx, func(context.Context, *store.ConfigurationTransaction) (*storepb.InstanceConfigurationState, error) {
		return &storepb.InstanceConfigurationState{ConfigDigest: "digest"}, nil
	})
	require.NoError(t, err)

	const name = "instance/settings/CONFIGURATION_STATE"
	_, err = ts.Service.GetInstanceSetting(adminCtx, &apipb.GetInstanceSettingRequest{Name: name})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = ts.Service.BatchGetInstanceSettings(adminCtx, &apipb.BatchGetInstanceSettingsRequest{Names: []string{name}})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: &apipb.InstanceSetting{Name: name}})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Contains(t, err.Error(), "state record and cannot be written")

	state, err := ts.Store.GetConfigurationState(ctx)
	require.NoError(t, err)
	require.Equal(t, "digest", state.ConfigDigest, "the API did not overwrite the record")
}
