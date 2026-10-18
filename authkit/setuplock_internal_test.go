// SPDX-License-Identifier: Apache-2.0

//go:build (unix && !aix && !solaris) || illumos

package authkit

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// renewChild names the environment variable that runs the test binary as a second copy renewing an expired code.
const renewChild = "AUTHKIT_SETUP_RENEW_CHILD"

// lockKept reports whether a renewal leaves a lock file beside the code file.
const lockKept = true

// takeLock opens the lock file at path as a renewal does and holds its exclusive lock until the file closes.
func takeLock(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		t.Fatalf("opening the lock file %s: %v", path, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatalf("locking %s: %v", path, err)
	}
	return file
}

// wantLockFree fails t unless another open file takes the exclusive lock on path at once.
func wantLockFree(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening the lock file %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Errorf("taking the lock at %s error = %v, want it free once the renewal ended", path, err)
	}
}

// wantEmptyPrivate fails t unless the lock file at path is an empty regular file at exactly mode 0600.
func wantEmptyPrivate(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("os.Lstat() error = %v, want the lock file %s kept", err, path)
	}
	if info.Mode() != 0o600 || info.Size() != 0 {
		t.Errorf("the lock file is %v holding %d bytes, want an empty regular file at 0600", info.Mode(), info.Size())
	}
}

// readOnlyFolder makes the folder holding path readable but not writable until the test ends.
func readOnlyFolder(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	folder := filepath.Dir(path)
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatalf("os.Chmod() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o700) })
}

// unopenableLock plants at lock an empty file that only its owner can read, skipping as root.
func unopenableLock(t *testing.T, lock string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	if err := os.WriteFile(lock, nil, 0o400); err != nil {
		t.Fatalf("writing %s: %v", lock, err)
	}
}

// linkLock plants at lock a symbolic link to target.
func linkLock(t *testing.T, target, lock string) {
	t.Helper()
	if err := os.Symlink(target, lock); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}
}

// danglingLock plants at lock a link to a missing file in another folder and checks at the end that none was made.
func danglingLock(t *testing.T, lock string) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "elsewhere")
	linkLock(t, target, lock)
	t.Cleanup(func() {
		if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("os.Lstat() of the link's target error = %v, want no file made there", err)
		}
	})
}

// linkedLock plants at lock a link to a file in another folder and checks at the end that it is unlocked and unchanged.
func linkedLock(t *testing.T, lock string) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", target, err)
	}
	before := lstatOf(t, target)
	linkLock(t, target, lock)
	t.Cleanup(func() {
		wantLockFree(t, target)
		wantUnchanged(t, target, before)
	})
}

// wantUnchanged fails t unless path holds the file seen before at the same mode, size and modification time.
func wantUnchanged(t *testing.T, path string, before fs.FileInfo) {
	t.Helper()
	after, err := os.Lstat(path)
	if err != nil || !sameFile(before, after) {
		t.Errorf("os.Lstat() = %v, %v, want the file seen before at %s, unmodified", after, err, path)
		return
	}
	if after.Mode() != before.Mode() || after.Size() != before.Size() {
		t.Errorf("the file at %s is %v holding %d bytes, want %v holding %d", path, after.Mode(), after.Size(),
			before.Mode(), before.Size())
	}
}

// allStacks returns the stack of every goroutine.
func allStacks() string {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return string(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

// onLock reports whether stack is of a goroutine blocked in the select of lockRenewal.
func onLock(stack string) bool {
	header, _, _ := strings.Cut(stack, "\n")
	return strings.Contains(header, " [select") && strings.Contains(stack, "authkit.(*Setup).lockRenewal(")
}

// onLeftFlock reports whether stack is of a goroutine blocked in the select of waitLeftFlock.
func onLeftFlock(stack string) bool {
	header, _, _ := strings.Cut(stack, "\n")
	return strings.Contains(header, " [select") && strings.Contains(stack, "authkit.(*Setup).waitLeftFlock(")
}

// blocked counts, of the goroutines whose stack names marker, those at reports and those on a mutex.
func blocked(marker string, at func(stack string) bool) (atWait, atMutex int) {
	for stack := range strings.SplitSeq(allStacks(), "\n\n") {
		switch {
		case !strings.Contains(stack, marker):
		case at(stack):
			atWait++
		case strings.Contains(stack, "sync.(*Mutex).Lock("):
			atMutex++
		}
	}
	return atWait, atMutex
}

// settledBlocked polls up to waitBound until blocked counts n goroutines whose stack names marker.
func settledBlocked(marker string, at func(stack string) bool, n int) (atWait, atMutex int) {
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	end := time.After(waitBound)
	for {
		select {
		case <-tick.C:
		case <-end:
			return atWait, atMutex
		}
		if atWait, atMutex = blocked(marker, at); atWait+atMutex == n {
			return atWait, atMutex
		}
	}
}

// blocksOnLock reports whether a goroutine whose stack names marker blocks on the lock file before done closes.
func blocksOnLock(marker string, done <-chan struct{}) bool {
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return false
		case <-tick.C:
		}
		if atLock, _ := blocked(marker, onLock); atLock > 0 {
			return true
		}
	}
}

