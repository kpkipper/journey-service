package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewToken(t *testing.T) {
	a, err := NewToken()
	require.NoError(t, err)
	b, err := NewToken()
	require.NoError(t, err)
	assert.Len(t, a, 64)
	assert.Len(t, b, 64)
	assert.NotEqual(t, a, b)
}

func TestHashToken(t *testing.T) {
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", HashToken("abc"))
}
