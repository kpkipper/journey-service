package logger

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestInitToHonorsWriter(t *testing.T) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	var buf bytes.Buffer
	InitTo(&buf, false)
	Get().Info().Msg("hello")
	os.Stdout = old
	w.Close()
	out, _ := io.ReadAll(r)
	if !bytes.Contains(buf.Bytes(), []byte("hello")) {
		t.Fatalf("buffer missing log: %q", buf.String())
	}
	if len(out) != 0 {
		t.Fatalf("stdout got output: %q", out)
	}
	Init(false) // restore default
}
