package packages

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

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

	expected, err := resolveSHA512(s)
	if err != nil {
		return "", err
	}

	if ok, _ := verify(path, expected); ok {
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

	if ok, err := verify(tmp, expected); !ok {
		os.Remove(tmp)
		return "", errs.New(errs.CodeDownloadChecksumMismatch, "",
			fmt.Sprintf("SHA-512 checksum mismatch for %s: %v", s.Filename, err))
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// resolveSHA512 returns the expected tarball checksum. If the caller pinned
// one on the Spec, that wins (useful for tests and for callers who want to
// avoid an extra HTTP round-trip). Otherwise we fetch the `.sha512` sidecar
// Apache publishes next to every release.
func resolveSHA512(s Spec) (string, error) {
	if s.SHA512 != "" {
		return strings.ToLower(s.SHA512), nil
	}
	if s.SHA512URL == "" {
		return "", errs.New(errs.CodeDownloadFailed, "",
			fmt.Sprintf("spec %s %s has neither SHA512 nor SHA512URL", s.Name, s.Version))
	}
	resp, err := http.Get(s.SHA512URL)
	if err != nil {
		return "", errs.Wrap(errs.CodeDownloadFailed, "", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", errs.New(errs.CodeDownloadFailed, "",
			fmt.Sprintf("HTTP %d from %s", resp.StatusCode, s.SHA512URL))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errs.Wrap(errs.CodeDownloadFailed, "", err)
	}
	sum, err := parseSHA512(string(body))
	if err != nil {
		return "", errs.New(errs.CodeDownloadFailed, "",
			fmt.Sprintf("cannot parse %s: %v", s.SHA512URL, err))
	}
	return sum, nil
}

// parseSHA512 extracts a 128-char lowercase hex SHA-512 from the `.sha512`
// file body. Apache projects publish checksums in inconsistent shapes:
//
//	GNU coreutils:  <hex>  <filename>
//	BSD / openssl:  SHA512 (<filename>) = <hex>
//	gpg --print-md: <filename>: <hex grouped in 4-char chunks across lines>
//
// The parser accepts any of them.
func parseSHA512(body string) (string, error) {
	// GNU / BSD: one field in the body is exactly 128 hex chars.
	for _, field := range strings.Fields(body) {
		if len(field) == 128 && isHex(field) {
			return strings.ToLower(field), nil
		}
	}
	// gpg style: strip whitespace, the last 128 chars are the hash.
	var b strings.Builder
	for _, r := range body {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) >= 128 {
		tail := strings.ToLower(s[len(s)-128:])
		if isHex(tail) {
			return tail, nil
		}
	}
	return "", fmt.Errorf("no 128-char SHA-512 hex found")
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
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
	if got != strings.ToLower(expectedHex) {
		return false, fmt.Errorf("expected %s, got %s", expectedHex, got)
	}
	return true, nil
}
