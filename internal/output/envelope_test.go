package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvelope_SuccessMarshalsOkTrue(t *testing.T) {
	buf := &bytes.Buffer{}
	env := NewEnvelope("install").WithSummary(map[string]any{
		"hosts_total": 3,
		"hosts_ok":    3,
		"elapsed_ms":  1000,
	})
	env.AddHost(HostResult{Host: "node1", OK: true, ElapsedMs: 333})

	require.NoError(t, env.Write(buf))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, "install", decoded["command"])
	require.Equal(t, true, decoded["ok"])
	require.NotNil(t, decoded["summary"])
	require.Len(t, decoded["hosts"], 1)
}

func TestEnvelope_FailureIncludesError(t *testing.T) {
	buf := &bytes.Buffer{}
	env := NewEnvelope("install").WithError(EnvelopeError{
		Code:    "SSH_AUTH_FAILED",
		Host:    "node2",
		Message: "public key authentication failed",
		Hint:    "check ssh.private_key in inventory",
	})

	require.NoError(t, env.Write(buf))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, false, decoded["ok"])
	errObj := decoded["error"].(map[string]any)
	require.Equal(t, "SSH_AUTH_FAILED", errObj["code"])
	require.Equal(t, "node2", errObj["host"])
}

func TestEnvelope_AddHostFailureFlipsOK(t *testing.T) {
	buf := &bytes.Buffer{}
	env := NewEnvelope("install")
	env.AddHost(HostResult{Host: "node1", OK: true, ElapsedMs: 10})
	env.AddHost(HostResult{Host: "node2", OK: false, ElapsedMs: 20, Message: "ssh timeout"})
	require.NoError(t, env.Write(buf))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, false, decoded["ok"])
	require.Len(t, decoded["hosts"], 2)
}

func TestEnvelope_WithRunIDSerialized(t *testing.T) {
	buf := &bytes.Buffer{}
	env := NewEnvelope("start").WithRunID("20260420-123456-start")
	require.NoError(t, env.Write(buf))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, "20260420-123456-start", decoded["run_id"])
}

func TestEnvelope_OmitsEmptyArraysAndSummary(t *testing.T) {
	buf := &bytes.Buffer{}
	require.NoError(t, NewEnvelope("status").Write(buf))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	_, hasHosts := decoded["hosts"]
	_, hasSummary := decoded["summary"]
	_, hasError := decoded["error"]
	_, hasRunID := decoded["run_id"]
	_, hasInvPath := decoded["inventory_path"]
	require.False(t, hasHosts)
	require.False(t, hasSummary)
	require.False(t, hasError)
	require.False(t, hasRunID)
	require.False(t, hasInvPath)
}

func TestEnvelope_InventoryPathSerialized(t *testing.T) {
	buf := &bytes.Buffer{}
	env := NewEnvelope("status")
	env.InventoryPath = "/tmp/cluster.yaml"
	require.NoError(t, env.Write(buf))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, "/tmp/cluster.yaml", decoded["inventory_path"])
}
