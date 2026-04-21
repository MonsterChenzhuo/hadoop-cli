package cmd

import (
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newUninstallCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop services and remove installed bits; optionally purge data dirs.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "uninstall")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			purge, _ := cmd.Flags().GetBool("purge-data")
			ctx := backgroundCtx(cmd)
			env := output.NewEnvelope("uninstall").WithRunID(rc.Env.Run.ID)
			comps, err := componentsForInv(rc.Inv, component, true, false)
			if err != nil {
				return err
			}
			for _, comp := range comps {
				rc.Progress.Infof("", "uninstalling %s (purge_data=%v) ...", comp.Name(), purge)
				aggregate(env, comp.Uninstall(ctx, rc.Env, purge))
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(env)
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	c.Flags().Bool("purge-data", false, "also remove data directories (DESTRUCTIVE)")
	return c
}
