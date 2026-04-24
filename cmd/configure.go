package cmd

import (
	"github.com/spf13/cobra"
)

func newConfigureCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "configure",
		Short: "Render and push configuration files for each component.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "configure")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			ctx := backgroundCtx(cmd)
			env := rc.envelope("configure").WithRunID(rc.Env.Run.ID)
			comps, err := componentsForInv(rc.Inv, component, false, false)
			if err != nil {
				return err
			}
			for _, comp := range comps {
				rc.Progress.Infof("", "configuring %s ...", comp.Name())
				aggregate(env, comp.Configure(ctx, rc.Env))
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
