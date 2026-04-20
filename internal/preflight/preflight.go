package preflight

import (
	"context"
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/errs"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

type Report struct {
	OK      bool
	Host    string
	Check   string
	Message string
	Results []orchestrator.Result
}

func Run(ctx context.Context, inv *inventory.Inventory, runner *orchestrator.Runner) (*Report, error) {
	hosts := inv.AllRoleHosts()

	checks := []struct {
		name     string
		cmd      string
		failCode errs.Code
	}{
		{
			name:     "preflight-jdk",
			cmd:      fmt.Sprintf("%s/bin/java -version 2>&1", inv.Cluster.JavaHome),
			failCode: errs.CodePreflightJDKMissing,
		},
		{
			name: "preflight-ports",
			cmd: fmt.Sprintf(`set -e
for p in %d %d %d %d %d %d %d; do
  if (echo > /dev/tcp/127.0.0.1/$p) 2>/dev/null; then echo "PORT_BUSY:$p"; exit 1; fi
done
echo ok
`,
				inv.Overrides.HDFS.NameNodeRPC, inv.Overrides.HDFS.NameNodeHTTP,
				inv.Overrides.ZooKeeper.ClientPort,
				inv.Overrides.HBase.MasterPort, inv.Overrides.HBase.MasterInfoPort,
				inv.Overrides.HBase.RSPort, inv.Overrides.HBase.RSInfoPort),
			failCode: errs.CodePreflightPortBusy,
		},
		{
			name:     "preflight-disk",
			cmd:      fmt.Sprintf("df -h %s 2>/dev/null | awk 'NR==2{print $4}'", inv.Cluster.DataDir),
			failCode: errs.CodePreflightDiskLow,
		},
		{
			name:     "preflight-clock",
			cmd:      `date -u +%s`,
			failCode: errs.CodePreflightClockSkew,
		},
	}

	allResults := []orchestrator.Result{}
	for _, ch := range checks {
		rs := runner.Run(ctx, hosts, orchestrator.Task{Name: ch.name, Cmd: ch.cmd})
		allResults = append(allResults, rs...)
		for _, r := range rs {
			if !r.OK {
				return &Report{OK: false, Host: r.Host, Check: ch.name, Message: strings.TrimSpace(r.Stderr + r.Stdout), Results: allResults},
					errs.New(ch.failCode, r.Host, fmt.Sprintf("%s on %s: %s", ch.name, r.Host, strings.TrimSpace(r.Stderr+r.Stdout)))
			}
		}
	}
	return &Report{OK: true, Results: allResults}, nil
}
