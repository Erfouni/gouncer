// SPDX-License-Identifier: Apache-2.0

//go:build (unix && !aix && !solaris) || illumos

package authkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockSuffix names the lock file beside the code file, the code file's path with .lock added.
const lockSuffix = ".lock"

// lockMode is the mode of the lock file beside the code file.
const lockMode = 0o600

// flock applies or removes an advisory lock on the open file fd.
var flock = syscall.Flock

// renewalLock is what the renewals of one Setup share of the lock file beside the code file.
type renewalLock struct {
	// left closes once the flock call an ended wait left behind answers and its file closes, nil when none is left.
	left chan struct{}
}

// lockRenewal waits until ctx ends for the private lock beside the code file, refusing a link, and answers its release.
func (s *Setup) lockRenewal(ctx context.Context) (func(), error) {
	path := s.cfg.CodeFile + lockSuffix
	if err := s.waitLeftFlock(ctx); err != nil {
		return nil, lockFailure(path, err)
	}
	lock, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, lockMode)
	if err != nil {
		return nil, lockFailure(path, err)
	}
	_ = lock.Chmod(lockMode)
	taken := make(chan error, 1)
	go flockInto(taken, flock, int(lock.Fd()))
	select {
	case err := <-taken:
		if err != nil {
			return nil, errors.Join(lockFailure(path, err), lock.Close())
		}
		return func() { _ = lock.Close() }, nil
	case <-ctx.Done():
		left := make(chan struct{})
		s.lock.left = left
		go closeAfterFlock(lock, taken, left)
		return nil, lockFailure(path, context.Cause(ctx))
	}
}

// waitLeftFlock waits until ctx ends for the flock call an ended wait left behind to answer and its file to close.
func (s *Setup) waitLeftFlock(ctx context.Context) error {
	if s.lock.left == nil {
		return nil
	}
	select {
	case <-s.lock.left:
		s.lock.left = nil
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// flockInto sends on taken what apply answers for the exclusive lock on the open file fd.
func flockInto(taken chan<- error, apply func(fd, how int) error, fd int) {
	taken <- apply(fd, syscall.LOCK_EX)
}

// closeAfterFlock closes lock once the flock call on it answers on taken, then closes left.
func closeAfterFlock(lock *os.File, taken <-chan error, left chan<- struct{}) {
	<-taken
	_ = lock.Close()
	close(left)
}

// lockFailure wraps err as a failure to take the lock file at path.
func lockFailure(path string, err error) error {
	return fmt.Errorf("authkit: take the setup code lock at %s: %w", path, err)
}
