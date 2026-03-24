// Command gort is the GORT (GitOps Reconciliation Tool) server.
// It wires all components together and starts the webhook HTTP server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gogithub "github.com/google/go-github/v71/github"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/oauth2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	gortv1alpha1 "github.com/clcollins/gort/api/v1alpha1"
	"github.com/clcollins/gort/internal/claudeai"
	"github.com/clcollins/gort/internal/flux"
	githubclient "github.com/clcollins/gort/internal/github"
	internalk8s "github.com/clcollins/gort/internal/k8s"
	_ "github.com/clcollins/gort/internal/metrics" // register metrics on init
	"github.com/clcollins/gort/internal/reconciler"
	"github.com/clcollins/gort/internal/webhook"
	"github.com/clcollins/gort/pkg/vcs"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
)

// version is set at build time via -ldflags="-X main.version=...".
var version string

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	slog.Info("starting gort", "version", version)

	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()

	// Build Kubernetes scheme with all required types.
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("add corev1: %w", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("add appsv1: %w", err)
	}
	if err := kustomizev1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("add kustomizev1: %w", err)
	}
	if err := gortv1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("add gortv1alpha1: %w", err)
	}

	// Build in-cluster Kubernetes client.
	restCfg, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("get kubeconfig: %w", err)
	}
	rawClient, err := ctrlclient.New(restCfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("build k8s client: %w", err)
	}
	k8sClient := internalk8s.NewClient(rawClient)

	// Build GitOps (Flux) client.
	gitopsClient := flux.NewClient(k8sClient)

	// Build VCS (GitHub) client.
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.githubToken})
	tc := oauth2.NewClient(context.Background(), ts)
	gc := gogithub.NewClient(tc)
	vcsClient := githubclient.NewClient(gc, cfg.webhookSecret)

	// Build AI (Claude) client.
	aiClient := claudeai.NewClient(cfg.claudeAPIKey, cfg.claudeModel)

	// Build reconciler.
	rec := reconciler.New(gitopsClient, vcsClient, aiClient)

	// Webhook dispatch: look up GitOpsWatcher CRDs, reconcile each matching one.
	dispatch := func(ctx context.Context, event *vcs.PushEvent) {
		if event.Branch != "main" {
			return
		}
		watchers := &gortv1alpha1.GitOpsWatcherList{}
		if err := rawClient.List(ctx, watchers); err != nil {
			slog.Error("list gitopswatchers", "err", err)
			return
		}
		for _, w := range watchers.Items {
			if w.Spec.TargetRepo != event.RepoFullName {
				continue
			}
			w := w // capture
			timeout := 10 * time.Minute
			if w.Spec.ReconcileTimeout != nil {
				timeout = w.Spec.ReconcileTimeout.Duration
			}
			go func() {
				pr, err := rec.Reconcile(ctx, reconciler.Input{
					WatcherName: w.Spec.AppName,
					Namespace:   w.Spec.Namespace,
					TargetRepo:  w.Spec.TargetRepo,
					FixRepo:     w.Spec.FixRepo,
					DocsPaths:   w.Spec.DocsPaths,
					Timeout:     timeout,
				})

				// Update the GitOpsWatcher status so results are visible via kubectl/oc.
				now := metav1.Now()
				w.Status.LastReconcileTime = &now
				if err != nil {
					slog.Error("reconcile", "watcher", w.Name, "err", err)
					w.Status.LastResult = "error"
				} else if pr != nil {
					slog.Info("fix PR opened", "watcher", w.Name, "pr", pr.URL)
					w.Status.LastResult = "fix_pr_opened"
					w.Status.LastFixPRURL = pr.URL
				} else {
					slog.Info("reconcile complete, no action needed", "watcher", w.Name)
					w.Status.LastResult = "success"
				}
				if statusErr := rawClient.Status().Update(ctx, &w); statusErr != nil {
					slog.Error("update watcher status", "watcher", w.Name, "err", statusErr)
				}
			}()
		}
	}

	// webhookMux handles inbound GitHub webhook traffic only.
	webhookMux := http.NewServeMux()
	webhookMux.Handle("/webhook", webhook.NewHandler(cfg.webhookSecret, dispatch))

	// metricsMux exposes Prometheus metrics and Kubernetes health probes on a
	// dedicated port so that scrape traffic is independent of webhook ingress.
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	metricsMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	webhookSrv := &http.Server{
		Addr:         cfg.listenAddr,
		Handler:      webhookMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	metricsSrv := &http.Server{
		Addr:         cfg.metricsAddr,
		Handler:      metricsMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		slog.Info("starting webhook server", "addr", cfg.listenAddr)
		if err := webhookSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("webhook server", "err", err)
			stop()
		}
	}()
	go func() {
		slog.Info("starting metrics server", "addr", cfg.metricsAddr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := webhookSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("webhook server shutdown: %w", err)
	}
	return metricsSrv.Shutdown(shutdownCtx)
}

type appConfig struct {
	listenAddr    string
	metricsAddr   string
	webhookSecret string
	githubToken   string
	claudeAPIKey  string
	claudeModel   string
}

func loadConfig() appConfig {
	cfg := appConfig{
		listenAddr:  getEnv("GORT_LISTEN_ADDR", ":8080"),
		metricsAddr: getEnv("GORT_METRICS_ADDR", ":8081"),
		claudeModel: getEnv("GORT_CLAUDE_MODEL", "claude-sonnet-4-6"),
	}
	cfg.webhookSecret = mustEnv("GORT_WEBHOOK_SECRET")
	cfg.githubToken = mustEnv("GORT_GITHUB_TOKEN")
	cfg.claudeAPIKey = mustEnv("GORT_CLAUDE_API_KEY")
	return cfg
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}
