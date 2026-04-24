package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolve_FlagWins(t *testing.T) {
	t.Setenv("HADOOPCLI_INVENTORY", "/env/path.yaml")
	t.Setenv("HOME", t.TempDir())

	path, src, err := Resolve("explicit.yaml")
	require.NoError(t, err)
	require.Equal(t, "explicit.yaml", path)
	require.Equal(t, "flag", src)
}

func TestResolve_EnvOverridesCWDAndHome(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte("x"), 0o644))
	t.Setenv("HADOOPCLI_INVENTORY", "/env/path.yaml")

	path, src, err := Resolve("")
	require.NoError(t, err)
	require.Equal(t, "/env/path.yaml", path)
	require.Equal(t, "env:HADOOPCLI_INVENTORY", src)
}

func TestResolve_CWDBeatsHome(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(cwd)

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".hadoop-cli"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".hadoop-cli", "cluster.yaml"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "cluster.yaml"), []byte("x"), 0o644))

	path, src, err := Resolve("")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(cwd, "cluster.yaml"), path)
	require.Equal(t, "cwd", src)
}

func TestResolve_HomeFallback(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(cwd)

	want := filepath.Join(home, ".hadoop-cli", "cluster.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(want), 0o755))
	require.NoError(t, os.WriteFile(want, []byte("x"), 0o644))

	path, src, err := Resolve("")
	require.NoError(t, err)
	require.Equal(t, want, path)
	require.Equal(t, "home", src)
}

func TestResolve_NothingFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("HADOOPCLI_INVENTORY", "")

	_, _, err := Resolve("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no inventory found")
	require.Contains(t, err.Error(), "HADOOPCLI_INVENTORY")
	require.Contains(t, err.Error(), "./cluster.yaml")
	require.Contains(t, err.Error(), "~/.hadoop-cli/cluster.yaml")
}
