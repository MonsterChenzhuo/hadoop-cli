package components

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNames_AreStableOrdered(t *testing.T) {
	require.Equal(t, []string{"zookeeper", "hdfs", "hbase"}, Ordered())
	require.Equal(t, []string{"hbase", "hdfs", "zookeeper"}, ReverseOrdered())
}
