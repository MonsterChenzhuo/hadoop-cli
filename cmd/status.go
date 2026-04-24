package cmd

import (
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: "Report per-host process status for each component.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "status")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			ctx := backgroundCtx(cmd)
			env := rc.envelope("status").WithRunID(rc.Env.Run.ID)
			comps, err := componentsForInv(rc.Inv, component, false, false)
			if err != nil {
				return err
			}
			for _, comp := range comps {
				aggregate(env, comp.Status(ctx, rc.Env))
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(env)
			// status exits 0 even if processes are down; the envelope reports state.
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	return c
}
