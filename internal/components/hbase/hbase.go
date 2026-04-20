package hbase

import (
	"context"
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/components"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/hadoop-cli/hadoop-cli/internal/packages"
)

type HBase struct{}

func (HBase) Name() string { return "hbase" }

func (HBase) Hosts(inv *inventory.Inventory) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, h := range append(append([]string{}, inv.Roles.HBaseMaster...), inv.Roles.RegionServer...) {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			out = append(out, h)
		}
	}
	return out
}

func (c HBase) allHosts(inv *inventory.Inventory) []string { return c.Hosts(inv) }

func (c HBase) Install(ctx context.Context, e components.Env) []orchestrator.Result {
	spec, err := packages.HBaseSpec(e.Inv.Versions.HBase)
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
mkdir -p %s %s %s
if [ -x %s/bin/hbase ]; then exit 0; fi
tar -xzf %s -C %s
mv %s/hbase-%s/* %s/
rmdir %s/hbase-%s
`,
		home, LogDir(e.Inv), PidDir(e.Inv),
		home,
		remoteTarball, e.Inv.Cluster.InstallDir,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.HBase, home,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.HBase,
	)
	task := orchestrator.Task{
		Name:  "hbase-install",
		Cmd:   script,
		Files: []orchestrator.FileXfer{{Local: local, Remote: remoteTarball, Mode: 0o644}},
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), task)
}

func (c HBase) Configure(ctx context.Context, e components.Env) []orchestrator.Result {
	site, err := RenderHBaseSite(e.Inv)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	rs := RenderRegionServers(e.Inv)
	bm := RenderBackupMasters(e.Inv)
	envSh := RenderHBaseEnv(e.Inv)
	home := Home(e.Inv)

	task := orchestrator.Task{
		Name: "hbase-configure",
		Cmd:  fmt.Sprintf("mkdir -p %s/conf", home),
		Inline: []orchestrator.InlineFile{
			{Remote: home + "/conf/hbase-site.xml", Content: []byte(site), Mode: 0o644},
			{Remote: home + "/conf/regionservers", Content: []byte(rs), Mode: 0o644},
			{Remote: home + "/conf/backup-masters", Content: []byte(bm), Mode: 0o644},
			{Remote: home + "/conf/hbase-env.sh", Content: []byte(envSh), Mode: 0o755},
		},
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), task)
}

func (c HBase) Start(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	masterScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
if ! jps -lm | grep -q HMaster; then
  %s/bin/hbase-daemon.sh start master
fi
`, e.Inv.Cluster.JavaHome, home)
	masterResults := e.Runner.Run(ctx, e.Inv.Roles.HBaseMaster, orchestrator.Task{Name: "hbase-master-start", Cmd: masterScript})

	rsScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
if ! jps -lm | grep -q HRegionServer; then
  %s/bin/hbase-daemon.sh start regionserver
fi
`, e.Inv.Cluster.JavaHome, home)
	rsResults := e.Runner.Run(ctx, e.Inv.Roles.RegionServer, orchestrator.Task{Name: "hbase-rs-start", Cmd: rsScript})

	return append(masterResults, rsResults...)
}

func (c HBase) Stop(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	rsScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hbase-daemon.sh stop regionserver || true
`, e.Inv.Cluster.JavaHome, home)
	rsResults := e.Runner.Run(ctx, e.Inv.Roles.RegionServer, orchestrator.Task{Name: "hbase-rs-stop", Cmd: rsScript})

	masterScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hbase-daemon.sh stop master || true
`, e.Inv.Cluster.JavaHome, home)
	masterResults := e.Runner.Run(ctx, e.Inv.Roles.HBaseMaster, orchestrator.Task{Name: "hbase-master-stop", Cmd: masterScript})

	return append(rsResults, masterResults...)
}

func (c HBase) Status(ctx context.Context, e components.Env) []orchestrator.Result {
	return e.Runner.Run(ctx, c.allHosts(e.Inv), orchestrator.Task{
		Name: "hbase-status",
		Cmd:  `jps -lm | grep -E 'HMaster|HRegionServer' || true`,
	})
}

func (c HBase) Uninstall(ctx context.Context, e components.Env, purgeData bool) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hbase-daemon.sh stop regionserver || true
%s/bin/hbase-daemon.sh stop master || true
rm -rf %s
`, e.Inv.Cluster.JavaHome, home, home, home)
	if purgeData {
		script += fmt.Sprintf("rm -rf %s %s\n", LogDir(e.Inv), PidDir(e.Inv))
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), orchestrator.Task{Name: "hbase-uninstall", Cmd: script})
}

func failAll(hosts []string, err error) []orchestrator.Result {
	out := make([]orchestrator.Result, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, orchestrator.Result{Host: h, OK: false, Err: err})
	}
	return out
}
