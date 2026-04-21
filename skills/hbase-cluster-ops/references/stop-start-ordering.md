# Start / stop ordering

- **start**: zookeeper → hdfs → hbase
- **stop**: hbase → hdfs → zookeeper

The CLI only acts on components declared in `cluster.components`, so a
ZooKeeper-only or HDFS-only inventory will simply skip the rest.

Single-component ops are allowed via `--component`, but follow the ordering:

- Restarting `hbase` alone is safe.
- Restarting `hdfs` alone is safe **only if** HBase is stopped first.
- Restarting `zookeeper` alone is safe **only if** HBase is stopped first
  (HDFS does not depend on ZooKeeper in v1, so HDFS can stay up).
- `--component <name>` must be one of the components declared in
  `cluster.components`; otherwise the CLI errors out with the declared set.
