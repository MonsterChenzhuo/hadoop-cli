package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestXMLSite_RendersKeyValues(t *testing.T) {
	props := []Property{
		{Name: "fs.defaultFS", Value: "hdfs://node1:8020"},
		{Name: "dfs.replication", Value: "2"},
	}
	out, err := XMLSite(props)
	require.NoError(t, err)
	require.Contains(t, out, `<name>fs.defaultFS</name>`)
	require.Contains(t, out, `<value>hdfs://node1:8020</value>`)
	require.Contains(t, out, `<name>dfs.replication</name>`)
	require.Contains(t, out, `<value>2</value>`)
}

func TestXMLSite_EscapesSpecialChars(t *testing.T) {
	out, err := XMLSite([]Property{{Name: "k", Value: "<&>"}})
	require.NoError(t, err)
	require.Contains(t, out, "&lt;&amp;&gt;")
}

func TestRenderText_SubstitutesVars(t *testing.T) {
	out, err := RenderText("hello {{.Name}}", map[string]any{"Name": "world"})
	require.NoError(t, err)
	require.Equal(t, "hello world", out)
}
