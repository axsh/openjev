package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/spf13/cobra"

	"openjev/features/decision-test/internal/api"
	"openjev/features/decision-test/internal/cli"
	"openjev/features/decision-test/internal/config"
	"openjev/features/decision-test/internal/decision"
	"openjev/features/decision-test/internal/domain"
	"openjev/features/decision-test/internal/engine"
	"openjev/features/decision-test/internal/logger"
)

type serverOptions struct {
	Config string `doc:"Settings file" default:"settings/decision-test.yaml"`
	Port   int    `doc:"Override api_port when non-zero" default:"0"`
}

func main() {
	rt := &runtime{}
	command := humacli.New(func(hooks humacli.Hooks, opts *serverOptions) {
		hooks.OnStart(func() {
			err := rt.listen(opts)
			if err != nil && !errors.Is(err, http.ErrServerClosed) && rt.log != nil {
				rt.log.Error("server stopped", "error", err)
			}
		})
		hooks.OnStop(func() {
			rt.shutdown()
		})
	})
	root := command.Root()
	root.AddCommand(serveCommand(rt))
	root.AddCommand(decideCommand())
	command.Run()
}

type runtime struct {
	srv *http.Server
	log *logger.Logger
}

func (rt *runtime) listen(opts *serverOptions) error {
	srv, log, err := buildServer(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	rt.srv = srv
	rt.log = log
	return srv.ListenAndServe()
}

func (rt *runtime) shutdown() {
	if rt.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = rt.srv.Shutdown(ctx)
}

func serveCommand(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use: "serve",
		Run: humacli.WithOptions(func(cmd *cobra.Command, args []string, opts *serverOptions) {
			err := rt.listen(opts)
			if err != nil && !errors.Is(err, http.ErrServerClosed) && rt.log != nil {
				rt.log.Error("server stopped", "error", err)
			}
		}),
	}
}

func decideCommand() *cobra.Command {
	var input, server, method string
	var asJSON bool
	cmd := &cobra.Command{
		Use: "decide",
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			humacli.WithOptions(func(cmd *cobra.Command, args []string, opts *serverOptions) {
				err = runDecide(opts, input, server, method, asJSON)
			})(cmd, args)
			return err
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "Request JSON file")
	cmd.Flags().StringVar(&server, "server", "", "API base URL")
	cmd.Flags().StringVar(&method, "method", "", "Override options.method")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the response body")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func runDecide(opts *serverOptions, input, server, method string, asJSON bool) error {
	if server == "" {
		cfg, err := config.Load(opts.Config)
		if err != nil {
			return err
		}
		port := cfg.APIPort
		if opts.Port != 0 {
			port = opts.Port
		}
		server = fmt.Sprintf("http://%s:%d", cfg.APIHost, port)
	}
	return cli.Decide(context.Background(), cli.Options{
		Input:  input,
		Server: server,
		Method: method,
		JSON:   asJSON,
	}, os.Stdout, os.Stderr)
}

func buildServer(opts *serverOptions) (*http.Server, *logger.Logger, error) {
	cfg, err := config.Load(opts.Config)
	if err != nil {
		return nil, nil, err
	}
	if opts.Port != 0 {
		cfg.APIPort = opts.Port
	}
	output := io.Writer(os.Stderr)
	if cfg.LogPath != "" {
		file, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, err
		}
		output = file
	}
	log := logger.New(output).WithComponent("decision")
	eng := engine.NewClient(cfg.LlamaURL, log)
	labels, labelErr := engine.ResolveLabels(context.Background(), eng)
	var svc *decision.Service
	readyErr := ""
	if labelErr != nil {
		readyErr = labelErr.Error()
		log.Error("label resolve failed", "error", labelErr)
	} else if err := eng.Warmup(context.Background()); err != nil {
		readyErr = err.Error()
		log.Error("warmup failed", "error", err)
	} else {
		svc = &decision.Service{Engine: eng, Labels: labels, Log: log, ModelID: cfg.ModelID}
		log.Info("server starting", "api_port", cfg.APIPort, "model_id", cfg.ModelID, "llama_url", cfg.LlamaURL)
	}
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, huma.DefaultConfig("Decision Test", "0.0.1"))
	api.Register(humaAPI, svc, func(ctx context.Context) domain.Health {
		reachable, herr := eng.Health(ctx)
		h := domain.Health{
			Ready:          svc != nil && reachable && herr == nil,
			Model:          cfg.ModelID,
			LlamaReachable: reachable && herr == nil,
			Error:          readyErr,
		}
		if herr != nil && h.Error == "" {
			h.Error = herr.Error()
		}
		if svc != nil {
			h.LabelTokenIDs = map[string]int{}
			for _, label := range labels {
				h.LabelTokenIDs[label.Letter] = label.TokenID
			}
		}
		return h
	})
	return &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.APIHost, cfg.APIPort),
		Handler: mux,
	}, log, nil
}
