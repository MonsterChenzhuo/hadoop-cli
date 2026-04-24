package cmd

import (
	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "start",
		Short: "Start the cluster in dependency order: ZK -> HDFS -> HBase.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "start")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()

			component, _ := cmd.Flags().GetString("component")
			forceFormat, _ := cmd.Flags().GetBool("force-format")
			ctx := backgroundCtx(cmd)

			env := rc.envelope("start").WithRunID(rc.Env.Run.ID)
			comps, err := componentsForInv(rc.Inv, component, false, forceFormat)
			if err != nil {
				return err
			}
			for _, comp := range comps {
				rc.Progress.Infof("", "starting %s ...", comp.Name())
				res := comp.Start(ctx, rc.Env)
				aggregate(env, res)
				if !allOK(res) {
					break
				}
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
	c.Flags().Bool("force-format", false, "force NameNode re-format (DESTRUCTIVE; wipes HDFS metadata)")
	return c
}
