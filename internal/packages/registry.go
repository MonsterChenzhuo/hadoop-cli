package packages

import "fmt"

type Spec struct {
	Name     string // hadoop | zookeeper | hbase
	Version  string
	URL      string
	Filename string
	SHA512   string // hex-encoded
}

var builtinChecksums = map[string]map[string]string{
	"hadoop": {
		"3.3.6": "PUT_REAL_SHA512_HERE_AT_IMPLEMENTATION_TIME",
	},
	"zookeeper": {
		"3.8.4": "PUT_REAL_SHA512_HERE_AT_IMPLEMENTATION_TIME",
	},
	"hbase": {
		"2.5.8": "PUT_REAL_SHA512_HERE_AT_IMPLEMENTATION_TIME",
	},
}

func HadoopSpec(version string) (Spec, error) {
	sum, ok := builtinChecksums["hadoop"][version]
	if !ok {
		return Spec{}, fmt.Errorf("no checksum registered for hadoop %s", version)
	}
	return Spec{
		Name:     "hadoop",
		Version:  version,
		URL:      fmt.Sprintf("https://archive.apache.org/dist/hadoop/common/hadoop-%s/hadoop-%s.tar.gz", version, version),
		Filename: fmt.Sprintf("hadoop-%s.tar.gz", version),
		SHA512:   sum,
	}, nil
}

func ZooKeeperSpec(version string) (Spec, error) {
	sum, ok := builtinChecksums["zookeeper"][version]
	if !ok {
		return Spec{}, fmt.Errorf("no checksum registered for zookeeper %s", version)
	}
	return Spec{
		Name:     "zookeeper",
		Version:  version,
		URL:      fmt.Sprintf("https://archive.apache.org/dist/zookeeper/zookeeper-%s/apache-zookeeper-%s-bin.tar.gz", version, version),
		Filename: fmt.Sprintf("apache-zookeeper-%s-bin.tar.gz", version),
		SHA512:   sum,
	}, nil
}

func HBaseSpec(version string) (Spec, error) {
	sum, ok := builtinChecksums["hbase"][version]
	if !ok {
		return Spec{}, fmt.Errorf("no checksum registered for hbase %s", version)
	}
	return Spec{
		Name:     "hbase",
		Version:  version,
		URL:      fmt.Sprintf("https://archive.apache.org/dist/hbase/%s/hbase-%s-bin.tar.gz", version, version),
		Filename: fmt.Sprintf("hbase-%s-bin.tar.gz", version),
		SHA512:   sum,
	}, nil
}
