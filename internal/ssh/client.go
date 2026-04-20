package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	xssh "golang.org/x/crypto/ssh"
)

type Config struct {
	Host       string
	Port       int
	User       string
	PrivateKey string
	Password   string // only used in tests
	Timeout    time.Duration
}

type Client struct {
	conn *xssh.Client
	cfg  Config
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func Dial(cfg Config) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}

	auths := []xssh.AuthMethod{}
	if cfg.PrivateKey != "" {
		path := cfg.PrivateKey
		if strings.HasPrefix(path, "~") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[1:])
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read private key %s: %w", path, err)
		}
		signer, err := xssh.ParsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		auths = append(auths, xssh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		auths = append(auths, xssh.Password(cfg.Password))
	}

	clientCfg := &xssh.ClientConfig{
		User:            cfg.User,
		Auth:            auths,
		HostKeyCallback: xssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.Timeout,
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := xssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, cfg: cfg}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) Exec(ctx context.Context, cmd string) (*ExecResult, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()

	select {
	case <-ctx.Done():
		_ = sess.Signal(xssh.SIGKILL)
		return nil, ctx.Err()
	case err := <-done:
		exit := 0
		if err != nil {
			var xerr *xssh.ExitError
			if errors.As(err, &xerr) {
				exit = xerr.ExitStatus()
			} else {
				return &ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}, err
			}
		}
		return &ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exit}, nil
	}
}

func (c *Client) Upload(ctx context.Context, local, remote string, mode os.FileMode) error {
	sc, err := sftp.NewClient(c.conn)
	if err != nil {
		return err
	}
	defer sc.Close()

	if err := sc.MkdirAll(filepath.Dir(remote)); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(remote), err)
	}
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := sc.Create(remote)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return sc.Chmod(remote, mode)
}

func (c *Client) WriteFile(ctx context.Context, remote string, content []byte, mode os.FileMode) error {
	sc, err := sftp.NewClient(c.conn)
	if err != nil {
		return err
	}
	defer sc.Close()

	if err := sc.MkdirAll(filepath.Dir(remote)); err != nil {
		return err
	}
	dst, err := sc.Create(remote)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := dst.Write(content); err != nil {
		return err
	}
	return sc.Chmod(remote, mode)
}