// asRenewChild runs Check of the expired code the environment names as a second copy, reporting whether it did.
func asRenewChild(t *testing.T) bool {
	t.Helper()
	old, path, found := strings.Cut(os.Getenv(renewChild), ":")
	if !found {
		return false
	}
	s := NewSetup(SetupConfig{CodeFile: path, Role: "owner", Logger: slog.New(slog.DiscardHandler)})
	handed := logCodes(s)
	temps := countTemps(s)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		err = s.Check(t.Context(), nil, old)
	}()
	if blocksOnLock("authkit.asRenewChild", done) {
		fmt.Println("waiting")
	}
	<-done
	fmt.Printf("expired %t\ncodes %s\ntemps %d\n", errors.Is(err, ErrSetupCodeExpired), strings.Join(handed.codes, " "),
		temps.Load())
	return true
}

// secondCopy is the test binary run again as a second copy renewing the code.
type secondCopy struct {
	// cmd is the running child.
	cmd *exec.Cmd
	// out reads what the child prints.
	out *bufio.Reader
	// errs keeps what the child writes to its standard error.
	errs *bytes.Buffer
}

// startSecondCopy runs the test of t again as a child that checks the expired code old at the code file of s.
func startSecondCopy(t *testing.T, s *Setup, old string) *secondCopy {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), renewChild+"="+old+":"+s.cfg.CodeFile)
	errs := &bytes.Buffer{}
	cmd.Stderr = errs
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("cmd.StdoutPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the second copy: %v", err)
	}
	return &secondCopy{cmd: cmd, out: bufio.NewReader(out), errs: errs}
}

// line returns the next line the child prints, without its line break.
func (c *secondCopy) line() string {
	text, _ := c.out.ReadString('\n')
	return strings.TrimSuffix(text, "\n")
}

// rest waits for the child to end and returns what else it printed, ending the test when the child failed.
func (c *secondCopy) rest(t *testing.T) string {
	t.Helper()
	text, _ := io.ReadAll(c.out)
	if err := c.cmd.Wait(); err != nil {
		t.Fatalf("the second copy failed: %v\n%s%s", err, text, c.errs)
	}
	return string(text)
}

// TestARenewalWaitsForALockAnotherFileHolds checks that a renewal writes nothing while another file holds the lock.
func TestARenewalWaitsForALockAnotherFileHolds(t *testing.T) {
	t.Parallel()

	s, old := expiredSetup(t)
	held := takeLock(t, lockOf(s.cfg.CodeFile))
	temps := countTemps(s)
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		err = s.Check(t.Context(), nil, old)
	}()

	waited := blocksOnLock(t.Name(), done)
	written := temps.Load()
	if closed := held.Close(); closed != nil {
		t.Fatalf("closing the held lock file: %v", closed)
	}
	<-done

	if !waited || written != 0 {
		t.Errorf("while another file held the lock the renewal waited %t and wrote %d temporary files, want it "+
			"waiting with none", waited, written)
	}
	if !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("Check() once the lock was free error = %v, want ErrSetupCodeExpired", err)
	}
	wantOnlyFreshCode(t, s.cfg.CodeFile, old)
	if written := temps.Load(); written != 1 {
		t.Errorf("the renewal wrote %d temporary files once the lock was free, want one", written)
	}
}

