# Error codes → remediation

Every non-zero exit emits a JSON error object:
```json
{"command":"<name>","ok":false,"error":{"code":"<CODE>","host":"<host>","message":"...","hint":"..."}}
```

| Code                              | Typical fix |
|-----------------------------------|-------------|
| SSH_CONNECT_FAILED                | Verify the host is reachable and `ssh.port` is correct. |
| SSH_AUTH_FAILED                   | Fix `ssh.private_key`, run `ssh-copy-id`. |
| PREFLIGHT_JDK_MISSING             | Install JDK 8/11, set `cluster.java_home` to its path. |
| PREFLIGHT_PORT_BUSY               | Free the port or change `overrides.*.port`. |
| PREFLIGHT_HOSTNAME_UNRESOLVABLE   | Sync `/etc/hosts` across all nodes. |
| PREFLIGHT_DISK_LOW                | Free space under `cluster.data_dir`. |
| PREFLIGHT_CLOCK_SKEW              | Enable ntpd/chrony. |
| DOWNLOAD_FAILED                   | Check outbound network or pre-populate `~/.hadoop-cli/cache/`. |
| DOWNLOAD_CHECKSUM_MISMATCH        | Delete the cached tarball; rerun. |
| CONFIG_RENDER_FAILED              | Bug in a template — file an issue. |
| REMOTE_COMMAND_FAILED             | Inspect `~/.hadoop-cli/runs/<run-id>/<host>.stderr`. |
| TIMEOUT                           | Rerun with `--log-level debug`; investigate the slow host. |
| INVENTORY_INVALID                 | Fix `cluster.yaml` per the message. |
| COMPONENT_NOT_READY               | Wait (ZK quorum, NN live) or rerun `start`. |
