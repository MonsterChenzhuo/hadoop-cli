package errs

import "fmt"

type Code string

const (
	CodeSSHConnectFailed          Code = "SSH_CONNECT_FAILED"
	CodeSSHAuthFailed             Code = "SSH_AUTH_FAILED"
	CodePreflightJDKMissing       Code = "PREFLIGHT_JDK_MISSING"
	CodePreflightPortBusy         Code = "PREFLIGHT_PORT_BUSY"
	CodePreflightHostUnresolvable Code = "PREFLIGHT_HOSTNAME_UNRESOLVABLE"
	CodePreflightDiskLow          Code = "PREFLIGHT_DISK_LOW"
	CodePreflightClockSkew        Code = "PREFLIGHT_CLOCK_SKEW"
	CodeDownloadFailed            Code = "DOWNLOAD_FAILED"
	CodeDownloadChecksumMismatch  Code = "DOWNLOAD_CHECKSUM_MISMATCH"
	CodeConfigRenderFailed        Code = "CONFIG_RENDER_FAILED"
	CodeRemoteCommandFailed       Code = "REMOTE_COMMAND_FAILED"
	CodeTimeout                   Code = "TIMEOUT"
	CodeInventoryInvalid          Code = "INVENTORY_INVALID"
	CodeComponentNotReady         Code = "COMPONENT_NOT_READY"
)

var hints = map[Code]string{
	CodeSSHConnectFailed:          "verify host reachability (ping / nc) and ssh.port in inventory",
	CodeSSHAuthFailed:             "check ssh.private_key in inventory and run `ssh-copy-id` to the target host",
	CodePreflightJDKMissing:       "install JDK 8 or 11 on the host and set cluster.java_home to a valid path",
	CodePreflightPortBusy:         "free the listed port on the host (lsof -i :<port>) or change overrides to another port",
	CodePreflightHostUnresolvable: "add host entries to /etc/hosts on every node so names resolve consistently",
	CodePreflightDiskLow:          "free space under cluster.data_dir (need at least a few GB)",
	CodePreflightClockSkew:        "enable ntpd / chrony so all nodes are within 30s of each other",
	CodeDownloadFailed:            "check the control machine's outbound network or pre-populate ~/.hadoop-cli/cache",
	CodeDownloadChecksumMismatch:  "delete the cached tarball and rerun; the Apache mirror may be corrupt",
	CodeConfigRenderFailed:        "this is a bug in a template; file an issue with the render error message",
	CodeRemoteCommandFailed:       "inspect the run directory ~/.hadoop-cli/runs/<run-id>/<host>.stderr for the exact remote failure",
	CodeTimeout:                   "the task exceeded its deadline; rerun with --log-level debug and investigate the slow host",
	CodeInventoryInvalid:          "fix cluster.yaml per the message (schema is documented in hbase-cluster-bootstrap skill)",
	CodeComponentNotReady:         "wait for prerequisites (ZK quorum, NN live) or rerun `start` which retries",
}

type CodedError struct {
	Code    Code
	Host    string
	Message string
	Cause   error
}

func (e *CodedError) Error() string {
	if e.Host != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Host, e.Message)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *CodedError) Unwrap() error { return e.Cause }

func (e *CodedError) Hint() string { return HintFor(e.Code) }

func New(code Code, host, message string) *CodedError {
	return &CodedError{Code: code, Host: host, Message: message}
}

func Wrap(code Code, host string, cause error) *CodedError {
	return &CodedError{Code: code, Host: host, Message: cause.Error(), Cause: cause}
}

func HintFor(code Code) string {
	return hints[code]
}

func AllCodes() []Code {
	out := make([]Code, 0, len(hints))
	for k := range hints {
		out = append(out, k)
	}
	return out
}