// TestASecondSetupWaitsForTheRenewalAndKeepsItsCode checks a renewal by another Setup that starts while one writes.
func TestASecondSetupWaitsForTheRenewalAndKeepsItsCode(t *testing.T) {
	t.Parallel()

	first, old := expiredSetup(t)
	second := beside(first)
	firstCodes, secondCodes := logCodes(first), logCodes(second)
	secondTemps := countTemps(second)
	done := make(chan struct{})
	var secondErr error
	var waited bool
	onTempFile(first, func() {
		go func() {
			defer close(done)
			secondErr = second.Check(t.Context(), nil, old)
		}()
		waited = blocksOnLock(t.Name(), done)
	})

	err := first.Check(t.Context(), nil, old)
	<-done

	if !errors.Is(err, ErrSetupCodeExpired) || !errors.Is(secondErr, ErrSetupCodeExpired) {
		t.Errorf("the two Check() errors = %v and %v, want ErrSetupCodeExpired twice", err, secondErr)
	}
	if !waited {
		t.Errorf("the second Setup renewed while the first wrote, want it waiting for the lock")
	}
	renewed := wantOnlyFreshCode(t, first.cfg.CodeFile, old)
	for name, got := range map[string][]string{"first": firstCodes.codes, "second": secondCodes.codes} {
		if !slices.Equal(got, []string{renewed}) {
			t.Errorf("OnCode of the %s Setup received %q, want once the code the file holds, %q", name, got, renewed)
		}
	}
	if written := secondTemps.Load(); written != 0 {
		t.Errorf("the second Setup wrote %d temporary files, want none", written)
	}
}

// TestASecondCopyKeepsTheCodeTheFirstRenewed checks a second process renewing the same expired code meanwhile.
func TestASecondCopyKeepsTheCodeTheFirstRenewed(t *testing.T) {
	if asRenewChild(t) {
		return
	}
	t.Parallel()

	s, old := expiredSetup(t)
	handed := logCodes(s)
	var second *secondCopy
	var first string
	onTempFile(s, func() {
		second = startSecondCopy(t, s, old)
		first = second.line()
	})

	err := s.Check(t.Context(), nil, old)

	if !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	if second == nil {
		t.Fatalf("the renewal wrote no temporary file, want it to start the second copy while it held the lock")
	}
	reported := second.rest(t)
	fresh := wantOnlyFreshCode(t, s.cfg.CodeFile, old)
	if first != "waiting" {
		t.Errorf("the second copy printed %q first, want it waiting for the lock while the first renewal wrote", first)
	}
	if want := fmt.Sprintf("expired true\ncodes %s\ntemps 0\n", fresh); !strings.HasPrefix(reported, want) {
		t.Errorf("the second copy reported %q, want %q, the first copy's code and no code of its own", reported, want)
	}
	if !slices.Equal(handed.codes, []string{fresh}) {
		t.Errorf("OnCode of the first copy received %q, want once the code the file holds, %q", handed.codes, fresh)
	}
}

// TestCheckKeepsTheExpiredCodeWhenItCannotOpenTheLock checks a lock path the renewal cannot open.
func TestCheckKeepsTheExpiredCodeWhenItCannotOpenTheLock(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		block func(t *testing.T, lock string)
	}{
		{"a folder at the lock path", plantDir},
		{"a folder the process cannot write", readOnlyFolder},
		{"a file the process cannot open", unopenableLock},
		{"a link to a missing file elsewhere", danglingLock},
		{"a link to a file elsewhere", linkedLock},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, old := expiredSetup(t)
			path, lock := s.cfg.CodeFile, lockOf(s.cfg.CodeFile)
			before := lstatOf(t, path)
			tt.block(t, lock)
			names := folderNames(t, path)
			handed := logCodes(s)
			temps := countTemps(s)

			err := s.Check(t.Context(), nil, old)

			if !strings.Contains(fmt.Sprint(err), "take the setup code lock at "+lock) {
				t.Errorf("Check() error = %v, want the failure naming the lock file %s", err, lock)
			}
			wantUnmoved(t, path, before)
			if held := codeAt(t, path); held != old {
				t.Errorf("the code file holds %q, want the expired %q kept", held, old)
			}
			wantFolderHolds(t, path, names...)
			if written := temps.Load(); written != 0 || len(handed.codes) != 0 {
				t.Errorf("the failed renewal wrote %d temporary files and announced %q, want nothing", written,
					handed.codes)
			}
		})
	}
}

// openFiles returns how many files the process holds open.
func openFiles(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		t.Fatalf("reading /dev/fd: %v", err)
	}
	return len(entries)
}

