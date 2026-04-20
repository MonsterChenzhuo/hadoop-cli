package errs

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_CarriesCodeAndHint(t *testing.T) {
	e := New(CodeSSHAuthFailed, "node2", "pubkey denied")
	var ce *CodedError
	require.True(t, errors.As(e, &ce))
	require.Equal(t, CodeSSHAuthFailed, ce.Code)
	require.Equal(t, "node2", ce.Host)
	require.Contains(t, ce.Error(), "pubkey denied")
	require.NotEmpty(t, ce.Hint())
}

func TestHintRegistry_CoversAllCodes(t *testing.T) {
	for _, c := range AllCodes() {
		require.NotEmptyf(t, HintFor(c), "missing hint for %s", c)
	}
}
