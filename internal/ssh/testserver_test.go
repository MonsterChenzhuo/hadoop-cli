package ssh

import (
	"io"
	"net"
	"os/exec"
	"testing"

	gssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/require"
)

// startTestServer starts an in-process SSH server that runs each requested
// command with /bin/sh -c. Returns host:port and a stop func.
func startTestServer(t *testing.T) (string, func()) {
	t.Helper()
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &gssh.Server{
		Handler: func(s gssh.Session) {
			cmd := exec.Command("/bin/sh", "-c", s.RawCommand())
			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()
			_ = cmd.Start()
			go io.Copy(s, stdout)
			go io.Copy(s.Stderr(), stderr)
			err := cmd.Wait()
			if exit, ok := err.(*exec.ExitError); ok {
				_ = s.Exit(exit.ExitCode())
				return
			}
			_ = s.Exit(0)
		},
		PasswordHandler: func(gssh.Context, string) bool { return true },
	}
	srv.SetOption(gssh.HostKeyFile("")) // random ephemeral key
	go func() { _ = srv.Serve(lst) }()
	return lst.Addr().String(), func() { _ = srv.Close() }
}
