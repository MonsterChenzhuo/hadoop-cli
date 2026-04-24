package packages

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHBaseSpec_DefaultFlavor(t *testing.T) {
	spec, err := HBaseSpec("2.5.8")
	require.NoError(t, err)
	require.Equal(t, "hbase-2.5.8-bin.tar.gz", spec.Filename)
	require.Equal(t, "https://archive.apache.org/dist/hbase/2.5.8/hbase-2.5.8-bin.tar.gz", spec.URL)
	require.Equal(t, spec.URL+".sha512", spec.SHA512URL)
}

func TestHBaseSpec_Hadoop3Variant(t *testing.T) {
	// The `-hadoop3` flavor keeps the base version (2.5.13) as the URL path
	// segment while the filename retains the full variant tag.
	spec, err := HBaseSpec("2.5.13-hadoop3")
	require.NoError(t, err)
	require.Equal(t, "hbase-2.5.13-hadoop3-bin.tar.gz", spec.Filename)
	require.Equal(t, "https://archive.apache.org/dist/hbase/2.5.13/hbase-2.5.13-hadoop3-bin.tar.gz", spec.URL)
}

func TestHBaseSpec_Rejects_Unsupported(t *testing.T) {
	_, err := HBaseSpec("2.4.0")
	require.Error(t, err)
}

func TestHadoopSpec_341(t *testing.T) {
	spec, err := HadoopSpec("3.4.1")
	require.NoError(t, err)
	require.Equal(t, "hadoop-3.4.1.tar.gz", spec.Filename)
	require.Equal(t, "https://archive.apache.org/dist/hadoop/common/hadoop-3.4.1/hadoop-3.4.1.tar.gz", spec.URL)
}
