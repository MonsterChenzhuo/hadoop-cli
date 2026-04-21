package inventory

type Cluster struct {
	Name       string   `yaml:"name"`
	InstallDir string   `yaml:"install_dir"`
	DataDir    string   `yaml:"data_dir"`
	User       string   `yaml:"user"`
	JavaHome   string   `yaml:"java_home"`
	Components []string `yaml:"components"`
}

type Versions struct {
	Hadoop    string `yaml:"hadoop"`
	ZooKeeper string `yaml:"zookeeper"`
	HBase     string `yaml:"hbase"`
}

type SSH struct {
	Port        int    `yaml:"port"`
	User        string `yaml:"user"`
	PrivateKey  string `yaml:"private_key"`
	Parallelism int    `yaml:"parallelism"`
	Sudo        bool   `yaml:"sudo"`
}

type Host struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
}

type Roles struct {
	NameNode     []string `yaml:"namenode"`
	DataNode     []string `yaml:"datanode"`
	ZooKeeper    []string `yaml:"zookeeper"`
	HBaseMaster  []string `yaml:"hbase_master"`
	RegionServer []string `yaml:"regionserver"`
}

type HDFSOverrides struct {
	Replication  int    `yaml:"replication"`
	NameNodeHeap string `yaml:"namenode_heap"`
	DataNodeHeap string `yaml:"datanode_heap"`
	NameNodeRPC  int    `yaml:"namenode_rpc_port"`
	NameNodeHTTP int    `yaml:"namenode_http_port"`
}

type ZKOverrides struct {
	ClientPort int `yaml:"client_port"`
	TickTime   int `yaml:"tick_time"`
	InitLimit  int `yaml:"init_limit"`
	SyncLimit  int `yaml:"sync_limit"`
}

type HBaseOverrides struct {
	MasterHeap       string `yaml:"master_heap"`
	RegionServerHeap string `yaml:"regionserver_heap"`
	RootDir          string `yaml:"root_dir"`
	MasterPort       int    `yaml:"master_port"`
	MasterInfoPort   int    `yaml:"master_info_port"`
	RSPort           int    `yaml:"regionserver_port"`
	RSInfoPort       int    `yaml:"regionserver_info_port"`
}

type Overrides struct {
	HDFS      HDFSOverrides  `yaml:"hdfs"`
	ZooKeeper ZKOverrides    `yaml:"zookeeper"`
	HBase     HBaseOverrides `yaml:"hbase"`
}

type Inventory struct {
	Cluster   Cluster   `yaml:"cluster"`
	Versions  Versions  `yaml:"versions"`
	SSH       SSH       `yaml:"ssh"`
	Hosts     []Host    `yaml:"hosts"`
	Roles     Roles     `yaml:"roles"`
	Overrides Overrides `yaml:"overrides"`
}

// HasComponent reports whether the named component is part of this cluster.
// Names are matched case-insensitively.
func (i *Inventory) HasComponent(name string) bool {
	for _, c := range i.Cluster.Components {
		if c == name {
			return true
		}
	}
	return false
}

func (i *Inventory) HostByName(name string) (Host, bool) {
	for _, h := range i.Hosts {
		if h.Name == name {
			return h, true
		}
	}
	return Host{}, false
}

func (i *Inventory) AllRoleHosts() []string {
	seen := map[string]struct{}{}
	add := func(xs []string) {
		for _, x := range xs {
			seen[x] = struct{}{}
		}
	}
	add(i.Roles.NameNode)
	add(i.Roles.DataNode)
	add(i.Roles.ZooKeeper)
	add(i.Roles.HBaseMaster)
	add(i.Roles.RegionServer)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}
