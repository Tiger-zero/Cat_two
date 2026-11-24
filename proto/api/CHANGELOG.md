# API Changelog

Breaking and deprecated changes to the browser protocol, external MCP tools and related behavior, newest release first.

Record schema, route, tool and behavior breaks here.

Add entries under a `## Unreleased` heading. After the release ships, rename it to the release tag.

`InstanceSetting` and `User` also define declarative configuration fields. Record a removed or renamed configuration field and its replacement, if available.

```markdown
## Unreleased

### Breaking

- `MemoService.ListShortcuts` is removed. Use `MemoService.ListMemos` with a filter.
- `InstanceSetting.StorageSetting.upload_size_limit_mb`: removed. Configuration files that set `uploadSizeLimitMb` are
  rejected from this release; set the `UPLOAD_BYTES` usage limit instead.

### Deprecated

- `Memo.unassigned`: use an unset `Memo.space`. Removed in a later release.
```

## Unreleased

### Breaking

- Cat_two removes the generic REST API, including `/api`, `/api/*` and the old `/memos.api.v1.*` paths. Removed paths return `404`. Personal access tokens can no longer be created or used. The web app uses `/memos.api.*` Connect and `/internal/` session routes. Attachments and ZIP downloads remain on `/file/` with browser session authentication. External note integrations use dedicated MCP connection tokens on `/mcp`. Telegram polls its Bot API, and enterprise WeChat uses the signed `/integrations/wecom` callback.
- Cat_two requires the current single-account SQLite schema. Startup rejects incompatible databases. Keep prior databases and volumes with the original image and use an independent volume for a new installation. [Deployment and backup](../../.agents/skills/cat-two-product-development/references/deployment.md#备份和升级)
- Declarative configuration accepts GENERAL settings and one administrator. Cat_two determines the local storage paths. Older declarations for identity providers, linked identities, access policy, executable scripts, regular users and storage must be removed before applying.
- Private instances have a one-time administrator setup. The server rejects additional registration and access-policy changes. Password changes require a browser session.
- The Space, saved view, identity provider, linked identity, notification, webhook and configuration status APIs are removed. `MemoService.ListMemos` accepts a CEL expression for list filtering. Note access depends on the signed-in account. Telegram and WeCom settings use the browser's native endpoints.
- MCP connections are created in **Settings → MCP**, with read-only or read-write permissions and a duration of 7 days, 14 days, one calendar year, or permanent. Deleting a connection invalidates the token on the next request. [MCP tools](../../server/mcp/README.md#tool-surface)
- When an account enables two-step verification, password sign-in requires `password_credentials.otp_code`. `ErrorInfo.reason` is `TOTP_REQUIRED` for a missing code and `TOTP_INVALID` for an invalid or reused code. Changing or resetting the factor revokes interactive sessions.
- Cat_two binds browser access and refresh credentials to the same revocable session. `SignOut` invalidates both credentials for the current session.
- Cat_two removes `GENERAL.additionalScript`. Branding and styling remain configurable.
- Create memo and comment requests accept optional `idempotent` with an explicit `memoId` or `commentId`. An exact
  retry returns the committed note. A conflicting payload returns `ALREADY_EXISTS`. Memo responses include
  `revision`. Updates can pass `expectedRevision` for a transactional precondition, with `ABORTED` on stale edits.
- The built-in transcription service is removed. `/memos.api.AIService/Transcribe` returns `404`.
- `MemoService.DeleteMemo` moves notes and replies to the recycle bin for 30 days. The browser uses `GET /internal/trash`, `POST /internal/trash/{uid}/restore` and `DELETE /internal/trash/{uid}`. Restoring recovers the prior normal or archived state.
- Cat_two restricts note and attachment reads, statistics and ZIP exports to the owning account. Notes are private. The usage limits instance setting no longer applies. Upload chunk and request validation continue to apply.
- New attachments are written under `assets/`. `GetInstanceStats` reports the active data directory and its managed Markdown and attachment directories.
- The `MEMO_RELATED` setting is removed. Reactions use the Unicode emoji picker.

## 26.10.0

### Breaking

- `UserService.ExportMemos` and `ExportMemosRequest` are removed. Export is an instance-administrator download at
  `GET /file/users/{username}/export`, authenticated like attachments (Bearer header or session cookie), streaming the
  archive with `Content-Disposition`. One export runs at a time per instance; a second request receives `429`. Members
  ask an administrator for their archive. `ImportMemos` is unchanged.
- Access tokens are now bound to the user's password hash and become invalid immediately after a password change or
  an administrator password reset. Pre-upgrade access tokens are also rejected; clients must refresh them or sign in
  again. The browser session that changes its own password can still refresh; other refresh sessions are revoked.
- Usage limits replace the instance-wide upload size and memo content caps. `StorageSetting.upload_size_limit_mb`
  and `MemoRelatedSetting.content_length_limit` are removed and old values are not migrated. The limits are the
  `USAGE_LIMITS` instance setting, `instance/settings/USAGE_LIMITS`, an ordered list of entries read and replaced whole
  through `GetInstanceSetting` and `UpdateInstanceSetting`; the first entry naming a user applies, the last entry
  applies to all users. The `usageLimits/{usage_limit}` resource and its five RPCs do not exist. `UserUsage` returns
  the limit set in effect as `limits` plus `limit_index`. Exceeding a limit returns `RESOURCE_EXHAUSTED` with reason
  `QUOTA_EXCEEDED` and metadata `metric`, `limit`, and `usage`.
- `MemoRelatedSetting.enable_double_click_edit` is removed; double-click editing is now a per-browser preference in
  the web client and is no longer an instance setting. `MemoRelatedSetting` keeps the single field `reactions = 1`.
- The deployment configuration files under `/etc/secrets` (`memos-idp-*.json`, `memos-instance-setting-*.json`)
  introduced in v0.31.0 are no longer read, and the stored values they overrode take effect again. Identity providers,
  instance settings, bootstrap users, and their linked identities are declared instead as `memos.api.Configuration`
  documents in the configuration directory (`--config-dir` or `MEMOS_CONFIG_DIR`, default `/etc/memos/config`), applied
  at startup unless `--config-apply=off`, or through `memos config plan` and `memos config apply`. Keep an identity
  provider's UID when moving it so linked accounts keep working. An API write that changes a field a file manages
  returns `FAILED_PRECONDITION` with `ErrorInfo{reason: MANAGED_BY_CONFIGURATION}` naming the file and fields;
  `GET /api/instance/configuration` (administrators) lists what is managed.
- `CreateIdentityProvider` and `UpdateIdentityProvider` validate the provider as it will be stored and return
  `INVALID_ARGUMENT` when it has no title, an invalid `identifier_filter`, no `client_id`, a non-absolute or non-HTTP(S)
  `auth_url`, `token_url`, or `user_info_url`, an empty scope entry, or no `field_mapping.identifier`. A request that
  omitted `config` or `field_mapping` previously failed with `INTERNAL`.
- `UpdateInstanceSetting` for `STORAGE` requires every new or changed S3 storage to have `endpoint`, `region`, `bucket`,
  `access_key_id`, and `access_key_secret` (an empty secret still keeps the stored one). For `GENERAL`,
  `week_start_day_offset` must be between -1 and 6. `BASIC` cannot be written. Notification validation errors name
  fields in proto form (`smtp_host`, `use_tls`).
- The API no longer carries a `v1` version marker. Every client must update its paths:
  - REST routes move from `/api/v1/...` to `/api/...` (for example `GET /api/memos`). Server-sent events move from
    `/api/v1/sse` to `/api/sse`.
  - The proto package `memos.api.v1` is now `memos.api`, so Connect and gRPC procedures move from
    `/memos.api.v1.<Service>/<Method>` to `/memos.api.<Service>/<Method>`.
  - Resource types move from `memos.api.v1/<Type>` to `memos.api/<Type>`.
  - The `domain` of `google.rpc.ErrorInfo` details is now `memos.api`.
  - For this release only, requests to `/api/v1/...` and `/memos.api.v1.*` return `410 Gone` with an `UNIMPLEMENTED`
    error that points here. The next release removes these routes.
- Field numbers in every message are renumbered from 1 in declaration order, and all `reserved` ranges are removed.
  JSON field names are unchanged; clients that use the binary protobuf encoding must regenerate from the new schema.
- Enum values are renumbered from 0 in declaration order, including `User.Role` (`ADMIN = 1`, `USER = 2`) and
  `UserSetting.Key` (`GENERAL = 1`, `WEBHOOKS = 2`, `TAGS = 3`). Clients that send numeric enum values must update them.
- Names now follow AIP conventions:
  - `MemoService`: `ListMemoAttachments`, `ListMemoRelations`, `ListMemoComments`, `CreateMemoComment`,
    `ListMemoReactions`, and `UpsertMemoReaction` take `parent` instead of `name` (REST path variable
    `{parent=memos/*}`). `ListMemoCommentsResponse.memos` is now `comments`. The nested `MemoRelation.Memo` type is
    now `MemoRelation.MemoRef`. The `MemoShare` resource singular/plural is now `memoShare`/`memoShares`.
  - `UserService.ListAllUserStats` is now `ListUserStats`. It requires a `parent`: `users/-` for all users (REST
    `GET /api/users/-/stats` instead of `GET /api/users:stats`) or `users/{user}` for one user. The response field
    `stats` is now `user_stats`.
  - `BatchGetUsersRequest.usernames` is now `names` and takes resource names (`users/{user}`), not bare usernames.
  - `PersonalAccessToken.created_at`, `expires_at`, and `last_used_at` are now `create_time`, `expire_time`, and
    `last_use_time`.
  - `LinkedIdentity.idp_name` and `extern_uid` are now `identity_provider` and `external_uid`;
    `CreateLinkedIdentityRequest.idp_name` is now `identity_provider`.
  - The `MemoView` and `UserNotification` resource singulars are now `memoView` and `userNotification`.
  - `UserStats.tag_count` is now `tag_counts`.
  - `MemoImportPlan`, `MemoImportReport`, and `MemoImportIssue` are now `ImportMemosPlan`, `ImportMemosReport`, and
    `ImportMemosIssue`. `ImportMemosPlan.memos` and `attachments` are now `memo_count` and `attachment_count`;
    `ImportMemosIssue.memo` is now `memo_uid`.
  - `UserNotification.SpaceInvitationPayload.State` is now `InvitationState`.
  - `UpdateUserSettingRequest.setting` and `ListUserSettingsResponse.settings` are now `user_setting` and
    `user_settings`; `UpdateInstanceSettingRequest.setting` and `BatchGetInstanceSettingsResponse.settings` are now
    `instance_setting` and `instance_settings`.
  - `SignInResponse.access_token_expires_at` and `RefreshTokenResponse.expires_at` are both now
    `access_token_expire_time`.
  - `SignInRequest.SSOCredentials` is now `SsoCredentials`, and its `idp_name` is now `identity_provider`.
