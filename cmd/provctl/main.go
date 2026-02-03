// Package main provides the provctl CLI tool for cluster provisioning
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/vjranagit/cluster-api/pkg/api"
	"github.com/vjranagit/cluster-api/pkg/engine"
	"github.com/vjranagit/cluster-api/pkg/providers/aws"
	"github.com/vjranagit/cluster-api/pkg/providers/azure"
	"github.com/vjranagit/cluster-api/pkg/state"
)

var (
	cfgFile     string
	provider    string
	region      string
	statePath   string
	logger      *slog.Logger
)

func main() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	rootCmd := &cobra.Command{
		Use:   "provctl",
		Short: "Multi-cloud Kubernetes cluster provisioning tool",
		Long: `provctl is a declarative infrastructure provisioner for Kubernetes clusters
across AWS, Azure, and other cloud providers.`,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.provctl.yaml)")
	rootCmd.PersistentFlags().StringVar(&statePath, "state", "./state.db", "path to state database")

	rootCmd.AddCommand(createCmd())
	rootCmd.AddCommand(applyCmd())
	rootCmd.AddCommand(deleteCmd())
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func createCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [cluster-name]",
		Short: "Create a new cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			return createCluster(clusterName)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "aws", "cloud provider (aws, azure)")
	cmd.Flags().StringVar(&region, "region", "us-west-2", "cloud region")

	return cmd
}

func applyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply [config-file]",
		Short: "Apply configuration from HCL file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configFile := args[0]
			return applyConfig(configFile)
		},
	}
}

func deleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete [cluster-name]",
		Short: "Delete a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			return deleteCluster(clusterName)
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listClusters()
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("provctl version 1.0.0")
			fmt.Println("Multi-cloud Kubernetes provisioner")
		},
	}
}

func createCluster(name string) error {
	ctx := context.Background()

	// Initialize state manager
	sm, err := state.NewSQLiteStateManager(statePath)
	if err != nil {
		return fmt.Errorf("failed to create state manager: %w", err)
	}
	defer sm.Close()

	// Initialize engine
	eng := engine.NewEngine(sm, nil)

	// Register providers
	switch provider {
	case "aws":
		awsProvider, err := aws.NewProvider(ctx, region, logger)
		if err != nil {
			return fmt.Errorf("failed to create AWS provider: %w", err)
		}
		eng.RegisterProvider(awsProvider)
	case "azure":
		// Azure requires subscription ID - would come from config
		azureProvider, err := azure.NewProvider(ctx, "subscription-id", region, logger)
		if err != nil {
			return fmt.Errorf("failed to create Azure provider: %w", err)
		}
		eng.RegisterProvider(azureProvider)
	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	// Create cluster spec
	spec := api.ClusterSpec{
		Provider: provider,
		Region:   region,
		Network: api.NetworkSpec{
			VPCCIDR:           "10.0.0.0/16",
			AvailabilityZones: []string{region + "a", region + "b"},
		},
		ControlPlane: api.ControlPlaneSpec{
			Type:    api.ControlPlaneManaged,
			Version: "1.28",
		},
		Config: map[string]interface{}{
			"name": name,
		},
	}

	// Create cluster
	cloudProvider := eng.GetProvider(provider)
	cluster, err := cloudProvider.CreateCluster(ctx, spec)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	logger.Info("cluster created successfully",
		"id", cluster.ID,
		"name", cluster.Metadata.Name,
		"provider", provider,
	)

	return nil
}

func applyConfig(configFile string) error {
	logger.Info("applying configuration", "file", configFile)
	// TODO: Parse HCL config and apply
	return fmt.Errorf("not implemented yet")
}

func deleteCluster(name string) error {
	logger.Info("deleting cluster", "name", name)
	// TODO: Implement cluster deletion
	return fmt.Errorf("not implemented yet")
}

func listClusters() error {
	ctx := context.Background()

	sm, err := state.NewSQLiteStateManager(statePath)
	if err != nil {
		return fmt.Errorf("failed to create state manager: %w", err)
	}
	defer sm.Close()

	state, err := sm.GetState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get state: %w", err)
	}

	fmt.Println("Clusters:")
	for id, cluster := range state.Clusters {
		fmt.Printf("  - %s (%s) - %s - %s\n",
			cluster.Metadata.Name,
			id,
			cluster.Spec.Provider,
			cluster.Status.Phase,
		)
	}

	return nil
}
