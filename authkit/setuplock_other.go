// SPDX-License-Identifier: Apache-2.0

//go:build !((unix && !aix && !solaris) || illumos)

package authkit

import "context"

// renewalLock holds nothing on a system where renewals take no lock file.
type renewalLock struct{}

// release frees nothing.
func (renewalLock) release() {}

// lockRenewal takes no lock on this system and answers a call that frees nothing.
func (s *Setup) lockRenewal(context.Context) (func(), error) {
	return s.lock.release, nil
}
