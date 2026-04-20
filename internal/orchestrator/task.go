package orchestrator

import (
	"os"
	"time"
)

type FileXfer struct {
	Local  string
	Remote string
	Mode   os.FileMode
}

type InlineFile struct {
	Remote  string
	Content []byte
	Mode    os.FileMode
}

type Task struct {
	Name    string
	Cmd     string
	Files   []FileXfer
	Inline  []InlineFile
	Timeout time.Duration
}

type Result struct {
	Host     string
	OK       bool
	Stdout   string
	Stderr   string
	ExitCode int
	Elapsed  time.Duration
	Err      error
}
