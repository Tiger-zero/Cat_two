package test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/store"
	storedb "github.com/usememos/memos/store/db"
)

func openMemoBodyStore(t *testing.T, p *profile.Profile) *store.Store {
	t.Helper()
	driver, err := storedb.NewDBDriver(p)
	require.NoError(t, err)
	st := store.New(driver, p)
	t.Cleanup(func() { _ = st.Close() })
	require.NoError(t, st.InitializeSchema(t.Context()))
	return st
}

func memoBodyPath(t *testing.T, st *store.Store, digest string) string {
	t.Helper()
	var namespace string
	require.NoError(t, st.GetDriver().GetDB().QueryRowContext(t.Context(), `SELECT namespace FROM memo_body_namespace WHERE id = 1`).Scan(&namespace))
	return filepath.Join(st.GetDataDir(), store.MemoBodyDirectory, namespace, digest+".md")
}

func assertMemoBodyFile(t *testing.T, st *store.Store, id int32, content string) string {
	t.Helper()
	var projection, digest string
	require.NoError(t, st.GetDriver().GetDB().QueryRowContext(t.Context(), `SELECT content, body_digest FROM memo WHERE id = ?`, id).Scan(&projection, &digest))
	require.Equal(t, content, projection)
	hash := sha256.Sum256([]byte(content))
	require.Equal(t, hex.EncodeToString(hash[:]), digest)
	path := memoBodyPath(t, st, digest)
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(file))
	return path
}

func TestMemoBodiesSweepLiveAndTrashWithinDatabaseNamespace(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	p := getTestingProfile(t)
	p.Personal = true
	st := openMemoBodyStore(t, p)
	owner, err := createTestingHostUser(ctx, st)
	require.NoError(t, err)
	live, err := st.CreateMemo(ctx, &store.Memo{UID: "sweep-live", CreatorID: owner.ID, Visibility: store.Private, Content: "保留\n"})
	require.NoError(t, err)
	trashed, err := st.CreateMemo(ctx, &store.Memo{UID: "sweep-trash", CreatorID: owner.ID, Visibility: store.Private, Content: "回收站\n"})
	require.NoError(t, err)
	now := time.Now().Unix()
	_, err = st.ApplyMemoTrash(ctx, &store.MemoTrashMutation{MemoID: trashed.ID, ActorUserID: owner.ID, Action: store.MemoTrashMove, NowTs: now})
	require.NoError(t, err)
	require.NoError(t, st.SweepMemoBodies(ctx))
	livePath := assertMemoBodyFile(t, st, live.ID, live.Content)
	trashedPath := assertMemoBodyFile(t, st, trashed.ID, trashed.Content)
	read, err := st.GetMemo(ctx, &store.FindMemo{ID: &trashed.ID, Trash: store.MemoTrashOnly})
	require.NoError(t, err)
	require.Equal(t, trashed.Content, read.Content)
	require.Equal(t, trashed.Revision+1, read.Revision)
	require.Empty(t, read.CreationRequestDigest)
	_, err = st.ApplyMemoTrash(ctx, &store.MemoTrashMutation{MemoID: trashed.ID, ActorUserID: owner.ID, Action: store.MemoTrashPurge, NowTs: now})
	require.NoError(t, err)
	require.FileExists(t, trashedPath, "an active reader may hold the previous database snapshot")

	otherProfile := *p
	otherProfile.DSN = filepath.Join(p.Data, "other.db")
	other := openMemoBodyStore(t, &otherProfile)
	otherOwner, err := createTestingHostUser(ctx, other)
	require.NoError(t, err)
	otherMemo, err := other.CreateMemo(ctx, &store.Memo{UID: "other-copy", CreatorID: otherOwner.ID, Visibility: store.Private, Content: trashed.Content})
	require.NoError(t, err)
	otherPath := assertMemoBodyFile(t, other, otherMemo.ID, trashed.Content)
	require.NotEqual(t, filepath.Dir(otherPath), filepath.Dir(trashedPath))

	orphanStages := make([]string, 150)
	for i := range orphanStages {
		path := filepath.Join(filepath.Dir(livePath), fmt.Sprintf(".memo-body-orphan-%d", i))
		require.NoError(t, os.WriteFile(path, []byte("unfinished body"), 0o600))
		orphanStages[i] = path
	}
	require.NoError(t, st.Close())
	st = openMemoBodyStore(t, p)
	require.NoError(t, st.SweepMemoBodies(ctx))
	for _, path := range orphanStages {
		require.NoFileExists(t, path)
	}
	require.NoFileExists(t, trashedPath)
	require.FileExists(t, livePath)
	require.FileExists(t, otherPath)
	read, err = st.GetMemo(ctx, &store.FindMemo{ID: &live.ID})
	require.NoError(t, err)
	require.Equal(t, live.Content, read.Content)
}

