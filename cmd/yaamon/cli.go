package yaamon

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"allstar-yaamon/internal/backup"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
	"allstar-yaamon/internal/server"
	"allstar-yaamon/internal/state"
)

var (
	cfgFile string
	webFS   embed.FS
)

// Execute is called from main with the embedded web FS.
func Execute(fs embed.FS) {
	webFS = fs
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:          "yaamon",
	Short:        "YAAMon — AllStar node management and monitoring",
	SilenceUsage: true, // don't print usage on runtime errors
	Long: `YAAMon (Yet Another Allstar Monitor) is a multi-node AllStar management
and monitoring web application. Run 'yaamon serve' to start the server.`,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web server",
	RunE:  runServe,
}

var (
	applyDryRun         bool
	applyResetPasswords bool
)

var (
	backupPassphrase string
	backupOutput     string
	restorePassphrase string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default: /etc/yaamon/config.yaml or ./config.yaml)")
	rootCmd.AddCommand(serveCmd)

	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "print planned changes without applying them")
	applyCmd.Flags().BoolVar(&applyResetPasswords, "reset-passwords", false, "overwrite existing user passwords from state file")
	rootCmd.AddCommand(applyCmd)

	backupCmd.Flags().StringVarP(&backupPassphrase, "passphrase", "p", "", "encrypt backup with passphrase")
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "output file path (default: yaamon-<timestamp>.owbackup)")
	rootCmd.AddCommand(backupCmd)

	restoreCmd.Flags().StringVarP(&restorePassphrase, "passphrase", "p", "", "passphrase for encrypted backup")
	rootCmd.AddCommand(restoreCmd)

	rootCmd.AddCommand(inspectCmd)
}

var applyCmd = &cobra.Command{
	Use:   "apply <state-file>",
	Short: "Apply a declarative state file to the database",
	Args:  cobra.ExactArgs(1),
	RunE:  runApply,
}

func runApply(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	setupLogging(cfg.Log.Level)

	sf, err := state.Load(args[0])
	if err != nil {
		return err
	}

	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer database.Close()

	opts := state.ApplyOptions{
		DryRun:         applyDryRun,
		ResetPasswords: applyResetPasswords,
	}

	report, err := state.Apply(context.Background(), database, sf, opts)
	if err != nil {
		return err
	}

	fmt.Println(report)
	return nil
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	setupLogging(cfg.Log.Level)
	slog.Info("YAAMon starting", "tls_mode", cfg.TLS.Mode, "db", cfg.DB.Path)

	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer database.Close()

	srv, err := server.New(cfg, database, webFS)
	if err != nil {
		return fmt.Errorf("server init: %w", err)
	}

	return srv.Run()
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a .owbackup file from the live database",
	RunE:  runBackup,
}

var restoreCmd = &cobra.Command{
	Use:   "restore <file.owbackup>",
	Short: "Restore the database from a .owbackup file",
	Args:  cobra.ExactArgs(1),
	RunE:  runRestore,
}

var inspectCmd = &cobra.Command{
	Use:   "inspect <file.owbackup>",
	Short: "Print the manifest of a .owbackup file",
	Args:  cobra.ExactArgs(1),
	RunE:  runInspect,
}

func runBackup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	setupLogging(cfg.Log.Level)

	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer database.Close()

	data, manifest, err := backup.Create(context.Background(), database, server.Version, backup.CreateOptions{
		Passphrase: backupPassphrase,
	})
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	outPath := backupOutput
	if outPath == "" {
		outPath = fmt.Sprintf("yaamon-%s.owbackup", manifest.CreatedAt.Format("20060102T150405Z"))
	}
	if err := os.WriteFile(outPath, data, 0600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	fmt.Printf("Backup written to %s (%d bytes, encrypted=%v)\n", outPath, len(data), manifest.Encrypted)
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	setupLogging(cfg.Log.Level)

	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}

	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer database.Close()

	preRestorePath, err := backup.Restore(context.Background(), database, server.Version, data, backup.RestoreOptions{
		Passphrase: restorePassphrase,
	})
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	fmt.Printf("Restore complete. Pre-restore backup saved to %s\n", preRestorePath)
	fmt.Println("Restart the server to use the restored database.")
	return nil
}

func runInspect(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	manifest, err := backup.Inspect(data)
	if err != nil {
		return err
	}
	fmt.Printf("Format:         %s v%d\n", manifest.Format, manifest.FormatVersion)
	fmt.Printf("App Version:    %s\n", manifest.AppVersion)
	fmt.Printf("Schema Version: %d\n", manifest.SchemaVersion)
	fmt.Printf("Created At:     %s\n", manifest.CreatedAt.Local().Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("Hostname:       %s\n", manifest.Hostname)
	fmt.Printf("Encrypted:      %v\n", manifest.Encrypted)
	fmt.Printf("Contents:       %d nodes, %d favorites, %d users, %d configs\n",
		manifest.Contents.Nodes, manifest.Contents.Favorites,
		manifest.Contents.Users, manifest.Contents.Configs)
	return nil
}

func setupLogging(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}
