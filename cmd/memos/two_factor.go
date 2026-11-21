package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/version"
	"github.com/usememos/memos/store"
)

func newTwoFactorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "two-factor",
		Short: "Recover a user's two-factor authentication from the server terminal",
	}
	flags := cmd.PersistentFlags()
	flags.String("data", "", "data directory of the instance")
	flags.String("driver", "sqlite", "database driver of the instance (sqlite, mysql, postgres)")
	flags.String("dsn", "", "database source name of the instance")
	reset := &cobra.Command{
		Use:   "reset",
		Short: "Disable two-factor authentication and revoke sign-in sessions for one user",
		Args:  cobra.NoArgs,
		RunE:  runTwoFactorReset,
	}
	reset.Flags().String("username", "", "username of the account to recover")
	if err := reset.MarkFlagRequired("username"); err != nil {
		panic(err)
	}
	cmd.AddCommand(reset)
	return cmd
}

func runTwoFactorReset(cmd *cobra.Command, _ []string) error {
	username, err := cmd.Flags().GetString("username")
	if err != nil || strings.TrimSpace(username) == "" {
		return errors.New("provide a username with --username")
	}
	dsn, err := recoveryDSN(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	instance, err := openInstanceStore(ctx, &profile.Profile{
		Demo:    viper.GetBool("demo"),
		Data:    flagOrSetting(cmd, "data"),
		Driver:  flagOrSetting(cmd, "driver"),
		DSN:     dsn,
		Version: version.GetCurrentVersion(),
		Commit:  version.Commit,
	}, false)
	if err != nil {
		// A driver error may include a connection string or other database details.
		return errors.New("cannot open the instance database with the current schema. Check the database settings and start the current server version")
	}
	defer instance.Close()

	user, err := instance.GetUser(ctx, &store.FindUser{Username: &username})
	if err != nil {
		return errors.New("failed to look up account")
	}
	if user == nil {
		return errors.New("account not found")
	}
	if err := instance.ResetUserTwoFactor(ctx, user.ID); err != nil {
		return errors.New("failed to reset two-factor authentication")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Two-factor authentication reset. Sign in again.")
	return nil
}

// recoveryDSN resolves Docker's file-based secret when exec bypasses the entrypoint.
func recoveryDSN(cmd *cobra.Command) (string, error) {
	dsn := flagOrSetting(cmd, "dsn")
	path := os.Getenv("MEMOS_DSN_FILE")
	if cmd.Flags().Changed("dsn") || path == "" {
		return dsn, nil
	}
	if dsn != "" {
		return "", errors.New("MEMOS_DSN and MEMOS_DSN_FILE cannot both be set")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("failed to read MEMOS_DSN_FILE")
	}
	return strings.TrimRight(string(content), "\n"), nil
}