// TestCheckKeepsTheExpiredCodeWhenTheLockFails checks a renewal whose wait for the lock fails.
func TestCheckKeepsTheExpiredCodeWhenTheLockFails(t *testing.T) {
	failed := errors.New("no lock")
	was := flock
	flock = func(int, int) error { return failed }
	t.Cleanup(func() { flock = was })
	s, old := expiredSetup(t)
	lock := lockOf(s.cfg.CodeFile)
	temps := countTemps(s)
	open := openFiles(t)

	err := s.Check(t.Context(), nil, old)

	if !errors.Is(err, failed) || !strings.Contains(fmt.Sprint(err), "take the setup code lock at "+lock) {
		t.Errorf("Check() error = %v, want the failed lock naming %s", err, lock)
	}
	if held := codeAt(t, s.cfg.CodeFile); held != old {
		t.Errorf("the code file holds %q, want the expired %q kept", held, old)
	}
	if written := temps.Load(); written != 0 {
		t.Errorf("the failed renewal wrote %d temporary files, want none", written)
	}
	if now := openFiles(t); now > open {
		t.Errorf("the failed lock left %d more files open, want none", now-open)
	}
	wantEmptyPrivate(t, lock)
}

// waitBound is how long a lock test waits for a check that should end.
const waitBound = 10 * time.Second

// endsWithin reports whether done closes within limit.
func endsWithin(done <-chan struct{}, limit time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(limit):
		return false
	}
}

// startedByLockRenewal is the line of a goroutine stack that names lockRenewal as the function that started it.
const startedByLockRenewal = "created by github.com/gopherium/gouncer/authkit.(*Setup).lockRenewal in goroutine "

// lockCleanups counts the goroutines lockRenewal started that have not yet ended.
func lockCleanups() int {
	return strings.Count(allStacks(), startedByLockRenewal)
}

// settledCleanups polls until every goroutine lockRenewal started has ended.
func settledCleanups() {
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for lockCleanups() > 0 {
		<-tick.C
	}
}

// cleanupWaits reports whether a goroutine waits in closeAfterFlock for the flock call on its file to answer.
func cleanupWaits() bool {
	for stack := range strings.SplitSeq(allStacks(), "\n\n") {
		header, _, _ := strings.Cut(stack, "\n")
		if strings.Contains(header, " [chan receive") && strings.Contains(stack, "authkit.closeAfterFlock(") {
			return true
		}
	}
	return false
}

// settledCleanupWait polls until a goroutine waits in closeAfterFlock, answering false once waitBound passes.
func settledCleanupWait() bool {
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	end := time.After(waitBound)
	for !cleanupWaits() {
		select {
		case <-tick.C:
		case <-end:
			return false
		}
	}
	return true
}

// endLockWait checks old under end's context with the lock held elsewhere, answering the files left open and the error.
func endLockWait(t *testing.T, s *Setup, old string, end func(t *testing.T) context.Context) (kept int, err error) {
	t.Helper()
	held := takeLock(t, lockOf(s.cfg.CodeFile))
	open := openFiles(t)
	ctx := end(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		err = s.Check(ctx, nil, old)
	}()

	if !endsWithin(done, waitBound) {
		t.Errorf("Check() still waited for the lock %v after its context ended, want it to end with it", waitBound)
	}
	if !settledCleanupWait() {
		t.Errorf("no cleanup waited for the left flock call within %v, want one", waitBound)
	}
	kept = openFiles(t) - open
	if closed := held.Close(); closed != nil {
		t.Fatalf("closing the held lock file: %v", closed)
	}
	<-done
	settledCleanups()
	return kept, err
}

// flockLooks keeps whether the descriptor of each flock call still named the lock file once the call answered.
type flockLooks struct {
	// lock is the path of the lock file.
	lock string
	// mu guards seen.
	mu sync.Mutex
	// seen holds one entry per answered flock call, true when its descriptor named the lock file.
	seen []bool
}

// lookAtFlocks logs for each flock call until t ends whether its descriptor names the lock file of s once it answers.
func lookAtFlocks(t *testing.T, s *Setup) *flockLooks {
	t.Helper()
	looks := &flockLooks{lock: lockOf(s.cfg.CodeFile)}
	was := flock
	flock = func(fd, how int) error {
		err := syscall.Flock(fd, how)
		var after syscall.Stat_t
		looks.add(&after, syscall.Fstat(fd, &after))
		return err
	}
	t.Cleanup(func() { flock = was })
	return looks
}

