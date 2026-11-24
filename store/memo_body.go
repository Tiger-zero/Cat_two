package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// MemoBodyDirectory holds private Markdown files inside the instance data directory.
const MemoBodyDirectory = "notes"

const memoBodyStagePrefix = ".memo-body-"

var errMemoBodyDigestMismatch = errors.New("memo body digest does not match its file")

// LegacyMemoBody carries one SQL body that has not been copied to its Markdown file.
type LegacyMemoBody struct {
	ID      int32
	Content string
}

type memoBodyDriver interface {
	GetMemoBodyNamespace(context.Context) (string, error)
	ListLegacyMemoBodies(context.Context, int32, int) ([]LegacyMemoBody, error)
	SetMemoBodyDigest(context.Context, int32, string, string) (bool, error)
	ListMemoBodyDigests(context.Context) ([]string, error)
	ListMemoBodyReferences(context.Context, int32, int) ([]MemoBodyReference, error)
	GetMemoBodyProjection(context.Context, int32, string) (string, error)
}

func (s *Store) memoBodiesEnabled() bool {
	return s.profile.Personal
}

// MigrateMemoBodies publishes legacy SQL bodies and removes unreferenced old
// versions in this database's namespace. Run it before serving requests and
// with exclusive access to the logical database and its body directory.
func (s *Store) MigrateMemoBodies(ctx context.Context) error {
	if !s.memoBodiesEnabled() {
		return nil
	}
	driver, ok := s.driver.(memoBodyDriver)
	if !ok {
		return errors.New("database driver does not implement memo body migration")
	}
	notes, err := s.openMemoBodyDirectory(ctx, true)
	if err != nil {
		return err
	}
	defer notes.Close()

	var afterID int32
	for {
		bodies, err := driver.ListLegacyMemoBodies(ctx, afterID, 128)
		if err != nil {
			return errors.Wrap(err, "list legacy memo bodies")
		}
		if len(bodies) == 0 {
			break
		}
		for _, body := range bodies {
			if err := ctx.Err(); err != nil {
				return err
			}
			digest, err := publishMemoBody(notes, body.Content)
			if err != nil {
				return errors.Wrapf(err, "write body for memo %d", body.ID)
			}
			changed, err := driver.SetMemoBodyDigest(ctx, body.ID, body.Content, digest)
			if err != nil {
				return errors.Wrapf(err, "link body for memo %d", body.ID)
			}
			if !changed {
				return errors.Errorf("memo %d changed during body migration", body.ID)
			}
			afterID = body.ID
		}
	}
	return sweepMemoBodies(ctx, driver, notes)
}

func (s *Store) stageMemoBody(ctx context.Context, content string) (string, error) {
	if !s.memoBodiesEnabled() {
		return "", nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	notes, err := s.openMemoBodyDirectory(ctx, true)
	if err != nil {
		return "", err
	}
	defer notes.Close()
	return publishMemoBody(notes, content)
}

func (s *Store) hydrateMemoBodies(ctx context.Context, memos []*Memo, budget int) error {
	var notes *os.Root
	defer func() {
		if notes != nil {
			_ = notes.Close()
		}
	}()
	bounded := budget > 0
	for _, memo := range memos {
		if memo.BodyDigest == "" {
			if bounded {
				budget -= len(memo.Content)
				if budget < 0 {
					return ErrMemoContentBudgetExceeded
				}
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if notes == nil {
			var err error
			notes, err = s.openMemoBodyDirectory(ctx, false)
			if err != nil {
				return err
			}
		}
		limit := -1
		if bounded {
			limit = budget
		}
		body, err := readMemoBodyWithinBudget(notes, memo.BodyDigest, limit)
		if err != nil {
			return errors.Wrapf(err, "read body for memo %d", memo.ID)
		}
		memo.Content = string(body)
		if bounded {
			budget -= len(body)
		}
	}
	return nil
}

func (s *Store) bodyNamespace(ctx context.Context) (string, error) {
	s.memoBodyMu.Lock()
	defer s.memoBodyMu.Unlock()
	if s.memoBodyNamespace != "" {
		return s.memoBodyNamespace, nil
	}
	driver, ok := s.driver.(memoBodyDriver)
	if !ok {
		return "", errors.New("database driver does not implement memo bodies")
	}
	namespace, err := driver.GetMemoBodyNamespace(ctx)
	if err != nil {
		return "", errors.Wrap(err, "read memo body namespace")
	}
	if !validMemoBodyHex(namespace, 16) {
		return "", errors.New("invalid memo body namespace")
	}
	s.memoBodyNamespace = namespace
	return namespace, nil
}

func (s *Store) openMemoBodyDirectory(ctx context.Context, create bool) (*os.Root, error) {
	namespace, err := s.bodyNamespace(ctx)
	if err != nil {
		return nil, err
	}
	data, err := os.OpenRoot(s.profile.Data)
	if err != nil {
		return nil, errors.Wrap(err, "open instance data directory")
	}
	defer data.Close()
	if create {
		if err := makeMemoBodyDirectory(data, MemoBodyDirectory); err != nil {
			return nil, err
		}
	}
	notes, err := openMemoBodySubdirectory(data, MemoBodyDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "open memo bodies directory")
	}
	defer notes.Close()
	if create {
		if err := makeMemoBodyDirectory(notes, namespace); err != nil {
			return nil, err
		}
	}
	root, err := openMemoBodySubdirectory(notes, namespace)
	return root, errors.Wrap(err, "open memo body namespace directory")
}

func openMemoBodySubdirectory(parent *os.Root, name string) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("memo body directory must be a real directory")
	}
	return parent.OpenRoot(name)
}

