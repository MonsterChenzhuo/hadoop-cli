package zookeeper

import (
	"context"
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/components"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/hadoop-cli/hadoop-cli/internal/packages"
)

type ZooKeeper struct{}

func (ZooKeeper) Name() string { return "zookeeper" }

func (ZooKeeper) Hosts(inv *inventory.Inventory) []string { return inv.Roles.ZooKeeper }

func (ZooKeeper) Install(ctx context.Context, e components.Env) []orchestrator.Result {
	spec, err := packages.ZooKeeperSpec(e.Inv.Versions.ZooKeeper)
	if err != nil {
		return failAll(e.Inv.Roles.ZooKeeper, err)
	}
	cache := packages.NewCache(e.Cache)
	local, err := cache.Ensure(spec)
	if err != nil {
		return failAll(e.Inv.Roles.ZooKeeper, err)
	}
	remoteTarball := fmt.Sprintf("%s/.cache/%s", e.Inv.Cluster.InstallDir, spec.Filename)
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
mkdir -p %s %s %s
if [ -x %s/bin/zkServer.sh ]; then exit 0; fi
tar -xzf %s -C %s
mv %s/apache-zookeeper-%s-bin/* %s/
rmdir %s/apache-zookeeper-%s-bin
`,
		home, DataDir(e.Inv), LogDir(e.Inv),
		home,
		remoteTarball, e.Inv.Cluster.InstallDir,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.ZooKeeper, home,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.ZooKeeper,
	)

	task := orchestrator.Task{
		Name:  "zk-install",
		Cmd:   script,
		Files: []orchestrator.FileXfer{{Local: local, Remote: remoteTarball, Mode: 0o644}},
	}
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, task)
}

func (ZooKeeper) Configure(ctx context.Context, e components.Env) []orchestrator.Result {
	zoo, err := RenderZooCfg(e.Inv)
	if err != nil {
		return failAll(e.Inv.Roles.ZooKeeper, err)
	}
	envSh, err := RenderEnv(e.Inv)
	if err != nil {
		return failAll(e.Inv.Roles.ZooKeeper, err)
	}
	home := Home(e.Inv)
	results := make([]orchestrator.Result, 0, len(e.Inv.Roles.ZooKeeper))
	// per-host because myid differs
	for _, host := range e.Inv.Roles.ZooKeeper {
		id := MyIDFor(e.Inv, host)
		task := orchestrator.Task{
			Name: "zk-configure",
			Cmd: fmt.Sprintf(`set -e
mkdir -p %s/conf %s
`, home, DataDir(e.Inv)),
			Inline: []orchestrator.InlineFile{
				{Remote: home + "/conf/zoo.cfg", Content: []byte(zoo), Mode: 0o644},
				{Remote: home + "/conf/zookeeper-env.sh", Content: []byte(envSh), Mode: 0o644},
				{Remote: DataDir(e.Inv) + "/myid", Content: []byte(fmt.Sprintf("%d\n", id)), Mode: 0o644},
			},
		}
		results = append(results, e.Runner.Run(ctx, []string{host}, task)...)
	}
	return results
}

func (ZooKeeper) Start(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
if %s/bin/zkServer.sh status >/dev/null 2>&1; then exit 0; fi
%s/bin/zkServer.sh start
`, e.Inv.Cluster.JavaHome, home, home)
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, orchestrator.Task{Name: "zk-start", Cmd: script})
}

func (ZooKeeper) Stop(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/zkServer.sh stop || true
`, e.Inv.Cluster.JavaHome, home)
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, orchestrator.Task{Name: "zk-stop", Cmd: script})
}

func (ZooKeeper) Status(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/zkServer.sh status
`, e.Inv.Cluster.JavaHome, home)
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, orchestrator.Task{Name: "zk-status", Cmd: script})
}

func (ZooKeeper) Uninstall(ctx context.Context, e components.Env, purgeData bool) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/zkServer.sh stop || true
rm -rf %s
`, e.Inv.Cluster.JavaHome, home, home)
	if purgeData {
		script += fmt.Sprintf("rm -rf %s\n", DataDir(e.Inv))
	}
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, orchestrator.Task{Name: "zk-uninstall", Cmd: script})
}

func failAll(hosts []string, err error) []orchestrator.Result {
	out := make([]orchestrator.Result, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, orchestrator.Result{Host: h, OK: false, Err: err})
	}
	return out
}
