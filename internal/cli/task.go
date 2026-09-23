package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/sausheong/sidecar/internal/adapter"
	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/loop"
	"github.com/sausheong/sidecar/internal/store"
	"github.com/spf13/cobra"
)

func taskCmd() *cobra.Command {
	var repoFlag string
	cmd := &cobra.Command{
		Use:   "task <description>",
		Short: "Submit an on-demand improvement task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			description := args[0]

			repoPath := repoFlag
			if repoPath == "" {
				repoPath = "."
			}
			abs, err := filepath.Abs(repoPath)
			if err != nil {
				return err
			}

			dbURL := os.Getenv("SIDECAR_DB_URL")
			if dbURL == "" {
				return fmt.Errorf("SIDECAR_DB_URL environment variable is required")
			}

			if os.Getenv("ANTHROPIC_API_KEY") == "" {
				return fmt.Errorf("ANTHROPIC_API_KEY environment variable is required")
			}

			ctx := context.Background()
			db, err := store.Connect(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("connecting to database: %w", err)
			}
			defer db.Close()

			if err := store.Migrate(ctx, db); err != nil {
				return err
			}

			cfg, err := config.Load(filepath.Join(abs, "sidecar.yaml"))
			if err != nil {
				log.Printf("warning: no sidecar.yaml at %s, using defaults", abs)
				cfg = &config.Config{}
			}

			ws, err := db.GetWorkspaceByPath(ctx, abs)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					return fmt.Errorf("looking up workspace: %w", err)
				}
				// workspace doesn't exist yet — create it
				ws = &store.Workspace{
					Name:       filepath.Base(abs),
					Path:       abs,
					ConfigHash: "",
				}
				if err := db.UpsertWorkspace(ctx, ws); err != nil {
					return err
				}
			}

			sig := adapter.Signal{
				Type:   adapter.SignalOnDemand,
				Source: "cli",
				Payload: map[string]any{
					"description": description,
				},
			}

			l := loop.New(db, ws, cfg, abs, buildEmbeddingProvider(cfg))
			log.Printf("Running task: %s", description)
			return l.Run(ctx, sig)
		},
	}
	cmd.Flags().StringVar(&repoFlag, "repo", "", "path to the target repository (default: .)")
	cmd.AddCommand(taskShowCmd())
	return cmd
}

func taskShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <task-id>",
		Short: "Show a task's sanitized completion handoff and trace references",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("invalid task ID: %w", err)
			}
			dbURL := os.Getenv("SIDECAR_DB_URL")
			if dbURL == "" {
				return fmt.Errorf("SIDECAR_DB_URL environment variable is required")
			}
			ctx := cmd.Context()
			db, err := store.Connect(ctx, dbURL)
			if err != nil {
				return fmt.Errorf("connecting to database: %w", err)
			}
			defer db.Close()
			task, err := db.GetTask(ctx, id)
			if err != nil {
				return err
			}
			events, err := db.GetTaskEvents(ctx, id)
			if err != nil {
				return err
			}
			traces, err := db.ListAgentTraceEvents(ctx, id, 100)
			if err != nil {
				return err
			}
			var handoff any
			traceIDs := map[string]bool{}
			for _, event := range events {
				if event.Type == "suggestion" || event.Type == "coding_incomplete" || event.Type == "evaluation_incomplete" || event.Type == "incomplete_handoff" {
					handoff = event.Payload
				}
			}
			for _, event := range traces {
				traceIDs[event.TraceID.String()] = true
			}
			result := map[string]any{"task_id": task.ID, "status": task.Status, "summary": task.Summary,
				"handoff": handoff, "trace_ids": traceIDs}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(result)
		},
	}
}
