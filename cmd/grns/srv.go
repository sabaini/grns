package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

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
			logger.Debug("resolved config",
				"api_url", cfg.APIURL,
				"api_url_source", cfg.Source("api_url"),
				"api_token_env_set", strings.TrimSpace(os.Getenv("GRNS_API_TOKEN")) != "",
				"admin_token_env_set", strings.TrimSpace(os.Getenv("GRNS_ADMIN_TOKEN")) != "",
				"db_path", cfg.DBPath,
				"db_path_source", cfg.Source("db_path"),
				"project_prefix", cfg.ProjectPrefix,
				"project_prefix_source", cfg.Source("project_prefix"),
				"log_level", cfg.LogLevel,
				"log_level_source", cfg.Source("log_level"),
				"attachments.max_upload_bytes", cfg.Attachments.MaxUploadBytes,
				"attachments.max_upload_bytes_source", cfg.Source("attachments.max_upload_bytes"),
				"attachments.multipart_max_memory", cfg.Attachments.MultipartMaxMemory,
				"attachments.multipart_max_memory_source", cfg.Source("attachments.multipart_max_memory"),
				"attachments.allowed_media_types", strings.Join(cfg.Attachments.AllowedMediaTypes, ","),
				"attachments.allowed_media_types_source", cfg.Source("attachments.allowed_media_types"),
				"attachments.reject_media_type_mismatch", cfg.Attachments.RejectMediaTypeMismatch,
				"attachments.reject_media_type_mismatch_source", cfg.Source("attachments.reject_media_type_mismatch"),
				"attachments.gc_batch_size", cfg.Attachments.GCBatchSize,
				"attachments.gc_batch_size_source", cfg.Source("attachments.gc_batch_size"),
				"loaded_config_paths", strings.Join(cfg.LoadedPaths(), ","),
			)

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
			srv.SetDBPath(cfg.DBPath)
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
