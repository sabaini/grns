package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"grns/internal/api"
	"grns/internal/config"
)

func newImportCmd(cfg *config.Config, jsonOutput *bool) *cobra.Command {
	var (
		inputPath      string
		dryRun         bool
		dedupe         string
		orphanHandling string
		stream         bool
	)

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import tasks from a JSONL file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputPath == "" {
				return errors.New("--input is required")
			}

			return withClient(cfg, func(client *api.Client) error {
				f, err := os.Open(inputPath)
				if err != nil {
					return err
				}
				defer f.Close()

				var (
					resp      api.ImportResponse
					importErr error
				)
				if stream {
					resp, importErr = client.ImportStream(cmd.Context(), f, dryRun, dedupe, orphanHandling)
				} else {
					// Preserve existing import semantics by default.
					var records []api.TaskImportRecord
					scanner := bufio.NewScanner(f)
					scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
					lineNum := 0
					for scanner.Scan() {
						lineNum++
						line := scanner.Bytes()
						if len(line) == 0 {
							continue
						}
						var rec api.TaskImportRecord
						if err := json.Unmarshal(line, &rec); err != nil {
							return fmt.Errorf("line %d: %w", lineNum, err)
						}
						records = append(records, rec)
					}
					if err := scanner.Err(); err != nil {
						return fmt.Errorf("reading input: %w", err)
					}
					if len(records) == 0 {
						return errors.New("no records found in input file")
					}
					resp, importErr = client.Import(cmd.Context(), api.ImportRequest{
						Tasks:          records,
						DryRun:         dryRun,
						Dedupe:         dedupe,
						OrphanHandling: orphanHandling,
					})
				}
				if importErr != nil {
					return importErr
				}

				if *jsonOutput {
					return writeJSON(resp)
				}

				return writePlain("created: %d, updated: %d, skipped: %d, errors: %d\n",
					resp.Created, resp.Updated, resp.Skipped, resp.Errors)
			})
		},
	}

	cmd.Flags().StringVarP(&inputPath, "input", "i", "", "input JSONL file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview without making changes")
	cmd.Flags().StringVar(&dedupe, "dedupe", "skip", "dedupe mode: skip|overwrite|error")
	cmd.Flags().StringVar(&orphanHandling, "orphan-handling", "allow", "orphan dep handling: allow|skip|strict")
	cmd.Flags().BoolVar(&stream, "stream", false, "use streaming import endpoint for large files")

	return cmd
}
