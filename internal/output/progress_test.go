package output

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgress_WritesHostPrefixedLine(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, false)
	p.Infof("node1", "extracting hbase-2.5.8-bin.tar.gz … ok (3.2s)")
	require.Contains(t, buf.String(), "[node1] extracting hbase-2.5.8-bin.tar.gz")
}

// TestProgress_NoANSIEscapes pins the current behavior that Progress never
// writes ANSI escape sequences. When color is added later (gated by noColor),
// this test keeps the noColor=true path correct.
func TestProgress_NoANSIEscapes(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, true)
	p.Errorf("node2", "boom")
	require.False(t, strings.Contains(buf.String(), "\x1b["))
}

func TestProgress_ConcurrentWritersSerialized(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, false)
	var wg sync.WaitGroup
	const n = 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p.Infof("n", "line-%d", i)
		}(i)
	}
	wg.Wait()
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, n)
	for _, ln := range lines {
		require.True(t, strings.HasPrefix(ln, "[n] line-"))
	}
}
