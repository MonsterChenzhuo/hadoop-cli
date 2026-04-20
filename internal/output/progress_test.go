package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgress_WritesHostPrefixedLine(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, false)
	p.Infof("node1", "extracting hbase-2.5.8-bin.tar.gz … ok (3.2s)")
	require.Contains(t, buf.String(), "[node1] extracting hbase-2.5.8-bin.tar.gz")
}

func TestProgress_ColorDisabled(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, true)
	p.Errorf("node2", "boom")
	require.False(t, strings.Contains(buf.String(), "\x1b["))
}
