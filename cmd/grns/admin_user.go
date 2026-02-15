package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"grns/internal/api"
	internalauth "grns/internal/auth"
	"grns/internal/config"
)

func newAdminUserCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage local admin users for browser authentication",
	}
	cmd.AddCommand(newAdminUserAddCmd(cfg, jsonOutput))
	cmd.AddCommand(newAdminUserListCmd(cfg, jsonOutput))
	cmd.AddCommand(newAdminUserSetDisabledCmd(cfg, jsonOutput, "disable", "Disable one admin user", true))
	cmd.AddCommand(newAdminUserSetDisabledCmd(cfg, jsonOutput, "enable", "Enable one admin user", false))
	cmd.AddCommand(newAdminUserDeleteCmd(cfg, jsonOutput))
	return cmd
}

func newAdminUserAddCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var passwordStdin bool

	cmd := &cobra.Command{
		Use:   "add <username>",
		Short: "Create one local admin user",
		Args:  requireExactlyArgs(1, "username is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !passwordStdin {
				return fmt.Errorf("--password-stdin is required")
			}

			username, err := internalauth.NormalizeUsername(args[0])
			if err != nil {
				return err
			}

			passwordBytes, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			password := strings.TrimSpace(string(passwordBytes))

			return withClient(cfg, func(client *api.Client) error {
				created, err := client.AdminUserAdd(cmd.Context(), api.AdminUserCreateRequest{Username: username, Password: password})
				if err != nil {
					return err
				}

				if *jsonOutput {
					return writeJSON(map[string]any{
						"id":       created.ID,
						"username": created.Username,
						"role":     created.Role,
						"disabled": created.Disabled,
					})
				}
				return writePlain("created admin user %s (%s)\n", created.Username, created.ID)
			})
		},
	}

	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin")
	return cmd
}

func newAdminUserListCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List provisioned admin users",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withClient(cfg, func(client *api.Client) error {
				users, err := client.AdminUserList(cmd.Context())
				if err != nil {
					return err
				}
				if *jsonOutput {
					return writeJSON(map[string]any{"count": len(users), "users": users})
				}
				if len(users) == 0 {
					return writePlain("no admin users configured\n")
				}
				if err := writePlain("USERNAME\tROLE\tSTATUS\tID\n"); err != nil {
					return err
				}
				for _, user := range users {
					status := "enabled"
					if user.Disabled {
						status = "disabled"
					}
					if err := writePlain("%s\t%s\t%s\t%s\n", user.Username, user.Role, status, user.ID); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
}

func newAdminUserSetDisabledCmd(cfg *config.Config, jsonOutput *bool, name, short string, disabled bool) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <username>",
		Short: short,
		Args:  requireExactlyArgs(1, "username is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			username, err := internalauth.NormalizeUsername(args[0])
			if err != nil {
				return err
			}

			return withClient(cfg, func(client *api.Client) error {
				updated, err := client.AdminUserSetDisabled(cmd.Context(), username, disabled)
				if err != nil {
					return err
				}

				if *jsonOutput {
					return writeJSON(map[string]any{
						"id":       updated.ID,
						"username": updated.Username,
						"role":     updated.Role,
						"disabled": updated.Disabled,
					})
				}

				action := "enabled"
				if disabled {
					action = "disabled"
				}
				return writePlain("%s admin user %s\n", action, updated.Username)
			})
		},
	}
}

func newAdminUserDeleteCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <username>",
		Aliases: []string{"rm"},
		Short:   "Delete one admin user",
		Args:    requireExactlyArgs(1, "username is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			username, err := internalauth.NormalizeUsername(args[0])
			if err != nil {
				return err
			}

			return withClient(cfg, func(client *api.Client) error {
				resp, err := client.AdminUserDelete(cmd.Context(), username)
				if err != nil {
					return err
				}

				if *jsonOutput {
					return writeJSON(resp)
				}
				return writePlain("deleted admin user %s\n", resp.Username)
			})
		},
	}
	return cmd
}
