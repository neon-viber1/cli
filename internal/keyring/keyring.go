// Package keyring is a simple wrapper that adds timeouts to the zalando/go-keyring package.
package keyring

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("secret not found in keyring")

// timeout is how long a single keyring call may block. It is long enough for a desktop unlock prompt to be answered.
var timeout = 60 * time.Second

// TimeoutError reports that the system keyring did not answer in time.
type TimeoutError struct {
	message string
}

func (e *TimeoutError) Error() string {
	return e.message
}

// timedOut remembers the first timeout in this process. A keyring that did not answer once is unlikely to answer
// moments later, and a command can make several sequential keyring calls, so every later call fails immediately
// instead of waiting out its own timeout.
var timedOut atomic.Pointer[TimeoutError]

// TimedOut returns the timeout error recorded by this process, or nil if no keyring call has timed out.
func TimedOut() *TimeoutError {
	return timedOut.Load()
}

// resetTimedOut clears the recorded timeout. It exists for tests.
func resetTimedOut() {
	timedOut.Store(nil)
}

// call runs fn with a timeout. The first timeout is remembered and returned by all later calls.
func call[T any](action string, fn func() (T, error)) (T, error) {
	var zero T
	if prev := timedOut.Load(); prev != nil {
		return zero, prev
	}
	type result struct {
		val T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		val, err := fn()
		ch <- result{val, err}
	}()
	select {
	case res := <-ch:
		return res.val, res.err
	case <-time.After(timeout):
		timeoutErr := &TimeoutError{"the system keyring did not respond while trying to " + action +
			"; unlock it or restart the Secret Service"}
		if !timedOut.CompareAndSwap(nil, timeoutErr) {
			return zero, timedOut.Load()
		}
		return zero, timeoutErr
	}
}

// Set secret in keyring for user.
func Set(service, user, secret string) error {
	_, err := call("set a secret in the keyring", func() (struct{}, error) {
		return struct{}{}, keyring.Set(service, user, secret)
	})
	return err
}

// Get secret from keyring given service and user name.
func Get(service, user string) (string, error) {
	val, err := call("get a secret from the keyring", func() (string, error) {
		return keyring.Get(service, user)
	})
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return val, err
}

// Delete secret from keyring.
func Delete(service, user string) error {
	_, err := call("delete a secret from the keyring", func() (struct{}, error) {
		return struct{}{}, keyring.Delete(service, user)
	})
	return err
}

func MockInit() {
	keyring.MockInit()
}

func MockInitWithError(err error) {
	keyring.MockInitWithError(err)
}

// MockTimedOut puts the process into the state after a keyring timeout, so tests can exercise the fail-fast
// behaviour and its messaging without waiting. It restores the previous state when the test ends.
func MockTimedOut(t interface{ Cleanup(func()) }) {
	prev := timedOut.Load()
	timedOut.Store(&TimeoutError{"the system keyring did not respond while trying to get a secret from the keyring; unlock it or restart the Secret Service"})
	t.Cleanup(func() { timedOut.Store(prev) })
}
