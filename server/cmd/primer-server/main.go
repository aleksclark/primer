// Command primer-server runs the Primer LMS HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aleksclark/primer/server/internal/agent"
	"github.com/aleksclark/primer/server/internal/api"
	"github.com/aleksclark/primer/server/internal/artifacts"
	"github.com/aleksclark/primer/server/internal/config"
	"github.com/aleksclark/primer/server/internal/db"
	"github.com/aleksclark/primer/server/internal/remoteagent"
	"github.com/aleksclark/primer/server/internal/spa"
	"github.com/aleksclark/primer/server/internal/tutor"
	mafagent "github.com/microsoft/agent-framework-go/agent"
	"github.com/microsoft/agent-framework-go/provider/openaiprovider"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.ServiceToken == "" {
		// Worth shouting about: without a token anyone who can reach the port
		// can inflate the student's instructional hours.
		slog.Warn("service token is not set; the instruction log ingest is unauthenticated")
	}

	tutorCfg := tutor.DefaultConfig()
	tutorCfg.Provider = cfg.TutorProvider
	tutorCfg.Enabled = cfg.TutorEnabled
	tutorCfg.Bedrock = tutor.BedrockConfig{
		URL:    cfg.TutorBedrockURL,
		APIKey: cfg.TutorBedrockAPIKey,
		Model:  cfg.TutorBedrockModel,
	}
	tutorSvc, err := tutor.NewFromConfig(tutorCfg)
	if err != nil {
		return fmt.Errorf("tutor: %w", err)
	}
	slog.Info("tutor configured", "provider", tutorSvc.ProviderName(), "enabled", tutorSvc.Enabled())

	var agentController *agent.Controller
	if cfg.AgentRuntimeEnabled {
		if cfg.AgentRuntimeBaseURL == "" || cfg.AgentRuntimeAPIKey == "" || cfg.AgentRuntimeModel == "" {
			return fmt.Errorf("agent runtime enabled but AGENT_RUNTIME_BASE_URL, AGENT_RUNTIME_API_KEY, and AGENT_RUNTIME_MODEL are required")
		}
		client := openai.NewClient(
			option.WithBaseURL(cfg.AgentRuntimeBaseURL),
			option.WithAPIKey(cfg.AgentRuntimeAPIKey),
		)
		root := openaiprovider.NewChatCompletionsAgent(client, openaiprovider.AgentConfig{
			Config: mafagent.Config{ID: "primer-overseer", Name: "Overseer", Description: "Primer session overseer"},
			Model:  cfg.AgentRuntimeModel,
		})
		agentController = agent.NewController(agent.ControllerConfig{
			Agent:     root,
			Spec:      agent.AgentSpec{Type: "overseer", Name: "Overseer", MaxChildren: 0},
			RunBudget: cfg.AgentRuntimeRunBudget,
		})
		slog.Info("agent runtime enabled", "maf_commit", "00ffc8c3648c547997eae3a3f2a3b00c28daea09", "budget", cfg.AgentRuntimeRunBudget)
	} else {
		slog.Info("agent runtime disabled")
	}

	// primer-agents remote service integration — PRIMER_AGENTS_ENABLED (default false).
	// Kept fully independent from AGENT_RUNTIME_ENABLED so both flags can coexist
	// during the rollout period. Fantasy/LMS tutor paths are unaffected when disabled.
	remoteAdapter := remoteagent.New(remoteagent.Config{
		Enabled:     cfg.PrimerAgentsEnabled,
		BaseURL:     cfg.PrimerAgentsBaseURL,
		Timeout:     cfg.PrimerAgentsTimeout,
		TokenSource: remoteagent.EnvTokenSource{EnvVar: cfg.PrimerAgentsTokenEnvVar},
	})
	if cfg.PrimerAgentsEnabled {
		slog.Info("primer-agents remote integration enabled", "base_url", cfg.PrimerAgentsBaseURL)
	} else {
		slog.Info("primer-agents remote integration disabled (PRIMER_AGENTS_ENABLED=false)")
	}
	_ = remoteAdapter // wired into API opts below once UI is implemented (Phase 7 UI deferred)

	var artStore *artifacts.Store
	if cfg.ArtifactStoreDir != "" {
		s, err := artifacts.NewStore(cfg.ArtifactStoreDir)
		if err != nil {
			return fmt.Errorf("artifact store: %w", err)
		}
		artStore = s
		slog.Info("artifact store configured", "root", s.Root)
	} else {
		slog.Warn("ARTIFACT_STORE_DIR is not set; artifact byte upload is disabled")
	}

	_, handler := api.New(pool, api.Options{
		CORSOrigins:       cfg.CORSOrigins,
		ServiceToken:      cfg.ServiceToken,
		Tutor:             tutorSvc,
		TutorProviderName: tutorSvc.ProviderName(),
		TutorEnabled:      tutorSvc.Enabled(),
		ArtifactStore:     artStore,
		AgentController:   agentController,
	})

	mux := http.NewServeMux()
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", handler))
	mux.Handle("/", spa.Handler())

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if agentController != nil {
			if err := agentController.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("agent runtime shutdown: %w", err)
			}
		}
		return srv.Shutdown(shutdownCtx)
	}
}
