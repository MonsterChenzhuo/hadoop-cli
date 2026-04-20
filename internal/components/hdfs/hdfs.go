package hdfs

import (
	"context"
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/components"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/hadoop-cli/hadoop-cli/internal/packages"
)

type HDFS struct{ ForceFormat bool }

func (HDFS) Name() string { return "hdfs" }

func (HDFS) Hosts(inv *inventory.Inventory) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, h := range append(append([]string{}, inv.Roles.NameNode...), inv.Roles.DataNode...) {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			out = append(out, h)
		}
	}
	return out
}

func (c HDFS) allHosts(inv *inventory.Inventory) []string { return c.Hosts(inv) }

func (c HDFS) Install(ctx context.Context, e components.Env) []orchestrator.Result {
	spec, err := packages.HadoopSpec(e.Inv.Versions.Hadoop)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	cache := packages.NewCache(e.Cache)
	local, err := cache.Ensure(spec)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	remoteTarball := fmt.Sprintf("%s/.cache/%s", e.Inv.Cluster.InstallDir, spec.Filename)
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
mkdir -p %s %s %s %s
if [ -x %s/bin/hdfs ]; then exit 0; fi
tar -xzf %s -C %s
mv %s/hadoop-%s/* %s/
rmdir %s/hadoop-%s
`,
		home, NNDir(e.Inv), DNDir(e.Inv), LogDir(e.Inv),
		home,
		remoteTarball, e.Inv.Cluster.InstallDir,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.Hadoop, home,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.Hadoop,
	)
	task := orchestrator.Task{
		Name:  "hdfs-install",
		Cmd:   script,
		Files: []orchestrator.FileXfer{{Local: local, Remote: remoteTarball, Mode: 0o644}},
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), task)
}

func (c HDFS) Configure(ctx context.Context, e components.Env) []orchestrator.Result {
	coreSite, err := RenderCoreSite(e.Inv)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	hdfsSite, err := RenderHDFSSite(e.Inv)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	workers := RenderWorkers(e.Inv)
	envSh := RenderHadoopEnv(e.Inv)
	home := Home(e.Inv)

	inline := []orchestrator.InlineFile{
		{Remote: home + "/etc/hadoop/core-site.xml", Content: []byte(coreSite), Mode: 0o644},
		{Remote: home + "/etc/hadoop/hdfs-site.xml", Content: []byte(hdfsSite), Mode: 0o644},
		{Remote: home + "/etc/hadoop/workers", Content: []byte(workers), Mode: 0o644},
		{Remote: home + "/etc/hadoop/hadoop-env.sh", Content: []byte(envSh), Mode: 0o755},
	}
	task := orchestrator.Task{
		Name:   "hdfs-configure",
		Cmd:    fmt.Sprintf("mkdir -p %s/etc/hadoop", home),
		Inline: inline,
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), task)
}

func (c HDFS) Start(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	nnMarker := NNDir(e.Inv) + "/.formatted"
	forceFlag := ""
	if c.ForceFormat {
		forceFlag = " -force"
	}
	// NameNode: format if needed, then start
	nnScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
export HADOOP_CONF_DIR=%s/etc/hadoop
export HADOOP_LOG_DIR=%s
if [ ! -f %s ] || [ "%s" = " -force" ]; then
  %s/bin/hdfs namenode -format -nonInteractive%s cluster 2>&1 || true
  mkdir -p %s
  touch %s
fi
if ! jps -lm | grep -q NameNode; then
  %s/bin/hdfs --daemon start namenode
fi
`,
		e.Inv.Cluster.JavaHome, home, LogDir(e.Inv),
		nnMarker, forceFlag,
		home, forceFlag,
		NNDir(e.Inv), nnMarker,
		home,
	)
	nnResults := e.Runner.Run(ctx, e.Inv.Roles.NameNode, orchestrator.Task{Name: "hdfs-nn-start", Cmd: nnScript})

	dnScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
export HADOOP_CONF_DIR=%s/etc/hadoop
export HADOOP_LOG_DIR=%s
if ! jps -lm | grep -q DataNode; then
  %s/bin/hdfs --daemon start datanode
fi
`, e.Inv.Cluster.JavaHome, home, LogDir(e.Inv), home)
	dnResults := e.Runner.Run(ctx, e.Inv.Roles.DataNode, orchestrator.Task{Name: "hdfs-dn-start", Cmd: dnScript})

	return append(nnResults, dnResults...)
}

func (c HDFS) Stop(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	dnScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hdfs --daemon stop datanode || true
`, e.Inv.Cluster.JavaHome, home)
	dnResults := e.Runner.Run(ctx, e.Inv.Roles.DataNode, orchestrator.Task{Name: "hdfs-dn-stop", Cmd: dnScript})

	nnScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hdfs --daemon stop namenode || true
`, e.Inv.Cluster.JavaHome, home)
	nnResults := e.Runner.Run(ctx, e.Inv.Roles.NameNode, orchestrator.Task{Name: "hdfs-nn-stop", Cmd: nnScript})

	return append(dnResults, nnResults...)
}

func (c HDFS) Status(ctx context.Context, e components.Env) []orchestrator.Result {
	script := `jps -lm | grep -E 'NameNode|DataNode' || true`
	return e.Runner.Run(ctx, c.allHosts(e.Inv), orchestrator.Task{Name: "hdfs-status", Cmd: script})
}

func (c HDFS) Uninstall(ctx context.Context, e components.Env, purgeData bool) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hdfs --daemon stop datanode || true
%s/bin/hdfs --daemon stop namenode || true
rm -rf %s
`, e.Inv.Cluster.JavaHome, home, home, home)
	if purgeData {
		script += fmt.Sprintf("rm -rf %s %s %s\n", NNDir(e.Inv), DNDir(e.Inv), LogDir(e.Inv))
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), orchestrator.Task{Name: "hdfs-uninstall", Cmd: script})
}

func failAll(hosts []string, err error) []orchestrator.Result {
	out := make([]orchestrator.Result, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, orchestrator.Result{Host: h, OK: false, Err: err})
	}
	return out
}
