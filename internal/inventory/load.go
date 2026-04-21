package inventory

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return LoadBytes(data)
}

func LoadBytes(data []byte) (*Inventory, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	inv := &Inventory{}
	if err := dec.Decode(inv); err != nil {
		return nil, fmt.Errorf("parse inventory: %w", err)
	}
	applyDefaults(inv)
	return inv, nil
}

func applyDefaults(inv *Inventory) {
	if len(inv.Cluster.Components) == 0 {
		inv.Cluster.Components = []string{"zookeeper", "hdfs", "hbase"}
	} else {
		seen := make(map[string]struct{}, len(inv.Cluster.Components))
		out := inv.Cluster.Components[:0]
		for _, c := range inv.Cluster.Components {
			lc := strings.ToLower(strings.TrimSpace(c))
			if lc == "" {
				continue
			}
			if _, dup := seen[lc]; dup {
				continue
			}
			seen[lc] = struct{}{}
			out = append(out, lc)
		}
		inv.Cluster.Components = out
	}
	if inv.SSH.Port == 0 {
		inv.SSH.Port = 22
	}
	if inv.SSH.Parallelism == 0 {
		inv.SSH.Parallelism = 8
	}
	if inv.Overrides.HDFS.Replication == 0 {
		inv.Overrides.HDFS.Replication = 3
	}
	if inv.Overrides.HDFS.NameNodeHeap == "" {
		inv.Overrides.HDFS.NameNodeHeap = "1g"
	}
	if inv.Overrides.HDFS.DataNodeHeap == "" {
		inv.Overrides.HDFS.DataNodeHeap = "1g"
	}
	if inv.Overrides.HDFS.NameNodeRPC == 0 {
		inv.Overrides.HDFS.NameNodeRPC = 8020
	}
	if inv.Overrides.HDFS.NameNodeHTTP == 0 {
		inv.Overrides.HDFS.NameNodeHTTP = 9870
	}
	if inv.Overrides.ZooKeeper.ClientPort == 0 {
		inv.Overrides.ZooKeeper.ClientPort = 2181
	}
	if inv.Overrides.ZooKeeper.TickTime == 0 {
		inv.Overrides.ZooKeeper.TickTime = 2000
	}
	if inv.Overrides.ZooKeeper.InitLimit == 0 {
		inv.Overrides.ZooKeeper.InitLimit = 10
	}
	if inv.Overrides.ZooKeeper.SyncLimit == 0 {
		inv.Overrides.ZooKeeper.SyncLimit = 5
	}
	if inv.Overrides.HBase.MasterHeap == "" {
		inv.Overrides.HBase.MasterHeap = "1g"
	}
	if inv.Overrides.HBase.RegionServerHeap == "" {
		inv.Overrides.HBase.RegionServerHeap = "1g"
	}
	if inv.Overrides.HBase.MasterPort == 0 {
		inv.Overrides.HBase.MasterPort = 16000
	}
	if inv.Overrides.HBase.MasterInfoPort == 0 {
		inv.Overrides.HBase.MasterInfoPort = 16010
	}
	if inv.Overrides.HBase.RSPort == 0 {
		inv.Overrides.HBase.RSPort = 16020
	}
	if inv.Overrides.HBase.RSInfoPort == 0 {
		inv.Overrides.HBase.RSInfoPort = 16030
	}
}
