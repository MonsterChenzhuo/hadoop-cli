package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// captureStderr replaces os.Stderr during fn and returns whatever was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		buf := &bytes.Buffer{}
		_, _ = io.Copy(buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stderr = orig
	return <-done
}

// minimalInventoryYAML is a ZK-only inventory that passes inventory.Validate.
const minimalInventoryYAML = `cluster:
  name: test
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
  components: [zookeeper]
versions:
  zookeeper: 3.8.4
ssh:
  user: hadoop
  private_key: /tmp/id_rsa
hosts:
  - { name: n1, address: 10.0.0.1 }
  - { name: n2, address: 10.0.0.2 }
  - { name: n3, address: 10.0.0.3 }
roles:
  zookeeper: [n1, n2, n3]
`

// newTestCmd returns a cobra.Command with the same persistent flags the root
// installs, so prepare() has something to read --inventory / --no-color from.
func newTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "test"}
	c.Flags().String("inventory", "", "")
	c.Flags().Bool("no-color", false, "")
	return c
}

func TestPrepare_ResolvesInventoryFromCWDAndAnnounces(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HADOOPCLI_INVENTORY", "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte(minimalInventoryYAML), 0o644))

	var ctx *runCtx
	var prepErr error
	stderr := captureStderr(t, func() {
		ctx, prepErr = prepare(newTestCmd(), "status")
	})

	require.NoError(t, prepErr)
	require.NotNil(t, ctx)
	require.Equal(t, filepath.Join(dir, "cluster.yaml"), ctx.InventoryPath)
	require.Contains(t, stderr, "using inventory:")
	require.Contains(t, stderr, "cluster.yaml")
	require.Contains(t, stderr, "(cwd)")
}

func TestPrepare_ExplicitFlagBeatsOtherRungs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte(minimalInventoryYAML), 0o644))

	explicit := filepath.Join(dir, "other.yaml")
	require.NoError(t, os.WriteFile(explicit, []byte(minimalInventoryYAML), 0o644))

	cmd := newTestCmd()
	require.NoError(t, cmd.Flags().Set("inventory", explicit))

	stderr := captureStderr(t, func() {
		ctx, err := prepare(cmd, "status")
		require.NoError(t, err)
		require.Equal(t, explicit, ctx.InventoryPath)
	})
	require.Contains(t, stderr, "(flag)")
}

func TestPrepare_MissingInventoryErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HADOOPCLI_INVENTORY", "")

	_, err := prepare(newTestCmd(), "status")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no inventory found")
}

func TestRootCommand_InventoryFlagHasEmptyDefault(t *testing.T) {
	root := NewRootCmd()
	flag := root.PersistentFlags().Lookup("inventory")
	require.NotNil(t, flag)
	require.Equal(t, "", flag.DefValue, "inventory flag must default to empty so prepare() can tell 'user did not pass one' apart from 'user passed cluster.yaml'")
	require.Contains(t, strings.ToLower(flag.Usage), "hadoopcli_inventory")
}

func TestRunCtx_EnvelopeCarriesInventoryPath(t *testing.T) {
	ctx := &runCtx{InventoryPath: "/tmp/cluster.yaml", Command: "status"}
	env := ctx.envelope("status")
	require.Equal(t, "/tmp/cluster.yaml", env.InventoryPath)
	require.Equal(t, "status", env.Command)
	require.True(t, env.OK)
}