// add logs whether the file fstat described in after is the lock file, a failed look counting as another file.
func (l *flockLooks) add(after *syscall.Stat_t, looked error) {
	info, err := os.Stat(l.lock)
	same := looked == nil && err == nil && sameInode(after, info.Sys().(*syscall.Stat_t))
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, same)
}

// calls returns what the log holds for each answered flock call, in order.
func (l *flockLooks) calls() []bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.seen)
}

// sameInode reports whether a and b describe the same file.
func sameInode(a, b *syscall.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino
}

// TestACheckWaitingForTheLockEndsWithItsContext checks a Check whose context ends while another file holds the lock.
func TestACheckWaitingForTheLockEndsWithItsContext(t *testing.T) {
	ended := errors.New("the request ended")
	for _, tt := range []struct {
		name string
		ctx  func(t *testing.T) context.Context
		want error
	}{
		{"a deadline that passes", func(t *testing.T) context.Context {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			t.Cleanup(cancel)
			return ctx
		}, context.DeadlineExceeded},
		{"a cancel with a cause", func(t *testing.T) context.Context {
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(ended)
			return ctx
		}, ended},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, old := expiredSetup(t)
			path, lock := s.cfg.CodeFile, lockOf(s.cfg.CodeFile)
			before := lstatOf(t, path)
			temps := countTemps(s)
			looks := lookAtFlocks(t, s)

			kept, err := endLockWait(t, s, old, tt.ctx)

			if !errors.Is(err, tt.want) || !strings.Contains(fmt.Sprint(err), "take the setup code lock at "+lock) {
				t.Errorf("Check() error = %v, want %v naming the lock file %s", err, tt.want, lock)
			}
			wantUnmoved(t, path, before)
			if now := codeAt(t, path); now != old {
				t.Errorf("the code file holds %q, want the expired %q kept", now, old)
			}
			if written := temps.Load(); written != 0 {
				t.Errorf("the ended wait wrote %d temporary files, want none", written)
			}
			if kept != 1 {
				t.Errorf("the ended wait left %d more files open, want the lock file open while its flock call runs", kept)
			}
			if seen := looks.calls(); !slices.Equal(seen, []bool{true}) {
				t.Errorf("the flock calls found the lock file at their descriptor once they answered %v, want one "+
					"that did", seen)
			}
			wantLockFree(t, lock)
		})
	}
}

