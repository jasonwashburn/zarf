// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The Zarf Authors

// Package http provides a http server for the webhook and proxy.
package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/defenseunicorns/zarf/src/internal/agent/hooks"
	"github.com/defenseunicorns/zarf/src/internal/agent/http/admission"
	"github.com/defenseunicorns/zarf/src/pkg/cluster"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewAdmissionServer creates a http.Server for the mutating webhook admission handler.
func NewAdmissionServer(ctx context.Context, port string) (*http.Server, error) {
	message.Debugf("http.NewAdmissionServer(%s)", port)

	c, err := cluster.NewCluster()
	if err != nil {
		return nil, err
	}

	// Instances hooks
	podsMutation := hooks.NewPodMutationHook(ctx, c)
	fluxGitRepositoryMutation := hooks.NewGitRepositoryMutationHook(ctx, c)
	argocdApplicationMutation := hooks.NewApplicationMutationHook(ctx, c)
	argocdRepositoryMutation := hooks.NewRepositorySecretMutationHook(ctx, c)

	// Routers
	ah := admission.NewHandler()
	mux := http.NewServeMux()
	mux.Handle("/healthz", healthz())
	mux.Handle("/mutate/pod", ah.Serve(podsMutation))
	mux.Handle("/mutate/flux-gitrepository", ah.Serve(fluxGitRepositoryMutation))
	mux.Handle("/mutate/argocd-application", ah.Serve(argocdApplicationMutation))
	mux.Handle("/mutate/argocd-repository", ah.Serve(argocdRepositoryMutation))
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second, // Set ReadHeaderTimeout to avoid Slowloris attacks
	}
	return srv, nil
}

// NewProxyServer creates and returns an http proxy server.
func NewProxyServer(port string) *http.Server {
	message.Debugf("http.NewHTTPProxy(%s)", port)

	mux := http.NewServeMux()
	mux.Handle("/healthz", healthz())
	mux.Handle("/", ProxyHandler())
	mux.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second, // Set ReadHeaderTimeout to avoid Slowloris attacks
	}
}

func healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		//nolint: errcheck // ignore
		w.Write([]byte("ok"))
	}
}
