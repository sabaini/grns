package main

import (
	"database/sql"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"grns/internal/config"
	"grns/internal/store"

	_ "modernc.org/sqlite"
)

func newMigrateCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var dryRun bool
	var inspect bool

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run or inspect database schema migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openRawDB(cfg.DBPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if inspect || dryRun {
				plan, err := store.MigrationPlan(db)
				if err != nil {
					return fmt.Errorf("inspect migrations: %w", err)
				}

				if *jsonOutput {
					return writeJSON(plan)
				}

				fmt.Printf("Current version: %d\n", plan.CurrentVersion)
				fmt.Printf("Available version: %d\n", plan.AvailableVersion)
				if len(plan.Pending) == 0 {
					fmt.Println("No pending migrations.")
				} else {
					fmt.Printf("Pending migrations: %d\n", len(plan.Pending))
					for _, m := range plan.Pending {
						fmt.Printf("  %d: %s\n", m.Version, m.Description)
					}
				}
				return nil
			}

			// Run migrations (same as what happens on server start).
			st, err := store.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			defer st.Close()

			if *jsonOutput {
				// Re-open raw DB to get status after migration.
				db2, err := openRawDB(cfg.DBPath)
				if err != nil {
					return err
				}
				defer db2.Close()

				plan, err := store.MigrationPlan(db2)
				if err != nil {
					return err
				}
				return writeJSON(plan)
			}

			fmt.Println("Migrations applied successfully.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show pending migrations without applying")
	cmd.Flags().BoolVar(&inspect, "inspect", false, "show migration status")

	return cmd
}

func openRawDB(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("db path is required")
	}
	u := url.URL{Scheme: "file", Path: path}
	return sql.Open("sqlite", u.String())
}
