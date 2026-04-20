package output

import (
	"fmt"
	"io"
	"sync"
)

type Progress struct {
	mu      sync.Mutex
	w       io.Writer
	noColor bool
}

func NewProgress(w io.Writer, noColor bool) *Progress {
	return &Progress{w: w, noColor: noColor}
}

func (p *Progress) Infof(host, format string, args ...any) {
	p.writef("", host, format, args...)
}

func (p *Progress) Warnf(host, format string, args ...any) {
	p.writef("WARN ", host, format, args...)
}

func (p *Progress) Errorf(host, format string, args ...any) {
	p.writef("ERROR ", host, format, args...)
}

func (p *Progress) writef(level, host, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	prefix := level
	if host != "" {
		prefix += "[" + host + "] "
	}
	line := prefix + msg + "\n"
	p.mu.Lock()
	defer p.mu.Unlock()
	_, _ = io.WriteString(p.w, line)
}
