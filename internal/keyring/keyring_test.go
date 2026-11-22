package keyring

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallFailsFastAfterFirstTimeout(t *testing.T) {
	timeout = 50 * time.Millisecond
	t.Cleanup(func() { timeout = 60 * time.Second; resetTimedOut() })
	resetTimedOut()

	block := make(chan struct{})
	t.Cleanup(func() { close(block) })

	start := time.Now()
	_, err := call("get a secret from the keyring", func() (string, error) {
		<-block
		return "", nil
	})
	var te *TimeoutError
	require.ErrorAs(t, err, &te)
	assert.Contains(t, err.Error(), "the system keyring did not respond")
	assert.NotNil(t, TimedOut())

	for range 3 {
		_, err = call("get a secret from the keyring", func() (string, error) {
			<-block
			return "", nil
		})
		require.ErrorAs(t, err, &te)
	}
	assert.Less(t, time.Since(start), 3*timeout, "later calls must not wait for their own timeout")
}

func TestCallPassesThroughNonTimeoutResults(t *testing.T) {
	resetTimedOut()
	boom := errors.New("boom")
	_, err := call("x", func() (string, error) { return "", boom })
	assert.ErrorIs(t, err, boom)
	assert.Nil(t, TimedOut())
}

func TestGetMapsNotFound(t *testing.T) {
	resetTimedOut()
	MockInit()
	_, err := Get("svc", "nobody")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, Set("svc", "u", "s"))
	v, err := Get("svc", "u")
	require.NoError(t, err)
	assert.Equal(t, "s", v)
	require.NoError(t, Delete("svc", "u"))
}
