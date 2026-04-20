package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Run struct {
	ID  string
	Dir string
}

func DefaultRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hadoop-cli", "runs")
}

func New(root, command string) (*Run, error) {
	id := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), command)
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Run{ID: id, Dir: dir}, nil
}

func (r *Run) WriteFile(rel string, data []byte) error {
	full := filepath.Join(r.Dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func (r *Run) SaveResult(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return r.WriteFile("result.json", b)
}
