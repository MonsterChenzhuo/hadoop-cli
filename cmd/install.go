package cmd

import (
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "install",
		Short: "Download, distribute, and extract tarballs for each component.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "install")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			ctx := backgroundCtx(cmd)
			env := output.NewEnvelope("install").WithRunID(rc.Env.Run.ID)
			comps, err := componentsForInv(rc.Inv, component, false, false)
			if err != nil {
				return err
			}
			for _, comp := range comps {
				rc.Progress.Infof("", "installing %s ...", comp.Name())
				aggregate(env, comp.Install(ctx, rc.Env))
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
