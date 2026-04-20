package packages

import (
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCache_DownloadsAndVerifiesChecksum(t *testing.T) {
	payload := []byte("fake-tarball-content")
	sum := sha512.Sum512(payload)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := NewCache(dir)
	spec := Spec{
		Name:     "hbase",
		Version:  "2.5.8",
		URL:      srv.URL + "/hbase-2.5.8-bin.tar.gz",
		Filename: "hbase-2.5.8-bin.tar.gz",
		SHA512:   hexSum,
	}
	path, err := c.Ensure(spec)
	require.NoError(t, err)
	require.FileExists(t, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, payload, data)

	// second call hits cache, no error
	path2, err := c.Ensure(spec)
	require.NoError(t, err)
	require.Equal(t, path, path2)
}

func TestCache_RejectsChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("wrong"))
	}))
	defer srv.Close()

	c := NewCache(t.TempDir())
	_, err := c.Ensure(Spec{
		Name: "x", Version: "1", URL: srv.URL,
		Filename: "x.tgz", SHA512: "deadbeef",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum")
}

func TestCache_PathIsDeterministic(t *testing.T) {
	c := NewCache("/tmp/hc")
	require.Equal(t, filepath.Join("/tmp/hc", "hbase-2.5.8-bin.tar.gz"),
		c.PathFor(Spec{Filename: "hbase-2.5.8-bin.tar.gz"}))
}
