package storage

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMemory_PutRawOpenDelete(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	var _ Store = m

	_, _, err := m.Open(ctx, "uploads/j1/m1")
	require.ErrorIs(t, err, ErrObjectNotFound)

	m.PutRaw("uploads/j1/m1", []byte("abc"))
	rc, n, err := m.Open(ctx, "uploads/j1/m1")
	require.NoError(t, err)
	b, _ := io.ReadAll(rc)
	rc.Close()
	require.Equal(t, "abc", string(b))
	require.EqualValues(t, 3, n)

	require.NoError(t, m.Put(ctx, "moments/j1/m1/full.jpg", "image/jpeg", strings.NewReader("zz")))
	require.Equal(t, []string{"moments/j1/m1/full.jpg", "uploads/j1/m1"}, m.Keys())

	require.NoError(t, m.Delete(ctx, "uploads/j1/m1"))
	require.NoError(t, m.Delete(ctx, "uploads/j1/m1"))
	require.Equal(t, []string{"moments/j1/m1/full.jpg"}, m.Keys())

	tgt, err := m.SignedPut(ctx, "uploads/j1/m2", "image/jpeg", 10, time.Minute)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(tgt.URL, "memory://uploads/j1/m2?"))
	require.Equal(t, "PUT", tgt.Method)
	u, err := m.SignedGet(ctx, "moments/j1/m1/full.jpg", time.Hour)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(u, "memory://moments/j1/m1/full.jpg?"))
}

func TestKeys(t *testing.T) {
	require.Equal(t, "uploads/j/m", RawKey("j", "m"))
	require.Equal(t, "moments/j/m/thumb.jpg", MomentKey("j", "m", "thumb.jpg"))
}
