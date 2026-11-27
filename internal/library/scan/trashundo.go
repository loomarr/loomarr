package scan

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Undo of Remove extra copy (#4476). A removal answers with a token that names the trashed copy:
// the title, the media folder, the file's place under it and the day's trash folder. The token is
// signed with a key this process makes for itself, so the only paths a restore touches are ones a
// removal wrote, and nothing is stored. A restart ends every token; the file stays in the trash.

var (
	// ErrUndoToken is a token this process did not sign for this title.
	ErrUndoToken = errors.New("library: that undo is not valid")
	// ErrTrashGone is a trashed copy that is not there: the trash was emptied, the file was moved
	// out by hand, or its share does not answer.
	ErrTrashGone = errors.New("library: the copy is not in the trash")
	// ErrPathTaken refuses to restore over whatever is at the copy's old place.
	ErrPathTaken = errors.New("library: something is where the copy was")
	// ErrRestore is a copy that could not be moved back: the folder is read-only, or the share
	// refused.
	ErrRestore = errors.New("library: couldn't move the copy back")
)

type trashed struct {
	Item string `json:"i"`
	Root string `json:"r"`
	Rel  string `json:"p"`
	Day  string `json:"d"`
}

// undoSigner signs under the Library's key, made on first use.
func (l *Library) undoSigner(payload string) []byte {
	l.undoKeyMade.Do(func() { _, _ = rand.Read(l.undoKey[:]) }) // rand.Read never fails
	mac := hmac.New(sha256.New, l.undoKey[:])
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func (l *Library) undoToken(t trashed) (string, error) {
	raw, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + base64.RawURLEncoding.EncodeToString(l.undoSigner(payload)), nil
}

// readUndoToken is the trashed copy a token names, if this process signed it for the title id.
func (l *Library) readUndoToken(id, token string) (trashed, error) {
	var t trashed
	payload, sig, ok := strings.Cut(token, ".")
	if !ok {
		return t, ErrUndoToken
	}
	mac, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(mac, l.undoSigner(payload)) {
		return t, ErrUndoToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(raw, &t) != nil || t.Item != id || !filepath.IsLocal(t.Rel) {
		return trashed{}, ErrUndoToken
	}
	if _, err := time.Parse(time.DateOnly, t.Day); err != nil {
		return trashed{}, ErrUndoToken
	}
	return t, nil
}

// RestoreExtraCopy undoes a RemoveExtraCopy of the title id: the file moves from the trash back to
// where it was, and its folder is passed over, so the title has it again once the pass reads it.
// ErrUndoToken, ErrTrashGone, ErrPathTaken or ErrRestore when it stays where it is. It shares the
// removal's lock, so a restore never runs while a removal reads the title.
func (l *Library) RestoreExtraCopy(_ context.Context, id, token string) error {
	t, err := l.readUndoToken(id, token)
	if err != nil {
		return err
	}
	l.removing.Lock()
	defer l.removing.Unlock()
	src := filepath.Join(t.Root, TrashDir, t.Day, t.Rel)
	dst := filepath.Join(t.Root, t.Rel)
	if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
		return ErrTrashGone
	} else if err != nil {
		return fmt.Errorf("%w: %w", ErrRestore, err)
	}
	// A rename replaces what is at its destination, so the place must be seen to be empty.
	if _, err := os.Lstat(dst); err == nil {
		return ErrPathTaken
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %w", ErrRestore, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("%w: %w", ErrRestore, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("%w: %w", ErrRestore, err)
	}
	l.pass([]librarystore.LibraryFilePath{{Root: t.Root, Path: dst}})
	return nil
}
