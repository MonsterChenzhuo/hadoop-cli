package inventory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// EnvVar names the environment variable consulted by Resolve.
const EnvVar = "HADOOPCLI_INVENTORY"

// DefaultFile is the file name looked up in the CWD and under HomeDir.
const DefaultFile = "cluster.yaml"

// HomeDir is the per-user state directory where hadoop-cli parks defaults.
const HomeDir = ".hadoop-cli"

// Resolve returns the inventory path to use. Lookup order:
//  1. flag (if non-empty)
//  2. $HADOOPCLI_INVENTORY
//  3. ./cluster.yaml
//  4. ~/.hadoop-cli/cluster.yaml
//
// The second return value is a short label identifying which rung matched
// ("flag", "env:HADOOPCLI_INVENTORY", "cwd", "home"). When nothing is found,
// the error lists every rung that was tried so users know how to fix it.
func Resolve(flag string) (string, string, error) {
	if flag != "" {
		return flag, "flag", nil
	}

	if env := os.Getenv(EnvVar); env != "" {
		return env, "env:" + EnvVar, nil
	}

	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, DefaultFile)
		if fileExists(candidate) {
			return candidate, "cwd", nil
		}
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		candidate := filepath.Join(home, HomeDir, DefaultFile)
		if fileExists(candidate) {
			return candidate, "home", nil
		}
	}

	return "", "", errors.New("no inventory found; tried --inventory flag, $" + EnvVar +
		", ./" + DefaultFile + ", ~/" + HomeDir + "/" + DefaultFile +
		fmt.Sprintf(" (cwd=%s home=%s)", cwd, home))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
