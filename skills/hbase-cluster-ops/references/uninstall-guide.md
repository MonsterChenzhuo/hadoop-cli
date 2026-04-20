# uninstall guide

Default (`hadoop-cli uninstall`): stops all processes and removes
`cluster.install_dir` on every node. HDFS data under `cluster.data_dir` is preserved.

Destructive (`hadoop-cli uninstall --purge-data`): ALSO deletes
`cluster.data_dir`, which wipes HDFS metadata, HBase data on HDFS is
effectively inaccessible after this. Confirm with the user before invoking.

After uninstall you can rerun `install` + `configure` + `start` to rebuild.
