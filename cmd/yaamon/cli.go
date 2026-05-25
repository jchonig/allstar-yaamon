package yaamon

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

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

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default: /etc/yaamon/config.yaml or ./config.yaml)")
	rootCmd.AddCommand(serveCmd)

	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "print planned changes without applying them")
	applyCmd.Flags().BoolVar(&applyResetPasswords, "reset-passwords", false, "overwrite existing user passwords from state file")
	rootCmd.AddCommand(applyCmd)
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
