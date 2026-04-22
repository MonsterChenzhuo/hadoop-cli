package cmd

import (
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/hbaseops"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newExportSnapshotCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "export-snapshot",
		Short: "Copy an HBase snapshot to a remote HDFS via hbase ExportSnapshot.",
		Long:  "Runs `hbase org.apache.hadoop.hbase.snapshot.ExportSnapshot` on an HBase master. Without YARN the job falls back to LocalJobRunner; tune with --mappers / --bandwidth for larger snapshots.",
		Example: `  # English: export with a literal HDFS URL.
  # 中文: 直接用 HDFS URL 同步。
  hadoop-cli export-snapshot --inventory cluster.yaml \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

  # English: derive the URL from the destination cluster.yaml.
  # 中文: 从目标集群 inventory 推导 URL。
  hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, _ := cmd.Flags().GetString("name")
			to, _ := cmd.Flags().GetString("to")
			toInv, _ := cmd.Flags().GetString("to-inventory")
			bandwidth, _ := cmd.Flags().GetInt("bandwidth")
			overwrite, _ := cmd.Flags().GetBool("overwrite")
			extra, _ := cmd.Flags().GetString("extra-args")
			onHost, _ := cmd.Flags().GetString("on")

			switch {
			case to != "" && toInv != "":
				return fmt.Errorf("--to and --to-inventory are mutually exclusive")
			case to == "" && toInv == "":
				return fmt.Errorf("one of --to or --to-inventory is required")
			}

			rc, err := prepare(cmd, "export-snapshot")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()

			opts := hbaseops.ExportOptions{
				Name:      name,
				Bandwidth: bandwidth,
				Overwrite: overwrite,
				ExtraArgs: extra,
			}
			if cmd.Flags().Changed("mappers") {
				m, _ := cmd.Flags().GetInt("mappers")
				opts.Mappers = &m
			}

			if to != "" {
				opts.CopyTo = to
			} else {
				dst, err := inventory.Load(toInv)
				if err != nil {
					return fmt.Errorf("load --to-inventory: %w", err)
				}
				for _, nn := range dst.Roles.NameNode {
					if _, ok := dst.HostByName(nn); !ok {
						return fmt.Errorf("--to-inventory: roles.namenode references unknown host %q (declare it under hosts:)", nn)
					}
				}
				url, err := hbaseops.DeriveCopyToFromInventory(dst)
				if err != nil {
					return err
				}
				opts.CopyTo = url
			}

			ctx := backgroundCtx(cmd)
			res, err := hbaseops.ExportSnapshot(ctx, rc.Runner, rc.Inv, opts, onHost)
			if err != nil {
				return err
			}
			env := output.NewEnvelope("export-snapshot").WithRunID(rc.Env.Run.ID)
			aggregateOne(env, res)
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(env)
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("name", "", "snapshot name (required)")
	c.Flags().String("to", "", "destination HDFS URL, e.g. hdfs://nn:8020/hbase")
	c.Flags().String("to-inventory", "", "path to a destination cluster.yaml; derives hdfs://<nn>:<rpc>/hbase")
	c.Flags().Int("mappers", 0, "number of mappers (pass 0 for LocalJobRunner); unset = HBase default")
	c.Flags().Int("bandwidth", 0, "per-mapper bandwidth limit in MB/s (0 = unlimited)")
	c.Flags().Bool("overwrite", false, "overwrite destination snapshot if it already exists")
	c.Flags().String("extra-args", "", "raw args appended to the hbase ExportSnapshot command")
	c.Flags().String("on", "", "host to run on (default: first hbase_master)")
	_ = c.MarkFlagRequired("name")
	return c
}
