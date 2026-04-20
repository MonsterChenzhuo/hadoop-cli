package packages

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hadoop-cli/hadoop-cli/internal/errs"
)

type Cache struct {
	Dir string
}

func NewCache(dir string) *Cache {
	return &Cache{Dir: dir}
}

func DefaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hadoop-cli", "cache")
}

func (c *Cache) PathFor(s Spec) string {
	return filepath.Join(c.Dir, s.Filename)
}

func (c *Cache) Ensure(s Spec) (string, error) {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return "", err
	}
	path := c.PathFor(s)
	if ok, _ := verify(path, s.SHA512); ok {
		return path, nil
	}

	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	resp, err := http.Get(s.URL)
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return "", errs.Wrap(errs.CodeDownloadFailed, "", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		f.Close()
		os.Remove(tmp)
		return "", errs.New(errs.CodeDownloadFailed, "", fmt.Sprintf("HTTP %d from %s", resp.StatusCode, s.URL))
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", errs.Wrap(errs.CodeDownloadFailed, "", err)
	}
	f.Close()

	if ok, err := verify(tmp, s.SHA512); !ok {
		os.Remove(tmp)
		return "", errs.New(errs.CodeDownloadChecksumMismatch, "",
			fmt.Sprintf("SHA-512 checksum mismatch for %s: %v", s.Filename, err))
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

func verify(path, expectedHex string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedHex {
		return false, fmt.Errorf("expected %s, got %s", expectedHex, got)
	}
	return true, nil
}
