package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"

	"grns/internal/blobstore"
	"grns/internal/config"
	"grns/internal/server"
	"grns/internal/store"
)

func newSrvCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "srv",
		Short: "Run the grns API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg == nil {
				return fmt.Errorf("config not initialized")
			}
			if cfg.DBPath == "" {
				return fmt.Errorf("db path is required")
			}

			logger := slog.Default().With("component", "server")

			addr, err := server.ListenAddr(cfg.APIURL)
			if err != nil {
				return err
			}

			logger.Info("opening database", "path", cfg.DBPath)
			st, err := store.Open(cfg.DBPath)
			if err != nil {
				return err
			}
			defer st.Close()

			blobRoot := filepath.Join(filepath.Dir(cfg.DBPath), ".grns", "blobs")
			bs, err := blobstore.NewLocalCAS(blobRoot)
			if err != nil {
				return err
			}

			srv := server.New(addr, st, cfg.ProjectPrefix, logger, bs)
			srv.ConfigureAttachmentOptions(server.AttachmentOptions{
				MaxUploadBytes:          cfg.Attachments.MaxUploadBytes,
				MultipartMaxMemory:      cfg.Attachments.MultipartMaxMemory,
				AllowedMediaTypes:       cfg.Attachments.AllowedMediaTypes,
				RejectMediaTypeMismatch: cfg.Attachments.RejectMediaTypeMismatch,
				GCBatchSize:             cfg.Attachments.GCBatchSize,
			})
			return srv.ListenAndServe()
		},
	}
}
