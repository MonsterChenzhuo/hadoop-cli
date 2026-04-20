# Bootstrap runbook

1. `hadoop-cli preflight --inventory cluster.yaml`
   - On `PREFLIGHT_JDK_MISSING`: install JDK on the listed host and fix `cluster.java_home`.
   - On `PREFLIGHT_PORT_BUSY`: free the port, or change `overrides.*.port` in inventory.
   - On `PREFLIGHT_HOSTNAME_UNRESOLVABLE`: sync `/etc/hosts` across nodes.
2. `hadoop-cli install --inventory cluster.yaml`
3. `hadoop-cli configure --inventory cluster.yaml`
4. `hadoop-cli start --inventory cluster.yaml`
5. `hadoop-cli status --inventory cluster.yaml`

Re-running any step is safe. If a step fails, inspect
`~/.hadoop-cli/runs/<run-id>/<host>.stderr` before retrying.

## First-run NameNode format

The first `start` formats the NameNode. You never need `--force-format` unless
you intentionally want to wipe HDFS metadata.

## Scope boundaries

- HA is not supported.
- JDK / `/etc/hosts` / system user are NOT managed by hadoop-cli (user must prepare).
- Only Linux / macOS target nodes.