func TestMemoBodiesWriteThroughMutationAndReadVerifiedFiles(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	p := getTestingProfile(t)
	p.Personal = true
	st := openMemoBodyStore(t, p)
	owner, err := createTestingHostUser(ctx, st)
	require.NoError(t, err)
	memo := &store.Memo{UID: "file-body", CreatorID: owner.ID, Visibility: store.Private, Content: "# 文件\n\n初始正文\n"}
	require.NoError(t, st.ApplyMemoMutation(ctx, &store.MemoMutation{MemoCreate: memo}))
	initialPath := assertMemoBodyFile(t, st, memo.ID, memo.Content)
	comment, err := st.CreateMemoComment(ctx, &store.Memo{UID: "file-comment", CreatorID: owner.ID, Content: "回复\n"}, memo.ID, owner.ID)
	require.NoError(t, err)
	assertMemoBodyFile(t, st, comment.ID, comment.Content)
	require.NoError(t, st.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, Content: new("直接更新\n")}))
	directPath := assertMemoBodyFile(t, st, memo.ID, "直接更新\n")
	empty, err := st.CreateMemo(ctx, &store.Memo{UID: "empty-body", CreatorID: owner.ID, Visibility: store.Private})
	require.NoError(t, err)
	assertMemoBodyFile(t, st, empty.ID, "")
	failedContent := "待保存正文\n"
	failedHash := sha256.Sum256([]byte(failedContent))
	failedPath := memoBodyPath(t, st, hex.EncodeToString(failedHash[:]))
	require.NoError(t, os.WriteFile(failedPath, []byte("损坏文件"), 0o600))
	require.ErrorContains(t, st.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, Content: &failedContent}), "memo body digest does not match")
	assertMemoBodyFile(t, st, memo.ID, "直接更新\n")

	next := "包含检索词的 Markdown\n"
	m := &store.MemoMutation{
		MemoID: memo.ID, MemoCreatorID: owner.ID, ExpectedMemoContent: memo.Content,
		MemoUpdate: &store.UpdateMemo{ID: memo.ID, Content: &next},
	}
	require.ErrorIs(t, st.ApplyMemoMutation(ctx, m), store.ErrMemoMutationConflict)
	read, err := st.GetMemo(ctx, &store.FindMemo{ID: &memo.ID})
	require.NoError(t, err)
	require.Equal(t, "直接更新\n", read.Content)
	m.ExpectedMemoContent = read.Content
	require.NoError(t, st.ApplyMemoMutation(ctx, m))
	current := assertMemoBodyFile(t, st, memo.ID, next)
	search, err := st.ListMemos(ctx, &store.FindMemo{Filters: []string{`content.contains("检索词")`}})
	require.NoError(t, err)
	require.Len(t, search, 1)
	require.Equal(t, next, search[0].Content)
	require.FileExists(t, initialPath)
	require.FileExists(t, directPath)

	require.NoError(t, st.SweepMemoBodies(ctx))
	require.NoFileExists(t, initialPath)
	require.NoFileExists(t, directPath)
	require.NoFileExists(t, failedPath)
	require.NoError(t, os.Remove(current))
	_, err = st.GetMemo(ctx, &store.FindMemo{ID: &memo.ID})
	require.Error(t, err, "missing live files must not fall back to the SQL search projection")
	metadata, err := st.GetMemo(ctx, &store.FindMemo{ID: &memo.ID, ExcludeContent: true})
	require.NoError(t, err)
	require.Empty(t, metadata.Content)

	var issues, repaired []int32
	report := func(id int32, err error, fixed bool) {
		if err != nil {
			issues = append(issues, id)
		}
		if fixed {
			repaired = append(repaired, id)
		}
	}
	require.NoError(t, st.CheckMemoBodies(ctx, false, report))
	require.Equal(t, []int32{memo.ID}, issues)
	require.NoFileExists(t, current)

	issues = nil
	require.NoError(t, st.CheckMemoBodies(ctx, true, report))
	require.Empty(t, issues)
	require.Equal(t, []int32{memo.ID}, repaired)
	assertMemoBodyFile(t, st, memo.ID, next)

	// A damaged file can be replaced when the SQL copy still verifies.
	repaired = nil
	require.NoError(t, os.WriteFile(current, []byte("damaged"), 0o600))
	require.NoError(t, st.CheckMemoBodies(ctx, true, report))
	require.Empty(t, issues)
	require.Equal(t, []int32{memo.ID}, repaired)
	assertMemoBodyFile(t, st, memo.ID, next)

	// A mismatched SQL copy does not invalidate a healthy Markdown file.
	_, err = st.GetDriver().GetDB().ExecContext(ctx, `UPDATE memo SET content = ? WHERE id = ?`, "different copy", memo.ID)
	require.NoError(t, err)
	require.NoError(t, st.CheckMemoBodies(ctx, false, report))
	require.Empty(t, issues)

	// A mismatched SQL copy must never replace the damaged Markdown file.
	require.NoError(t, os.WriteFile(current, []byte("damaged"), 0o600))
	require.NoError(t, st.CheckMemoBodies(ctx, true, report))
	require.Equal(t, []int32{memo.ID}, issues)
	body, err := os.ReadFile(current)
	require.NoError(t, err)
	require.Equal(t, "damaged", string(body))
}

