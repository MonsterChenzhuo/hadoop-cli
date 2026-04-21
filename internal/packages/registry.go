package packages

import "fmt"

type Spec struct {
	Name      string // hadoop | zookeeper | hbase
	Version   string
	URL       string
	Filename  string
	SHA512    string // optional override; if empty, SHA512URL is fetched at install time
	SHA512URL string // Apache .sha512 sidecar (usually URL + ".sha512")
}

// supportedVersions gates installs to versions we've actually tested.
// Checksums are no longer embedded — they are fetched from the Apache
// sidecar `.sha512` file next to each tarball.
var supportedVersions = map[string]map[string]struct{}{
	"hadoop":    {"3.3.6": {}},
	"zookeeper": {"3.8.4": {}},
	"hbase":     {"2.5.8": {}},
}

func buildSpec(name, version, url, filename string) (Spec, error) {
	if _, ok := supportedVersions[name][version]; !ok {
		return Spec{}, fmt.Errorf("unsupported %s version %s", name, version)
	}
	return Spec{
		Name:      name,
		Version:   version,
		URL:       url,
		Filename:  filename,
		SHA512URL: url + ".sha512",
	}, nil
}

func HadoopSpec(version string) (Spec, error) {
	return buildSpec("hadoop", version,
		fmt.Sprintf("https://archive.apache.org/dist/hadoop/common/hadoop-%s/hadoop-%s.tar.gz", version, version),
		fmt.Sprintf("hadoop-%s.tar.gz", version),
	)
}

func ZooKeeperSpec(version string) (Spec, error) {
	return buildSpec("zookeeper", version,
		fmt.Sprintf("https://archive.apache.org/dist/zookeeper/zookeeper-%s/apache-zookeeper-%s-bin.tar.gz", version, version),
		fmt.Sprintf("apache-zookeeper-%s-bin.tar.gz", version),
	)
}

func HBaseSpec(version string) (Spec, error) {
	return buildSpec("hbase", version,
		fmt.Sprintf("https://archive.apache.org/dist/hbase/%s/hbase-%s-bin.tar.gz", version, version),
		fmt.Sprintf("hbase-%s-bin.tar.gz", version),
	)
}
