package cmd

import (
	"github.com/hadoop-cli/hadoop-cli/internal/hbaseops"
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newSnapshotCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "snapshot",
		Short: "Take an online HBase snapshot via hbase shell.",
		Long:  "Connects to an HBase master over SSH and runs `snapshot '<table>','<name>'` in hbase shell. Runs online by default; pass --skip-flush to skip the memstore flush.",
		Example: `  # English: snapshot the users table.
  # 中文: 对 users 表做快照。
  hadoop-cli snapshot --inventory cluster.yaml --table default:users --name users_20260422`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "snapshot")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()

			table, _ := cmd.Flags().GetString("table")
			name, _ := cmd.Flags().GetString("name")
			skipFlush, _ := cmd.Flags().GetBool("skip-flush")
			onHost, _ := cmd.Flags().GetString("on")

			ctx := backgroundCtx(cmd)
			res, err := hbaseops.Snapshot(ctx, rc.Runner, rc.Inv, hbaseops.SnapshotOptions{
				Table:     table,
				Name:      name,
				SkipFlush: skipFlush,
			}, onHost)
			if err != nil {
				return err
			}
			env := output.NewEnvelope("snapshot").WithRunID(rc.Env.Run.ID)
			aggregateOne(env, res)
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(env)
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("table", "", "HBase table name, e.g. namespace:table (required)")
	c.Flags().String("name", "", "snapshot name (required)")
	c.Flags().Bool("skip-flush", false, "do not flush memstore before snapshotting")
	c.Flags().String("on", "", "host to run hbase shell on (default: first hbase_master)")
	_ = c.MarkFlagRequired("table")
	_ = c.MarkFlagRequired("name")
	return c
}
