# Start / stop ordering

- **start**: zookeeper → hdfs → hbase
- **stop**: hbase → hdfs → zookeeper

Single-component ops are allowed via `--component`, but follow the ordering:

- Restarting `hbase` alone is safe.
- Restarting `hdfs` alone is safe **only if** HBase is stopped first.
- Restarting `zookeeper` alone is safe **only if** HBase is stopped first
  (HDFS does not depend on ZooKeeper in v1, so HDFS can stay up).
