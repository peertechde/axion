package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"peertech.de/axion/api/client"
	"peertech.de/axion/pkg/config"
	"peertech.de/axion/pkg/manifest"
	manifeststarlark "peertech.de/axion/pkg/manifest/starlark"
	manifestyaml "peertech.de/axion/pkg/manifest/yaml"
	"peertech.de/axion/pkg/orchestrator"
)

var endpoint string
var configFile string
var concurrency int
var manifestFile string

func main() {
	rootCmd := &cobra.Command{
		Use:           "axionctl",
		Short:         "Declarative configuration manager",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	rootCmd.PersistentFlags().StringVar(&endpoint, "endpoint", "http://localhost:8080",
		"API endpoint (e.g., https://localhost:8080)")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "",
		"Path to optional YAML configuration file")
	rootCmd.PersistentFlags().IntVar(&concurrency, "concurrency", 1,
		"Maximum number of resources to process concurrently (default: 1 for sequential processing)")

	rootCmd.AddCommand(cmdPlan())
	rootCmd.AddCommand(cmdApply())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", prettifyError(err))
		os.Exit(1)
	}
}

func cmdPlan() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview configuration changes without applying them",
		Long: `Plan evaluates the manifest against the current system state and shows
what changes would be made without actually applying them.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			cfg, err := setupConfig(false, "", concurrency, endpoint)
			if err != nil {
				return err
			}

			o, err := setupOrchestrator(cfg, manifestFile)
			if err != nil {
				return err
			}

			summary := o.Run(ctx, true)
			if summary.Error != nil {
				return summary.Error
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestFile, "manifest", "",
		"Path to YAML manifest file containing resource definitions (required)")
	cmd.MarkFlagRequired("manifest")

	return cmd
}

func cmdApply() *cobra.Command {
	var (
		enableBackups bool
		backupDir     string
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply the configuration to the target system",
		Long: `Apply evaluates the manifest and makes the necessary changes to bring
the system to the desired state defined in the manifest.

WARNING: This command makes actual changes to your system.
Always run 'plan' first to review changes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			cfg, err := setupConfig(enableBackups, backupDir, concurrency, endpoint)
			if err != nil {
				return err
			}

			o, err := setupOrchestrator(cfg, manifestFile)
			if err != nil {
				return err
			}

			summary := o.Run(ctx, false)
			if summary.Error != nil {
				return summary.Error
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&enableBackups, "enable-backups", false,
		"Enable automatic backups before applying changes to resources\n"+
			"\n"+
			"Resources that don't support backups will be processed normally but without\n"+
			"backup protection. This includes:\n"+
			"  - File resources: Full file content backup\n"+
			"  - Other resource types may have limited or no backup support\n"+
			"\n"+
			"Backups enable automatic rollback if subsequent resources fail during apply.\n"+
			"Highly recommended for production environments.")
	cmd.Flags().StringVar(&backupDir, "backup-dir", config.DefaultBackupDir(),
		"Directory to store backups (only used when --enable-backups is set)\n"+
			"Defaults to $AXION_BACKUP_DIR or ~/.config/axion/backups\n"+
			"Directory will be created if it doesn't exist")
	cmd.Flags().StringVar(&manifestFile, "manifest", "",
		"Path to YAML manifest file containing resource definitions (required)")
	cmd.MarkFlagRequired("manifest")

	return cmd
}

func setupConfig(enableBackups bool, backupDir string, concurrency int, endpoint string) (*config.Config, error) {
	cfg := &config.Config{
		Concurrency: concurrency,
	}

	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Overrides file config
	if enableBackups {
		cfg.EnableBackups = true
	}

	if backupDir != "" {
		cfg.BackupDir = backupDir
	} else if cfg.BackupDir == "" {
		cfg.BackupDir = config.DefaultBackupDir()
	}

	// Validate backup directory
	if cfg.EnableBackups {
		if err := config.ValidateBackupDir(cfg.BackupDir); err != nil {
			return nil, fmt.Errorf("invalid backup directory: %w", err)
		}
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}

	host := u.Host
	if host == "" {
		return nil, fmt.Errorf("invalid endpoint: missing host in %q", endpoint)
	}

	cfg.Client = client.NewHTTPClientWithConfig(nil, &client.TransportConfig{
		Host:     host,
		BasePath: "/api/v1",
		Schemes:  []string{scheme},
	})

	return cfg, nil
}

func setupOrchestrator(cfg *config.Config, manifestFile string) (*orchestrator.Orchestrator, error) {
	opts := []orchestrator.Option{}
	if cfg.EnableBackups {
		opts = append(opts, orchestrator.WithEnableBackups())
	}
	if cfg.Concurrency > 1 {
		opts = append(opts, orchestrator.WithConcurrency(cfg.Concurrency))
	}
	o := orchestrator.NewOrchestrator(opts...)

	var resources []orchestrator.ResourceSpec
	var err error

	var loader manifest.Loader

	switch strings.ToLower(filepath.Ext(manifestFile)) {
	case ".yaml", ".yml":
		loader = &manifestyaml.Loader{}
	case ".json":
	case ".star":
		loader = &manifeststarlark.Loader{}
	default:
		return nil, fmt.Errorf("unsupported manifest file extension: %s", manifestFile)
	}

	resources, err = loader.Load(context.Background(), cfg, manifestFile)
	if err != nil {
		return nil, err
	}

	for _, r := range resources {
		if err := o.Add(r); err != nil {
			return nil, fmt.Errorf("failed to add resource %q: %w", r.Resource.Name(), err)
		}
	}

	return o, nil
}

func prettifyError(err error) string {
	// Traverse wrapped errors and build a list
	type unwrapper interface {
		Unwrap() error
	}

	var parts []string
	current := err
	for current != nil {
		parts = append(parts, current.Error())

		if u, ok := current.(unwrapper); ok {
			current = u.Unwrap()
		} else {
			break
		}
	}

	// Return the top-level message + root cause
	if len(parts) == 1 {
		return parts[0]
	}

	return fmt.Sprintf("%s\n- %s", parts[0], parts[len(parts)-1])
}
