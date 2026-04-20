package ssh

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_ExecCapturesStdoutAndExitCode(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	c, err := Dial(Config{
		Host:     host,
		Port:     port,
		User:     "test",
		Password: "x",
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Exec(context.Background(), "echo hello && echo err 1>&2")
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Contains(t, res.Stdout, "hello")
	require.Contains(t, res.Stderr, "err")
}

func TestClient_ExecPropagatesNonZeroExit(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	c, err := Dial(Config{Host: host, Port: port, User: "t", Password: "x", Timeout: 2 * time.Second})
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Exec(context.Background(), "exit 7")
	require.NoError(t, err)
	require.Equal(t, 7, res.ExitCode)
}
