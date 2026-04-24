package cmd

import (
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/hadoop-cli/hadoop-cli/internal/preflight"
	"github.com/spf13/cobra"
)

func newPreflightCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "preflight",
		Short: "Run connectivity, JDK, port, disk, and clock checks on every host.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "preflight")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()

			ctx := backgroundCtx(cmd)
			env := rc.envelope("preflight").WithRunID(rc.Env.Run.ID)
			rep, runErr := preflight.Run(ctx, rc.Inv, rc.Runner)
			if rep != nil {
				aggregate(env, rep.Results)
			}
			if runErr != nil {
				host := ""
				if rep != nil {
					host = rep.Host
				}
				env.WithError(output.EnvelopeError{
					Code:    "PREFLIGHT_FAILED",
					Host:    host,
					Message: runErr.Error(),
					Hint:    "fix the failing host per the message and rerun `hadoop-cli preflight`",
				})
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(env)
			if runErr != nil {
				return runErr
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	return c
}
