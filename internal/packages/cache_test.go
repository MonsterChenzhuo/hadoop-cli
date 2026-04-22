package packages

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestCache_FetchesSHA512FromSidecar(t *testing.T) {
	payload := []byte("fake-tarball-for-sidecar-test")
	sum := sha512.Sum512(payload)
	hexSum := hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/hadoop-3.3.6.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	})
	mux.HandleFunc("/hadoop-3.3.6.tar.gz.sha512", func(w http.ResponseWriter, r *http.Request) {
		// GNU coreutils format — most common.
		fmt.Fprintf(w, "%s  hadoop-3.3.6.tar.gz\n", hexSum)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewCache(t.TempDir())
	spec := Spec{
		Name: "hadoop", Version: "3.3.6",
		URL:       srv.URL + "/hadoop-3.3.6.tar.gz",
		Filename:  "hadoop-3.3.6.tar.gz",
		SHA512URL: srv.URL + "/hadoop-3.3.6.tar.gz.sha512",
	}
	path, err := c.Ensure(spec)
	require.NoError(t, err)
	require.FileExists(t, path)
}

func TestCache_FailsWhenSidecarChecksumDoesNotMatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/x.tgz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("actual-bytes"))
	})
	mux.HandleFunc("/x.tgz.sha512", func(w http.ResponseWriter, r *http.Request) {
		// 128 hex chars, but not the real hash of "actual-bytes".
		fmt.Fprintln(w, strings.Repeat("a", 128)+"  x.tgz")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewCache(t.TempDir())
	_, err := c.Ensure(Spec{
		Name: "x", Version: "1",
		URL:       srv.URL + "/x.tgz",
		Filename:  "x.tgz",
		SHA512URL: srv.URL + "/x.tgz.sha512",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum")
}

func TestParseSHA512_AcceptsMultipleFormats(t *testing.T) {
	want := strings.Repeat("ab", 64) // 128 hex chars

	cases := map[string]string{
		"GNU coreutils": want + "  apache-zookeeper-3.8.4-bin.tar.gz\n",
		"BSD / openssl": "SHA512 (apache-zookeeper-3.8.4-bin.tar.gz) = " + want + "\n",
		"uppercase GNU": strings.ToUpper(want) + "  hadoop-3.3.6.tar.gz\n",
		"gpg print-md": "apache-zookeeper-3.8.4-bin.tar.gz: " +
			strings.ToUpper(want[:32]) + "\n  " +
			strings.ToUpper(want[32:64]) + "\n  " +
			strings.ToUpper(want[64:96]) + "\n  " +
			strings.ToUpper(want[96:]) + "\n",
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			// Inject a space every 4 chars to exercise the gpg-style stripping.
			if name == "gpg print-md" {
				var b strings.Builder
				for i := 0; i < len(body); i += 4 {
					end := i + 4
					if end > len(body) {
						end = len(body)
					}
					b.WriteString(body[i:end])
					b.WriteByte(' ')
				}
				body = b.String()
			}
			got, err := parseSHA512(body)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestParseSHA512_RejectsGarbage(t *testing.T) {
	_, err := parseSHA512("not a checksum file")
	require.Error(t, err)
}
