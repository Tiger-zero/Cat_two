package test

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	driver := getDriverFromEnv()
	// Keep later fixture-level .env loads from changing the selected driver.
	if err := os.Setenv("DRIVER", driver); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if driver == "mysql" {
		if _, err := getMySQLVariant(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	defer TerminateContainers()
	m.Run() //nolint:revive // The testing package uses the result after cleanup.
}
