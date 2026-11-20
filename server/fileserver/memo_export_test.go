package fileserver

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/core/memoexport"
	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/ratelimit"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

type memoExportFixture struct {
	svc    *api.APIService
	fs     *FileServerService
	stores *store.Store
	mux    *http.ServeMux
	admin  *store.User
	member *store.User
}

func newMemoExportFixture(ctx context.Context, t *testing.T) (*memoExportFixture, func()) {
	t.Helper()
	svc, fs, stores, cleanup := newShareAttachmentTestServices(ctx, t)
	fs.MemoExporter = svc
	fs.RateLimiter = ratelimit.NewMemoryLimiter(ratelimit.DefaultPolicy())

	admin, err := stores.CreateUser(ctx, &store.User{Username: "export-admin", Role: store.RoleAdmin, Email: "export-admin@example.com"})
	require.NoError(t, err)
	member, err := stores.CreateUser(ctx, &store.User{Username: "export-member", Role: store.RoleUser, Email: "export-member@example.com"})
	require.NoError(t, err)
	for _, owner := range []*store.User{admin, member} {
		ownerCtx := context.WithValue(ctx, auth.UserIDContextKey, owner.ID)
		for i := range 2 {
			_, err := svc.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{
				Content:    fmt.Sprintf("%s memo %d", owner.Username, i),
				Visibility: apipb.Visibility_PRIVATE,
			}})
			require.NoError(t, err)
		}
	}

	mux := http.NewServeMux()
	fs.RegisterRoutes(mux, http.HandlerFunc(httphandler.NotFound))
	return &memoExportFixture{svc: svc, fs: fs, stores: stores, mux: mux, admin: admin, member: member}, cleanup
}

func (f *memoExportFixture) request(ctx context.Context, t *testing.T, method, username string, as *store.User) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/file/users/"+username+"/export", nil)
	if as != nil {
		req.AddCookie(newRefreshTokenCookie(ctx, t, f.stores, as.ID, f.svc.Secret))
	}
	recorder := httptest.NewRecorder()
	f.mux.ServeHTTP(recorder, req)
	return recorder
}

func TestServeMemoExport_StreamsOwnArchive(t *testing.T) {
	ctx := context.Background()
	f, cleanup := newMemoExportFixture(ctx, t)
	defer cleanup()

	recorder := f.request(ctx, t, http.MethodGet, f.member.Username, f.member)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, memoexport.MediaType, recorder.Header().Get("Content-Type"))
	require.Regexp(t, `^attachment; filename=memos-export-export-member-\d{8}T\d{6}Z\.zip$`, recorder.Header().Get("Content-Disposition"))
	require.Equal(t, privateAttachmentCacheControl, recorder.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))

	archive, err := memoexport.Read(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	require.NoError(t, err)
	require.Equal(t, "export-member", archive.Manifest.Scope.User.Username)
	require.Len(t, archive.Memos, 2)
	for _, memo := range archive.Memos {
		require.Equal(t, "export-member", memo.Creator)
	}

	own := f.request(ctx, t, http.MethodGet, f.admin.Username, f.admin)
	require.Equal(t, http.StatusOK, own.Code)
	ownArchive, err := memoexport.Read(bytes.NewReader(own.Body.Bytes()), int64(own.Body.Len()))
	require.NoError(t, err)
	require.Equal(t, "export-admin", ownArchive.Manifest.Scope.User.Username)
}

func TestServeMemoExport_RefusesOtherUsersAndAnonymous(t *testing.T) {
	ctx := context.Background()
	f, cleanup := newMemoExportFixture(ctx, t)
	defer cleanup()

	require.Equal(t, http.StatusForbidden, f.request(ctx, t, http.MethodGet, f.admin.Username, f.member).Code)
	require.Equal(t, http.StatusUnauthorized, f.request(ctx, t, http.MethodGet, f.member.Username, nil).Code)
	require.Equal(t, http.StatusForbidden, f.request(ctx, t, http.MethodGet, "nobody", f.admin).Code)

	// The error envelope is JSON; nothing of the archive leaks.
	forbidden := f.request(ctx, t, http.MethodGet, f.member.Username, f.admin)
	require.Equal(t, http.StatusForbidden, forbidden.Code)
	require.Equal(t, "application/json", forbidden.Header().Get("Content-Type"))
	require.Contains(t, forbidden.Body.String(), "forbidden access")
}

