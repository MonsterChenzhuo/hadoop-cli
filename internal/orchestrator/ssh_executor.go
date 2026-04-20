package orchestrator

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/ssh"
)

type SSHExecutor struct {
	Pool *ssh.Pool
}

func (s *SSHExecutor) Execute(ctx context.Context, host string, task Task) Result {
	client, err := s.Pool.Get(host)
	if err != nil {
		return Result{Host: host, OK: false, Err: err}
	}

	for _, f := range task.Files {
		if err := client.Upload(ctx, f.Local, f.Remote, f.Mode); err != nil {
			return Result{Host: host, OK: false, Err: err}
		}
	}
	for _, f := range task.Inline {
		if err := client.WriteFile(ctx, f.Remote, f.Content, f.Mode); err != nil {
			return Result{Host: host, OK: false, Err: err}
		}
	}
	if task.Cmd == "" {
		return Result{Host: host, OK: true}
	}
	exec, err := client.Exec(ctx, task.Cmd)
	if err != nil {
		return Result{Host: host, OK: false, Err: err}
	}
	return Result{
		Host:     host,
		OK:       exec.ExitCode == 0,
		Stdout:   exec.Stdout,
		Stderr:   exec.Stderr,
		ExitCode: exec.ExitCode,
	}
}
