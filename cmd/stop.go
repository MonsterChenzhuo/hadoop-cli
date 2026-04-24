package cmd

import (
	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "stop",
		Short: "Stop the cluster in reverse order: HBase -> HDFS -> ZK.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "stop")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			ctx := backgroundCtx(cmd)
			env := rc.envelope("stop").WithRunID(rc.Env.Run.ID)
			comps, err := componentsForInv(rc.Inv, component, true, false)
			if err != nil {
				return err
			}
			for _, comp := range comps {
				rc.Progress.Infof("", "stopping %s ...", comp.Name())
				aggregate(env, comp.Stop(ctx, rc.Env))
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
	return c
}
