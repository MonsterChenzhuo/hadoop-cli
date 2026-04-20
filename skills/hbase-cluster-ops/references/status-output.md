# Reading `hadoop-cli status` output

The command emits one JSON envelope:
```json
{
  "command": "status",
  "ok": true,
  "hosts": [
    {"host": "node1", "ok": true, "elapsed_ms": 80, "message": "NameNode HRegionServer HMaster QuorumPeerMain"},
    {"host": "node2", "ok": true, "elapsed_ms": 75, "message": "DataNode HRegionServer QuorumPeerMain"}
  ]
}
```

Healthy cluster, per role:
- `QuorumPeerMain` on every ZooKeeper host.
- `NameNode` on the NameNode host.
- `DataNode` on every DataNode host.
- `HMaster` on the HBase master host(s).
- `HRegionServer` on every RegionServer host.

If one host is missing a process, restart that component:
```bash
hadoop-cli stop --component <name> --inventory cluster.yaml
hadoop-cli start --component <name> --inventory cluster.yaml
```
