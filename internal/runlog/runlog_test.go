package runlog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_CreatesRunDirAndID(t *testing.T) {
	root := t.TempDir()
	r, err := New(root, "install")
	require.NoError(t, err)
	require.NotEmpty(t, r.ID)
	require.DirExists(t, r.Dir)
	require.True(t, filepath.IsAbs(r.Dir))
}

func TestWriteFile_StoresUnderRun(t *testing.T) {
	r, err := New(t.TempDir(), "install")
	require.NoError(t, err)
	require.NoError(t, r.WriteFile("hosts/node1.stdout", []byte("ok")))
	b, err := os.ReadFile(filepath.Join(r.Dir, "hosts/node1.stdout"))
	require.NoError(t, err)
	require.Equal(t, "ok", string(b))
}

func TestSaveResult_WritesJSON(t *testing.T) {
	r, err := New(t.TempDir(), "install")
	require.NoError(t, err)
	require.NoError(t, r.SaveResult(map[string]any{"ok": true, "hosts": 3}))
	b, err := os.ReadFile(filepath.Join(r.Dir, "result.json"))
	require.NoError(t, err)
	require.Contains(t, string(b), `"ok": true`)
}
