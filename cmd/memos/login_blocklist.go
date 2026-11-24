package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/version"
)

func newLoginBlocklistCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login-blocklist",
		Short: "Manage the sign-in blocklist from the server terminal",
	}
	flags := cmd.PersistentFlags()
	flags.String("data", "", "data directory of the instance")
	flags.String("driver", profile.SQLiteDriver, "database driver (sqlite)")
	flags.String("dsn", "", "SQLite database path or file URI")
	remove := &cobra.Command{
		Use:   "remove",
		Short: "Clear the sign-in block for one IP address",
		Args:  cobra.NoArgs,
		RunE:  runLoginBlocklistRemove,
	}
	remove.Flags().String("ip", "", "one IPv4 or IPv6 address")
	if err := remove.MarkFlagRequired("ip"); err != nil {
		panic(err)
	}
	cmd.AddCommand(remove)
	return cmd
}

func runLoginBlocklistRemove(cmd *cobra.Command, _ []string) error {
	ip, err := cmd.Flags().GetString("ip")
	if err != nil {
		return err
	}
	address, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || address.Zone() != "" {
		return errors.New("provide one IPv4 or IPv6 address with --ip")
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
		// Database errors can contain connection credentials.
		return errors.New("cannot open the instance database with the current schema. Check the database settings and start the current server version")
	}
	defer instance.Close()

	address = address.Unmap()
	removed, err := instance.UnblockLoginIP(ctx, address.String())
	if err != nil {
		return errors.New("failed to clear the sign-in block")
	}
	if removed {
		fmt.Fprintf(cmd.OutOrStdout(), "Removed %s from the sign-in blocklist.\n", address)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "No sign-in block found for %s.\n", address)
	}
	return nil
}
