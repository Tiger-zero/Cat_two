package main

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/version"
)

func init() {
	rootCmd.AddCommand(newNotesCommand())
}

func newNotesCommand() *cobra.Command {
	command := &cobra.Command{Use: "notes", Short: "Check and repair Markdown files while the server is stopped"}
	command.PersistentFlags().String("data", "", "data directory of the instance")
	command.PersistentFlags().String("dsn", "", "SQLite database path of the instance")
	command.AddCommand(
		&cobra.Command{Use: "check", Short: "Check the Markdown files against the database", Args: cobra.NoArgs, RunE: runNotesCheck(false)},
		&cobra.Command{Use: "repair", Short: "Rebuild damaged Markdown from verified database copies", Args: cobra.NoArgs, RunE: runNotesCheck(true)},
	)
	return command
}

func runNotesCheck(repair bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		p := &profile.Profile{
			Data: flagOrSetting(cmd, "data"), Driver: flagOrSetting(cmd, "driver"), DSN: flagOrSetting(cmd, "dsn"),
			Version: version.GetCurrentVersion(), Personal: true,
		}
		if err := p.Validate(); err != nil {
			return err
		}
		ctx := cmd.Context()
		release, err := lockInstance(ctx, p)
		if err != nil {
			return err
		}
		defer release()
		st, err := openInstanceStore(ctx, p, false)
		if err != nil {
			return err
		}
		defer st.Close()
		var checked, repaired, issues int
		if err := st.CheckMemoBodies(ctx, repair, func(id int32, err error, repairedFile bool) {
			checked++
			if err != nil {
				issues++
				fmt.Fprintf(cmd.ErrOrStderr(), "memo %d: %v\n", id, err)
			}
			if repairedFile {
				repaired++
			}
		}); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Checked %d Markdown references, repaired %d, unresolved %d.\n", checked, repaired, issues)
		if issues > 0 {
			return errors.Errorf("%d Markdown references still need attention", issues)
		}
		return nil
	}
}
