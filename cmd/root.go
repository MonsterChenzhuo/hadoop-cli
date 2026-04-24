package cmd

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X ...cmd.Version=<tag>".
var Version = "0.1.0-dev"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "hadoop-cli",
		Short:         "hadoop-cli bootstraps and manages HBase clusters (HDFS + ZooKeeper + HBase).",
		Long:          "hadoop-cli is a single-binary CLI that installs, configures, starts, stops, and uninstalls an HBase cluster (or a standalone ZooKeeper ensemble, via cluster.components) over SSH, driven by a YAML inventory.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("inventory", "", "path to cluster inventory YAML (default: $HADOOPCLI_INVENTORY, ./cluster.yaml, ~/.hadoop-cli/cluster.yaml)")
	root.PersistentFlags().String("log-level", "info", "log level: debug|info|warn|error")
	root.PersistentFlags().Bool("no-color", false, "disable color in stderr progress output")

	root.AddCommand(newPreflightCmd())
	root.AddCommand(newInstallCmd())
	root.AddCommand(newConfigureCmd())
	root.AddCommand(newStartCmd())
	root.AddCommand(newStopCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newUninstallCmd())
	return root
}