func makeMemoBodyDirectory(parent *os.Root, name string) error {
	if err := parent.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return errors.Wrap(err, "create memo body directory")
	}
	return syncMemoBodyDirectory(parent)
}

func syncMemoBodyDirectory(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return errors.Wrap(err, "open directory for sync")
	}
	defer dir.Close()
	return errors.Wrap(dir.Sync(), "sync memo body directory")
}

func validMemoBodyHex(value string, size int) bool {
	if len(value) != 2*size || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func memoBodyDigest(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func readMemoBody(notes *os.Root, digest string) ([]byte, error) {
	return readMemoBodyWithinBudget(notes, digest, -1)
}

func openMemoBodyFile(notes *os.Root, digest string, limit int) (*os.File, error) {
	if !validMemoBodyHex(digest, sha256.Size) {
		return nil, errors.New("invalid memo body digest")
	}
	name := digest + ".md"
	info, err := notes.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("memo body must be a regular file")
	}
	if limit >= 0 && info.Size() > int64(limit) {
		return nil, ErrMemoContentBudgetExceeded
	}
	return notes.Open(name)
}

func readMemoBodyWithinBudget(notes *os.Root, digest string, limit int) ([]byte, error) {
	file, err := openMemoBodyFile(notes, digest, limit)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var reader io.Reader = file
	if limit >= 0 {
		reader = io.LimitReader(reader, int64(limit)+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if limit >= 0 && len(body) > limit {
		return nil, ErrMemoContentBudgetExceeded
	}
	if memoBodyDigest(body) != digest {
		return nil, errMemoBodyDigestMismatch
	}
	return body, nil
}

func publishMemoBody(notes *os.Root, content string) (string, error) {
	digest := memoBodyDigest([]byte(content))
	if _, err := readMemoBody(notes, digest); err == nil {
		return digest, syncMemoBodyDirectory(notes)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errors.Wrap(err, "verify existing memo body")
	}
	return digest, writeMemoBody(notes, digest, content)
}

func writeMemoBody(notes *os.Root, digest, content string) error {
	// Publish a durable immutable file before any SQL transaction can point to it.
	temp := memoBodyStagePrefix + rand.Text()
	file, err := notes.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.Wrap(err, "stage memo body")
	}
	defer notes.Remove(temp)
	defer file.Close()
	if _, err := io.WriteString(file, content); err != nil {
		return errors.Wrap(err, "write memo body")
	}
	if err := file.Sync(); err != nil {
		return errors.Wrap(err, "sync memo body")
	}
	if err := file.Close(); err != nil {
		return errors.Wrap(err, "close memo body")
	}
	if err := notes.Rename(temp, digest+".md"); err != nil {
		return errors.Wrap(err, "publish memo body")
	}
	return syncMemoBodyDirectory(notes)
}

func sweepMemoBodies(ctx context.Context, driver memoBodyDriver, notes *os.Root) error {
	digests, err := driver.ListMemoBodyDigests(ctx)
	if err != nil {
		return errors.Wrap(err, "list committed memo body files")
	}
	protected := make(map[string]struct{}, len(digests))
	for _, digest := range digests {
		if !validMemoBodyHex(digest, sha256.Size) {
			return errors.New("invalid committed memo body digest")
		}
		protected[digest] = struct{}{}
	}
	dir, err := notes.Open(".")
	if err != nil {
		return errors.Wrap(err, "open memo body directory for cleanup")
	}
	defer dir.Close()
	for {
		entries, readErr := dir.ReadDir(128)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return errors.Wrap(readErr, "list memo body directory")
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.Type().IsRegular() {
				continue
			}
			name := entry.Name()
			digest, markdown := strings.CutSuffix(name, ".md")
			_, referenced := protected[digest]
			if strings.HasPrefix(name, memoBodyStagePrefix) || (markdown && validMemoBodyHex(digest, sha256.Size) && !referenced) {
				if err := notes.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
					return errors.Wrap(err, "remove unreferenced memo body")
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			return syncMemoBodyDirectory(notes)
		}
	}
}