// endingContext returns a context of t cancelled with a cause for even i and one with a short deadline for odd i.
func endingContext(t *testing.T, i int) context.Context {
	t.Helper()
	if i%2 == 0 {
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(errors.New("the request ended"))
		return ctx
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	t.Cleanup(cancel)
	return ctx
}

// endChecks checks old n times under ending contexts, failing t unless each error names the lock file and its cause.
func endChecks(t *testing.T, s *Setup, old string, n int) {
	t.Helper()
	lock := lockOf(s.cfg.CodeFile)
	for i := range n {
		ctx := endingContext(t, i)
		err := s.Check(ctx, nil, old)
		cause := context.Cause(ctx)
		if !errors.Is(err, cause) || !strings.Contains(fmt.Sprint(err), "take the setup code lock at "+lock) {
			t.Errorf("Check() %d error = %v, want %v naming the lock file %s", i, err, cause, lock)
		}
	}
}

// TestEndedWaitsLeaveOneFlockBehind checks that many waits ending while the lock is held leave one flock call behind.
func TestEndedWaitsLeaveOneFlockBehind(t *testing.T) {
	const waits = 20
	s, old := expiredSetup(t)
	lock := lockOf(s.cfg.CodeFile)
	open := openFiles(t)
	held := takeLock(t, lock)
	holding := openFiles(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		endChecks(t, s, old, waits)
	}()

	ended := endsWithin(done, waitBound)
	started, kept := lockCleanups(), openFiles(t)-holding
	if err := held.Close(); err != nil {
		t.Fatalf("closing the held lock file: %v", err)
	}
	<-done
	settledCleanups()

	if !ended {
		t.Errorf("the %d checks still waited %v after their contexts ended, want each to end with its own", waits,
			waitBound)
	}
	if started > 2 || kept != 1 {
		t.Errorf("%d ended waits left %d goroutines of lockRenewal and %d more files open, want at most two "+
			"goroutines and one file", waits, started, kept)
	}
	if now := openFiles(t); now != open {
		t.Errorf("once the lock was free %d more files stayed open, want none", now-open)
	}
	wantLockFree(t, lock)
	live, cancel := context.WithTimeout(t.Context(), waitBound)
	defer cancel()
	if err := s.Check(live, nil, old); !errors.Is(err, ErrSetupCodeExpired) {
		t.Errorf("a later Check() error = %v, want ErrSetupCodeExpired", err)
	}
	wantOnlyFreshCode(t, s.cfg.CodeFile, old)
	if s.lock.left != nil {
		t.Errorf("the Setup still holds the left flock call's channel after the later renewal waited for it, want none")
	}
}

// renewEach checks old on every Setup at once, frees held once each blocks, and answers where they blocked.
func renewEach(t *testing.T, held *os.File, old string, setups ...*Setup) (atLeft, atMutex int) {
	t.Helper()
	renewed := make(chan error, len(setups))
	for _, s := range setups {
		go func() { renewed <- s.Check(t.Context(), nil, old) }()
	}
	atLeft, atMutex = settledBlocked("authkit.renewEach.func", onLeftFlock, len(setups))
	if err := held.Close(); err != nil {
		t.Fatalf("closing the held lock file: %v", err)
	}
	for range setups {
		if err := <-renewed; !errors.Is(err, ErrSetupCodeExpired) {
			t.Errorf("a live Check() error = %v, want ErrSetupCodeExpired", err)
		}
	}
	settledCleanups()
	return atLeft, atMutex
}

// TestEndedWaitsLeaveOneFlockPerSetup checks that two Setups on one code file each leave one flock call behind.
func TestEndedWaitsLeaveOneFlockPerSetup(t *testing.T) {
	const waits = 5
	a, old := expiredSetup(t)
	b := beside(a)
	lock := lockOf(a.cfg.CodeFile)
	open := openFiles(t)
	held := takeLock(t, lock)
	holding := openFiles(t)

	endChecks(t, a, old, waits)
	endChecks(t, b, old, waits)
	started, kept := lockCleanups(), openFiles(t)-holding
	atLeft, atMutex := renewEach(t, held, old, a, b)

	if started != 4 || kept != 2 {
		t.Errorf("two Setups after %d ended waits each left %d goroutines of lockRenewal and %d more files open, "+
			"want 4 and 2", waits, started, kept)
	}
	if atLeft != 2 || atMutex != 0 {
		t.Errorf("%d live checks blocked on a left flock call and %d on a mutex, want 2 and 0", atLeft, atMutex)
	}
	wantOnlyFreshCode(t, a.cfg.CodeFile, old)
	if now := openFiles(t); now != open {
		t.Errorf("once the lock was free %d more files stayed open, want none", now-open)
	}
	wantLockFree(t, lock)
	if a.lock.left != nil || b.lock.left != nil {
		t.Errorf("a Setup still holds a left flock call's channel after its renewal waited for it, want none")
	}
}

// failFlockOnRelease makes every flock call wait for release to close and then fail with ENOLCK until t ends.
func failFlockOnRelease(t *testing.T, release <-chan struct{}) {
	t.Helper()
	was := flock
	flock = func(int, int) error {
		<-release
		return syscall.ENOLCK
	}
	t.Cleanup(func() { flock = was })
}

// TestAFlockFailingAfterItsWaitEndedClosesTheFile checks that a left flock call that fails closes its file.
func TestAFlockFailingAfterItsWaitEndedClosesTheFile(t *testing.T) {
	release := make(chan struct{})
	failFlockOnRelease(t, release)
	s, old := expiredSetup(t)
	open := openFiles(t)

	endChecks(t, s, old, 1)
	close(release)
	settledCleanups()

	if now := openFiles(t); now != open {
		t.Errorf("once the left flock call failed %d more files stayed open, want none", now-open)
	}
	ctx, cancel := context.WithTimeout(t.Context(), waitBound)
	defer cancel()
	if err := s.Check(ctx, nil, old); !errors.Is(err, syscall.ENOLCK) {
		t.Errorf("a later Check() error = %v, want its own flock call failing with ENOLCK", err)
	}
}

// TestTheLockFileStaysEmptyAndPrivate checks the lock file after a renewal and after the first account closes setup.
func TestTheLockFileStaysEmptyAndPrivate(t *testing.T) {
	s, old := expiredSetup(t)
	lock := lockOf(s.cfg.CodeFile)
	if err := s.Check(t.Context(), nil, old); !errors.Is(err, ErrSetupCodeExpired) {
		t.Fatalf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	wantEmptyPrivate(t, lock)
	wantLockFree(t, lock)

	_, err := s.CreateFirstAccount(t.Context(), &takingSetupStore{}, FirstAccount{
		Code: codeAt(t, s.cfg.CodeFile), Email: "maria@example.com", Name: "Maria Perez",
		Password: "correct horse battery",
	})

	if err != nil {
		t.Fatalf("CreateFirstAccount() error = %v, want nil", err)
	}
	wantFolderHolds(t, s.cfg.CodeFile, filepath.Base(lock))
	wantEmptyPrivate(t, lock)
}

// TestAFailedRenewalFreesTheLock checks that the lock is free once a renewal failed after it took the lock.
func TestAFailedRenewalFreesTheLock(t *testing.T) {
	s, old := expiredSetup(t)
	failTemp(s)

	err := s.Check(t.Context(), nil, old)

	if err == nil || errors.Is(err, ErrSetupCodeExpired) {
		t.Fatalf("Check() error = %v, want the failed renewal", err)
	}
	wantLockFree(t, lockOf(s.cfg.CodeFile))
}

// TestTheLockIsHeldThroughTheReadBack checks that the lock is still held while the renewal reads back its code.
func TestTheLockIsHeldThroughTheReadBack(t *testing.T) {
	s, old := expiredSetup(t)
	lock := lockOf(s.cfg.CodeFile)
	var held error
	atReadBack(s, func(string) {
		file, err := os.Open(lock)
		if err != nil {
			held = err
			return
		}
		defer func() { _ = file.Close() }()
		held = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	})

	if err := s.Check(t.Context(), nil, old); !errors.Is(err, ErrSetupCodeExpired) {
		t.Fatalf("Check() error = %v, want ErrSetupCodeExpired", err)
	}
	if !errors.Is(held, syscall.EWOULDBLOCK) {
		t.Errorf("taking the lock during the read back error = %v, want EWOULDBLOCK while the renewal holds it", held)
	}
}

// TestTheLockIsTakenInsideTheMutex checks that of racing renewals only the one holding the mutex waits for the lock.
func TestTheLockIsTakenInsideTheMutex(t *testing.T) {
	const racers = 8
	s, old := expiredSetup(t)
	held := takeLock(t, lockOf(s.cfg.CodeFile))
	answers := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() { answers[i] = s.Check(t.Context(), nil, old) })
	}

	atLock, atMutex := settledBlocked(t.Name(), onLock, racers)
	if err := held.Close(); err != nil {
		t.Fatalf("closing the held lock file: %v", err)
	}
	wg.Wait()

	if atLock != 1 || atMutex != racers-1 {
		t.Errorf("%d checks blocked on the lock file and %d on the mutex, want one on the lock file and %d on the "+
			"mutex", atLock, atMutex, racers-1)
	}
	for i, err := range answers {
		if !errors.Is(err, ErrSetupCodeExpired) {
			t.Errorf("Check() %d error = %v, want ErrSetupCodeExpired", i, err)
		}
	}
}

// TestTheLeftFlockIsAwaitedInsideTheMutex checks that only the renewal holding the mutex waits for the left flock call.
func TestTheLeftFlockIsAwaitedInsideTheMutex(t *testing.T) {
	const racers = 8
	s, old := expiredSetup(t)
	held := takeLock(t, lockOf(s.cfg.CodeFile))
	endChecks(t, s, old, 1)
	renewed := make(chan error, racers)
	for range racers {
		go func() { renewed <- s.Check(t.Context(), nil, old) }()
	}

	atLeft, atMutex := settledBlocked(t.Name(), onLeftFlock, racers)
	if err := held.Close(); err != nil {
		t.Fatalf("closing the held lock file: %v", err)
	}
	for range racers {
		if err := <-renewed; !errors.Is(err, ErrSetupCodeExpired) {
			t.Errorf("a racing Check() error = %v, want ErrSetupCodeExpired", err)
		}
	}
	settledCleanups()

	if atLeft != 1 || atMutex != racers-1 {
		t.Errorf("%d checks blocked on the left flock call and %d on the mutex, want one on the flock call and %d "+
			"on the mutex", atLeft, atMutex, racers-1)
	}
}
