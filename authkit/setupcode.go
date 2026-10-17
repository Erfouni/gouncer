// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// codeLength is how many characters a setup code holds.
const codeLength = 26

// codeAlphabet is every character a setup code may hold.
const codeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

// maxCodeFileBytes is the most a file holding a setup code may hold.
const maxCodeFileBytes = 64

// sharedBits are the mode bits that let the group or others into a file.
const sharedBits fs.FileMode = 0o077

// heldCode is a setup code read from the code file, with when it expires and whether others could read it.
type heldCode struct {
	// code is the setup code the file holds.
	code string
	// expiresAt is when the code stops working, in UTC.
	expiresAt time.Time
	// burned reports whether the file let the group or others in.
	burned bool
}

// ensureCode answers the live private code at the code file, writing a fresh one when none is there or it went stale.
func (s *Setup) ensureCode(ctx context.Context) (heldCode, error) {
	held, err := s.readCode()
	if errors.Is(err, fs.ErrNotExist) {
		return s.placeCode()
	}
	if err != nil {
		return heldCode{}, err
	}
	if s.stale(held) {
		return s.renewCode(ctx, held)
	}
	return held, nil
}

// stale reports whether a held code expired or others could read it.
func (s *Setup) stale(held heldCode) bool {
	return held.burned || s.expired(held.expiresAt)
}

// expired reports whether a code expiring at expiresAt has stopped working.
func (s *Setup) expired(expiresAt time.Time) bool {
	return !s.now().Before(expiresAt)
}

// renewCode replaces a stale code with a fresh one, warning when others could read the old one.
func (s *Setup) renewCode(ctx context.Context, held heldCode) (heldCode, error) {
	if held.burned {
		s.cfg.Logger.WarnContext(ctx, "setup code was readable by others, so a new one replaced it",
			"path", s.cfg.CodeFile)
	}
	if err := s.unlink(); err != nil {
		return heldCode{}, err
	}
	return s.placeCode()
}

// readCode reads the code file without following a link, refusing what is there unless it is one setup code.
func (s *Setup) readCode() (heldCode, error) {
	seen, err := s.lstat(s.cfg.CodeFile)
	if err != nil {
		return heldCode{}, s.readFailure(err)
	}
	if !seen.Mode().IsRegular() {
		return heldCode{}, fmt.Errorf("%w: %s is not a regular file, so it stays in place", ErrSetupCodeFile,
			s.cfg.CodeFile)
	}
	text, err := s.readSame(seen)
	if err != nil {
		return heldCode{}, err
	}
	if !isCode(text) {
		return heldCode{}, s.notACode()
	}
	return heldCode{
		code:      text,
		expiresAt: seen.ModTime().Add(s.cfg.CodeLifetime).UTC(),
		burned:    seen.Mode().Perm()&sharedBits != 0,
	}, nil
}

// readSame returns the trimmed text of the code file, refusing a file other than the one seen or a long one.
func (s *Setup) readSame(seen fs.FileInfo) (string, error) {
	file, err := s.openFile(s.cfg.CodeFile)
	if err != nil {
		return "", s.readFailure(err)
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(seen, opened) {
		return "", fmt.Errorf("%w: %s changed while it was read, so it stays in place", ErrSetupCodeFile,
			s.cfg.CodeFile)
	}
	if opened.Size() > maxCodeFileBytes {
		return "", s.notACode()
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCodeFileBytes))
	if err != nil {
		return "", s.readFailure(err)
	}
	return strings.TrimSpace(string(data)), nil
}

// isCode reports whether text is one setup code, codeLength characters from codeAlphabet.
func isCode(text string) bool {
	return len(text) == codeLength && strings.Trim(text, codeAlphabet) == ""
}

// codeSeparators drops the spaces, tabs, line breaks and hyphens a person may type inside a code.
var codeSeparators = strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "", "-", "")

// normalizeCode returns input without separators and in capitals.
func normalizeCode(input string) string {
	return strings.ToUpper(codeSeparators.Replace(input))
}

// sameCode reports in constant time whether input and held are the same code.
func sameCode(input, held string) bool {
	given := sha256.Sum256([]byte(input))
	want := sha256.Sum256([]byte(held))
	return subtle.ConstantTimeCompare(given[:], want[:]) == 1
}

// readFailure wraps err as a failure to read the code file.
func (s *Setup) readFailure(err error) error {
	return fmt.Errorf("authkit: read the setup code at %s: %w", s.cfg.CodeFile, err)
}

// notACode reports a code file that holds no setup code and stays in place.
func (s *Setup) notACode() error {
	return fmt.Errorf("%w: %s does not hold a setup code, so it stays in place", ErrSetupCodeFile, s.cfg.CodeFile)
}

// placeCode writes a fresh code beside the code file and links it into place unless another copy did first.
func (s *Setup) placeCode() (heldCode, error) {
	linked := s.linkFresh()
	if linked != nil && !errors.Is(linked, fs.ErrExist) {
		return heldCode{}, fmt.Errorf("authkit: write the setup code at %s: %w", s.cfg.CodeFile, linked)
	}
	held, err := s.readCode()
	if err != nil {
		return heldCode{}, s.failedReadBack(err, linked)
	}
	if held.burned {
		refused := fmt.Errorf("%w: this system cannot keep %s private", ErrSetupCodeFile, s.cfg.CodeFile)
		return heldCode{}, errors.Join(refused, s.unlink())
	}
	return held, nil
}

// failedReadBack answers err, first removing the code file when linked is nil and err is not ErrSetupCodeFile.
func (s *Setup) failedReadBack(err, linked error) error {
	if linked != nil || errors.Is(err, ErrSetupCodeFile) {
		return err
	}
	return errors.Join(err, s.unlink())
}

// linkFresh writes a fresh code to a private temporary file beside the code file and links it there unless one exists.
func (s *Setup) linkFresh() error {
	temp, err := s.createTemp(filepath.Dir(s.cfg.CodeFile), "."+filepath.Base(s.cfg.CodeFile)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp.Name()) }()
	_, written := temp.WriteString(rand.Text() + "\n")
	if err = errors.Join(written, temp.Sync(), temp.Close()); err != nil {
		return err
	}
	return os.Link(temp.Name(), s.cfg.CodeFile)
}

// removeCode deletes the code file when it holds a setup code and leaves anything else at the path in place.
func (s *Setup) removeCode() error {
	_, err := s.readCode()
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrSetupCodeFile) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.unlink()
}

// unlink deletes the code file, a file already gone included.
func (s *Setup) unlink() error {
	if err := os.Remove(s.cfg.CodeFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("authkit: remove the setup code at %s: %w", s.cfg.CodeFile, err)
	}
	return nil
}
