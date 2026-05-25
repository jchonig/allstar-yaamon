package yaamon

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"allstar-yaamon/internal/ami"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
)

func init() {
	nodeAddCmd.Flags().StringP("number", "n", "", "node number (required)")
	nodeAddCmd.Flags().StringP("ami-host", "H", "localhost", "AMI host")
	nodeAddCmd.Flags().IntP("ami-port", "p", 5038, "AMI port")
	nodeAddCmd.Flags().StringP("ami-user", "u", "", "AMI username")
	nodeAddCmd.Flags().StringP("ami-pass", "P", "", "AMI password")
	nodeAddCmd.Flags().BoolP("enabled", "e", true, "connect AMI on start")
	_ = nodeAddCmd.MarkFlagRequired("number")

	nodeEditCmd.Flags().StringP("name", "N", "", "new name")
	nodeEditCmd.Flags().StringP("number", "n", "", "new node number")
	nodeEditCmd.Flags().StringP("ami-host", "H", "", "new AMI host")
	nodeEditCmd.Flags().IntP("ami-port", "p", 0, "new AMI port")
	nodeEditCmd.Flags().StringP("ami-user", "u", "", "new AMI username")
	nodeEditCmd.Flags().StringP("ami-pass", "P", "", "new AMI password")

	nodeCmd.AddCommand(nodeListCmd, nodeAddCmd, nodeEditCmd, nodeEnableCmd, nodeDisableCmd, nodeDeleteCmd, nodeTestCmd)
	rootCmd.AddCommand(nodeCmd)
}

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Manage nodes",
}

var nodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		nodes, err := database.ListNodes(context.Background())
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tNODE#\tAMI HOST\tPORT\tUSER\tENABLED")
		for _, n := range nodes {
			fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%s\t%v\n",
				n.ID, n.Name, n.NodeNumber, n.AMIHost, n.AMIPort, n.AMIUser, n.Enabled)
		}
		return tw.Flush()
	},
}

var nodeAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new node",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		number, _ := cmd.Flags().GetString("number")
		host, _ := cmd.Flags().GetString("ami-host")
		port, _ := cmd.Flags().GetInt("ami-port")
		user, _ := cmd.Flags().GetString("ami-user")
		pass, _ := cmd.Flags().GetString("ami-pass")
		enabled, _ := cmd.Flags().GetBool("enabled")

		n, err := database.CreateNode(context.Background(), db.Node{
			Name:       args[0],
			NodeNumber: number,
			AMIHost:    host,
			AMIPort:    port,
			AMIUser:    user,
			AMIPass:    pass,
			Enabled:    enabled,
		})
		if err != nil {
			return fmt.Errorf("create node: %w", err)
		}
		fmt.Printf("Created node %s (id=%d, node#=%s)\n", n.Name, n.ID, n.NodeNumber)
		return nil
	},
}

var nodeEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a node's configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id: %s", args[0])
		}
		n, err := database.GetNodeByID(context.Background(), id)
		if err != nil {
			return fmt.Errorf("node %d not found", id)
		}

		if v, _ := cmd.Flags().GetString("name"); v != "" {
			n.Name = v
		}
		if v, _ := cmd.Flags().GetString("number"); v != "" {
			n.NodeNumber = v
		}
		if v, _ := cmd.Flags().GetString("ami-host"); v != "" {
			n.AMIHost = v
		}
		if v, _ := cmd.Flags().GetInt("ami-port"); v != 0 {
			n.AMIPort = v
		}
		if v, _ := cmd.Flags().GetString("ami-user"); v != "" {
			n.AMIUser = v
		}
		if v, _ := cmd.Flags().GetString("ami-pass"); v != "" {
			n.AMIPass = v
		}

		if err := database.UpdateNode(context.Background(), *n); err != nil {
			return fmt.Errorf("update node: %w", err)
		}
		fmt.Printf("Updated node %s (id=%d)\n", n.Name, n.ID)
		return nil
	},
}

var nodeEnableCmd = &cobra.Command{
	Use:   "enable <id>",
	Short: "Enable a node",
	Args:  cobra.ExactArgs(1),
	RunE:  setNodeEnabled(true),
}

var nodeDisableCmd = &cobra.Command{
	Use:   "disable <id>",
	Short: "Disable a node",
	Args:  cobra.ExactArgs(1),
	RunE:  setNodeEnabled(false),
}

func setNodeEnabled(enabled bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id: %s", args[0])
		}
		n, err := database.GetNodeByID(context.Background(), id)
		if err != nil {
			return fmt.Errorf("node %d not found", id)
		}
		n.Enabled = enabled
		if err := database.UpdateNode(context.Background(), *n); err != nil {
			return fmt.Errorf("update node: %w", err)
		}
		state := "enabled"
		if !enabled {
			state = "disabled"
		}
		fmt.Printf("Node %s (id=%d) %s\n", n.Name, n.ID, state)
		return nil
	}
}

var nodeDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a node and its favorites",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id: %s", args[0])
		}
		n, err := database.GetNodeByID(context.Background(), id)
		if err != nil {
			return fmt.Errorf("node %d not found", id)
		}
		if err := database.DeleteNode(context.Background(), id); err != nil {
			return fmt.Errorf("delete node: %w", err)
		}
		fmt.Printf("Deleted node %s (id=%d)\n", n.Name, id)
		return nil
	},
}

var nodeTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Test AMI connection for a node",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		database, err := db.Open(cfg.DB.Path)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer database.Close()

		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id: %s", args[0])
		}
		n, err := database.GetNodeByID(context.Background(), id)
		if err != nil {
			return fmt.Errorf("node %d not found", id)
		}

		if err := ami.TestConnection(n.AMIHost, n.AMIPort, n.AMIUser, n.AMIPass); err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
		fmt.Printf("AMI connection to %s (%s:%d) OK\n", n.Name, n.AMIHost, n.AMIPort)
		return nil
	},
}
