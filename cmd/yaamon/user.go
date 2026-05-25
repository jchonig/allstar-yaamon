package yaamon

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
)

func init() {
	userPasswdCmd.Flags().StringP("password", "p", "", "new password (prompted if omitted)")
	userAddCmd.Flags().StringP("permission", "P", db.PermReadOnly, "permission level (readonly|readwrite|admin|superuser)")
	userAddCmd.Flags().StringP("password", "p", "", "password (prompted if omitted)")
	userPermissionCmd.Flags().StringP("permission", "P", "", "new permission level")
	_ = userPermissionCmd.MarkFlagRequired("permission")

	userCmd.AddCommand(userListCmd, userAddCmd, userPasswdCmd, userPermissionCmd, userDeleteCmd)
	rootCmd.AddCommand(userCmd)
}

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		users, err := database.ListUsers(context.Background())
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tUSERNAME\tPERMISSION")
		for _, u := range users {
			fmt.Fprintf(tw, "%d\t%s\t%s\n", u.ID, u.Username, u.Permission)
		}
		return tw.Flush()
	},
}

var userAddCmd = &cobra.Command{
	Use:   "add <username>",
	Short: "Add a new user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		perm, _ := cmd.Flags().GetString("permission")
		if !db.ValidPermission(perm) {
			return fmt.Errorf("invalid permission level: %s", perm)
		}

		pw, _ := cmd.Flags().GetString("password")
		if pw == "" {
			pw, err = promptPassword("Password: ")
			if err != nil {
				return err
			}
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			return err
		}

		u, err := database.CreateUser(context.Background(), args[0], hash, perm)
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		fmt.Printf("Created user %s (id=%d, permission=%s)\n", u.Username, u.ID, u.Permission)
		return nil
	},
}

var userPasswdCmd = &cobra.Command{
	Use:   "passwd <username>",
	Short: "Change a user's password",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		u, err := database.GetUser(context.Background(), args[0])
		if err != nil {
			return fmt.Errorf("user %q not found", args[0])
		}

		pw, _ := cmd.Flags().GetString("password")
		if pw == "" {
			pw, err = promptPassword("New password: ")
			if err != nil {
				return err
			}
		}
		hash, err := auth.HashPassword(pw)
		if err != nil {
			return err
		}

		if err := database.UpdateUserPassword(context.Background(), u.ID, hash); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		fmt.Printf("Password updated for %s\n", u.Username)
		return nil
	},
}

var userPermissionCmd = &cobra.Command{
	Use:   "permission <username>",
	Short: "Change a user's permission level",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		u, err := database.GetUser(context.Background(), args[0])
		if err != nil {
			return fmt.Errorf("user %q not found", args[0])
		}

		perm, _ := cmd.Flags().GetString("permission")
		if !db.ValidPermission(perm) {
			return fmt.Errorf("invalid permission level: %s", perm)
		}

		if err := database.UpdateUserPermission(context.Background(), u.ID, perm); err != nil {
			return fmt.Errorf("update permission: %w", err)
		}
		fmt.Printf("Permission for %s updated to %s\n", u.Username, perm)
		return nil
	},
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete <username>",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		u, err := database.GetUser(context.Background(), args[0])
		if err != nil {
			return fmt.Errorf("user %q not found", args[0])
		}

		if u.Permission == db.PermSuperuser {
			n, _ := database.CountSuperusers(context.Background())
			if n <= 1 {
				return fmt.Errorf("cannot delete the last superuser")
			}
		}

		if err := database.DeleteUser(context.Background(), u.ID); err != nil {
			return fmt.Errorf("delete user: %w", err)
		}
		fmt.Printf("Deleted user %s\n", u.Username)
		return nil
	},
}

func openDB() (*db.DB, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return db.Open(cfg.DB.Path)
}
