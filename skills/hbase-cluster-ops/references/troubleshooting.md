# Troubleshooting by symptom

| Symptom                                       | Likely cause                             | Fix                                                                 |
|-----------------------------------------------|------------------------------------------|---------------------------------------------------------------------|
| `status` shows no HMaster                     | HMaster crashed                          | `hadoop-cli stop --component hbase && start --component hbase`; read master log in `$data_dir/hbase/logs/hbase-*-master-*.log`. |
| DataNode missing on one host                  | Disk full or datanode log shows errors   | Check `PREFLIGHT_DISK_LOW`; inspect `$data_dir/hdfs/logs/hadoop-*-datanode-*.log`. |
| ZK quorum not forming                         | Ports blocked, myid mismatch             | Check firewall on 2888/3888; rerun `configure` then `start`.        |
| `install` hangs                               | Slow mirror                              | Pre-download tarballs into `~/.hadoop-cli/cache/`, rerun install.   |
| "Cannot create directory" on any remote step  | `cluster.user` lacks permission          | Grant ownership of `install_dir` / `data_dir`, or set `ssh.sudo: true`. |

For every non-trivial failure: read `~/.hadoop-cli/runs/<run-id>/<host>.stderr`.