func TestServeMemoExport_SelectedMemos(t *testing.T) {
	ctx := context.Background()
	f, cleanup := newMemoExportFixture(ctx, t)
	defer cleanup()

	owned, err := f.stores.ListMemos(ctx, &store.FindMemo{CreatorID: &f.admin.ID})
	require.NoError(t, err)
	require.Len(t, owned, 2)
	other, err := f.stores.ListMemos(ctx, &store.FindMemo{CreatorID: &f.member.ID})
	require.NoError(t, err)
	require.Len(t, other, 2)

	request := func(method, query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/file/users/"+f.admin.Username+"/export?"+query, nil)
		req.AddCookie(newRefreshTokenCookie(ctx, t, f.stores, f.admin.ID, f.svc.Secret))
		recorder := httptest.NewRecorder()
		f.mux.ServeHTTP(recorder, req)
		return recorder
	}
	require.Equal(t, http.StatusBadRequest, request(http.MethodHead, "memo=%zz").Code)
	require.Equal(t, http.StatusBadRequest, request(http.MethodGet, "memo=").Code)
	require.Equal(t, http.StatusNotFound, request(http.MethodHead, "memo="+other[0].UID).Code)
	require.Equal(t, http.StatusNotFound, request(http.MethodGet, "memo="+owned[0].UID+"&memo="+other[0].UID).Code)

	query := "memo=" + owned[0].UID + "&memo=" + owned[0].UID
	require.Equal(t, http.StatusOK, request(http.MethodHead, query).Code)
	recorder := request(http.MethodGet, query)
	require.Equal(t, http.StatusOK, recorder.Code)
	archive, err := memoexport.Read(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	require.NoError(t, err)
	require.Len(t, archive.Memos, 1)
	require.Equal(t, owned[0].UID, archive.Memos[0].UID)
	require.Equal(t, 1, archive.Manifest.Counts.Memos)
}

func TestServeMemoExport_OmitsLegacyForeignCommentParent(t *testing.T) {
	ctx := context.Background()
	svc, fs, cleanup := newSharedProfileTestServices(ctx, t)
	defer cleanup()
	fs.MemoExporter = svc
	owner, err := svc.Store.CreateUser(ctx, &store.User{Username: "comment-owner", Role: store.RoleUser, Email: "comment-owner@example.com"})
	require.NoError(t, err)
	other, err := svc.Store.CreateUser(ctx, &store.User{Username: "former-author", Role: store.RoleUser, Email: "former-author@example.com"})
	require.NoError(t, err)
	comment, err := svc.Store.CreateMemo(ctx, &store.Memo{UID: "legacy-comment", CreatorID: owner.ID, Content: "my comment", Visibility: store.Private})
	require.NoError(t, err)
	parent, err := svc.Store.CreateMemo(ctx, &store.Memo{UID: "legacy-parent", CreatorID: other.ID, Content: "someone else's memo", Visibility: store.Public})
	require.NoError(t, err)
	db, err := sql.Open("sqlite", svc.Profile.DSN)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.ExecContext(ctx, "INSERT INTO memo_relation (memo_id, related_memo_id, type) VALUES (?, ?, ?)", comment.ID, parent.ID, store.MemoRelationComment)
	require.NoError(t, err)

	mux := http.NewServeMux()
	fs.RegisterRoutes(mux, http.HandlerFunc(httphandler.NotFound))
	f := &memoExportFixture{svc: svc, fs: fs, stores: svc.Store, mux: mux}
	recorder := f.request(ctx, t, http.MethodGet, owner.Username, owner)
	require.Equal(t, http.StatusOK, recorder.Code)
	archive, err := memoexport.Read(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
	require.NoError(t, err)
	require.Len(t, archive.Memos, 1)
	require.Equal(t, comment.UID, archive.Memos[0].UID)
	require.Empty(t, archive.Memos[0].Parent)
}

func TestServeMemoExport_HeadPreflightChecksWithoutExporting(t *testing.T) {
	ctx := context.Background()
	f, cleanup := newMemoExportFixture(ctx, t)
	defer cleanup()

	head := f.request(ctx, t, http.MethodHead, f.member.Username, f.member)
	require.Equal(t, http.StatusOK, head.Code)
	require.Equal(t, memoexport.MediaType, head.Header().Get("Content-Type"))
	require.Regexp(t, `^attachment; filename=memos-export-export-member-`, head.Header().Get("Content-Disposition"))
	require.Zero(t, head.Body.Len())

	require.Equal(t, http.StatusForbidden, f.request(ctx, t, http.MethodHead, f.member.Username, f.admin).Code)
	require.Equal(t, http.StatusUnauthorized, f.request(ctx, t, http.MethodHead, f.member.Username, nil).Code)

	// A preflight charges nothing: the full hourly budget is still available.
	budget := ratelimit.DefaultPolicy()[ratelimit.ScopeArchiveUser].Limit
	for range budget {
		require.Equal(t, http.StatusOK, f.request(ctx, t, http.MethodHead, f.member.Username, f.member).Code)
	}
	for range budget {
		require.Equal(t, http.StatusOK, f.request(ctx, t, http.MethodGet, f.member.Username, f.member).Code)
	}
	exhausted := f.request(ctx, t, http.MethodGet, f.member.Username, f.member)
	require.Equal(t, http.StatusTooManyRequests, exhausted.Code)
	require.NotEmpty(t, exhausted.Header().Get("Retry-After"))
	require.Equal(t, http.StatusTooManyRequests, f.request(ctx, t, http.MethodHead, f.member.Username, f.member).Code)
}

// stubExporter stands in for the API service. It parks inside WriteMemoExport
// until released, so a test can observe a running export, or fails before
// writing a byte when err is set.
type stubExporter struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (e *stubExporter) WriteMemoExport(ctx context.Context, _ *store.User, w io.Writer, _ time.Time, _ ...string) error {
	if e.err != nil {
		return e.err
	}
	close(e.started)
	select {
	case <-e.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	_, err := w.Write([]byte("PK"))
	return err
}

func TestServeMemoExport_OneExportAtATime(t *testing.T) {
	ctx := context.Background()
	f, cleanup := newMemoExportFixture(ctx, t)
	defer cleanup()

	exporter := &stubExporter{started: make(chan struct{}), release: make(chan struct{})}
	f.fs.MemoExporter = exporter
	running := make(chan *httptest.ResponseRecorder, 1)
	go func() { running <- f.request(ctx, t, http.MethodGet, f.member.Username, f.member) }()
	<-exporter.started

	// The lock spans the whole export: while one streams, another is refused.
	busy := f.request(ctx, t, http.MethodGet, f.member.Username, f.member)
	require.Equal(t, http.StatusTooManyRequests, busy.Code)
	require.Contains(t, busy.Body.String(), "another export is running")
	require.Equal(t, http.StatusTooManyRequests, f.request(ctx, t, http.MethodHead, f.member.Username, f.member).Code)

	close(exporter.release)
	require.Equal(t, http.StatusOK, (<-running).Code)

	// The refused request refunded its budget unit and the lock is free again.
	f.fs.MemoExporter = f.svc
	require.Equal(t, http.StatusOK, f.request(ctx, t, http.MethodGet, f.member.Username, f.member).Code)
	require.Equal(t, ratelimit.DefaultPolicy()[ratelimit.ScopeArchiveUser].Limit-2,
		f.fs.RateLimiter.Allowed(ratelimit.ScopeArchiveUser, fmt.Sprint(f.member.ID), 1).Remaining)
}

func TestServeMemoExport_FailureBeforeFirstByteIsAnErrorResponse(t *testing.T) {
	ctx := context.Background()
	f, cleanup := newMemoExportFixture(ctx, t)
	defer cleanup()

	f.fs.MemoExporter = &stubExporter{err: errors.New("storage unavailable")}
	failed := f.request(ctx, t, http.MethodGet, f.member.Username, f.member)
	require.Equal(t, http.StatusInternalServerError, failed.Code)
	require.Equal(t, "application/json", failed.Header().Get("Content-Type"))
	require.Empty(t, failed.Header().Get("Content-Disposition"))
	require.Contains(t, failed.Body.String(), "failed to export memos")
	require.NotContains(t, failed.Body.String(), "storage unavailable")

	// The lock is released after a failure.
	f.fs.MemoExporter = f.svc
	require.Equal(t, http.StatusOK, f.request(ctx, t, http.MethodGet, f.member.Username, f.member).Code)
}

func TestServeMemoExport_RouteDisabledWithoutExporter(t *testing.T) {
	ctx := context.Background()
	f, cleanup := newMemoExportFixture(ctx, t)
	defer cleanup()

	f.fs.MemoExporter = nil
	require.Equal(t, http.StatusNotFound, f.request(ctx, t, http.MethodGet, f.member.Username, f.member).Code)
}