func TestMemoMarkdownPagesKeepTheirOffsetAtByteLimit(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	p := getTestingProfile(t)
	p.Personal = true
	st := openMemoBodyStore(t, p)
	owner, err := createTestingHostUser(ctx, st)
	require.NoError(t, err)
	for index, note := range []struct{ uid, body string }{{"first", "中文字"}, {"oversize", "12345678901"}, {"last", "尾"}} {
		_, err := st.CreateMemo(ctx, &store.Memo{
			UID: note.uid, CreatorID: owner.ID, Visibility: store.Private, Content: note.body, CreatedTs: int64(index + 1),
		})
		require.NoError(t, err)
	}
	offset := 0
	find := &store.FindMemo{CreatorID: &owner.ID, OrderByTimeAsc: true, Offset: &offset}
	page, err := st.ListMemoContentPage(ctx, find, 3, 10)
	require.NoError(t, err)
	require.Len(t, page.Memos, 1)
	require.Equal(t, "中文字", page.Memos[0].Content)
	require.True(t, page.HasMore)
	require.Equal(t, 1, page.Consumed)
	offset += page.Consumed
	page, err = st.ListMemoContentPage(ctx, find, 3, 10)
	require.NoError(t, err)
	require.Equal(t, "oversize", page.OversizedUID)
	require.True(t, page.HasMore)
	offset += page.Consumed
	page, err = st.ListMemoContentPage(ctx, find, 3, 10)
	require.NoError(t, err)
	require.Len(t, page.Memos, 1)
	require.Equal(t, "尾", page.Memos[0].Content)
	require.False(t, page.HasMore)
}
