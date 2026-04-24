package packages

import (
	"fmt"
	"strings"
)

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
//
// HBase 2.5.x ships two flavors of the binary tarball: the default
// (`hbase-<ver>-bin.tar.gz`, Hadoop-2 dependencies) and the Hadoop 3 variant
// (`hbase-<ver>-hadoop3-bin.tar.gz`). The latter is expressed here as
// `<ver>-hadoop3`; HBaseSpec splits on `-` so the URL path segment stays
// `<ver>` while the filename keeps the full variant.
var supportedVersions = map[string]map[string]struct{}{
	"hadoop":    {"3.3.6": {}, "3.4.1": {}},
	"zookeeper": {"3.8.4": {}},
	"hbase":     {"2.5.8": {}, "2.5.13-hadoop3": {}},
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
	// The URL path segment on archive.apache.org is the base release number
	// (e.g. `2.5.13`), not the flavored variant (`2.5.13-hadoop3`).
	base := version
	if i := strings.Index(version, "-"); i > 0 {
		base = version[:i]
	}
	return buildSpec("hbase", version,
		fmt.Sprintf("https://archive.apache.org/dist/hbase/%s/hbase-%s-bin.tar.gz", base, version),
		fmt.Sprintf("hbase-%s-bin.tar.gz", version),
	)
}
