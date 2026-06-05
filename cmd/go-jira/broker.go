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

	"github.com/appleboy/go-jira/pkg/broker"
	"github.com/appleboy/go-jira/pkg/oauth"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

// brokerReadHeaderTimeout bounds how long the broker waits for request headers,
// guarding against slow-loris clients on the (internal) listener.
const brokerReadHeaderTimeout = 10 * time.Second

// brokerShutdownTimeout bounds graceful shutdown after a termination signal.
const brokerShutdownTimeout = 10 * time.Second

// newBrokerCmd builds the `broker` command group. Today it has one subcommand,
// `serve`, which runs the token refresh broker: a server-side service that holds
// the confidential client_secret and performs the secret-bearing refresh on a
// client's behalf. The secret is read ONLY from the environment, never a flag.
func newBrokerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "broker",
		Short:   "Run the OAuth token refresh broker (holds the client secret server-side)",
		GroupID: groupAuth,
		Long: `Run the OAuth token refresh broker.

A confidential Jira DC OAuth app requires the client_secret on the refresh step.
go-jira ships as a public PKCE client and must never embed that secret. The
broker is a small server-side service that holds the client_secret (injected
from the environment, e.g. a Kubernetes Secret sourced from Vault) and performs
only the secret-bearing refresh on a client's behalf. Clients set
JIRA_TOKEN_BROKER_URL and send their refresh_token; the broker adds the secret
and returns the rotated token pair.

The broker does not persist tokens at rest; it keeps only a short-TTL in-memory
cache that coalesces concurrent refreshes of the same refresh_token into one
upstream call (handling Jira DC's refresh-token rotation race).`,
		SilenceUsage: true,
	}
	cmd.AddCommand(newBrokerServeCmd())
	return cmd
}

func newBrokerServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the token refresh broker HTTP server",
		Long: `Start the token refresh broker HTTP server.

Required environment (fail-fast if missing):
  JIRA_BASE_URL             Jira DC base URL
  JIRA_OAUTH_CLIENT_ID      OAuth client ID
  JIRA_OAUTH_CLIENT_SECRET  confidential client secret (read ONLY from env)

Optional:
  JIRA_BROKER_TOKEN         caller bearer token; enforced only when set
  JIRA_BROKER_LISTEN        listen address (default ` + defaultBrokerListen + `)
  JIRA_BROKER_TLS_CERT/KEY  serve HTTPS directly (else terminate TLS at ingress)

Endpoints: POST /v1/refresh, GET /healthz, GET /readyz.`,
		Example: `  # Run the broker (secret comes from the environment)
  JIRA_BASE_URL=https://jira.example.com \
  JIRA_OAUTH_CLIENT_ID=my-client \
  JIRA_OAUTH_CLIENT_SECRET=… \
  go-jira broker serve --listen :8080`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBrokerServe(cmd)
		},
	}
	// base-url / insecure / client-id come from the common + a single client-id
	// flag (env still wins). The secret and caller token are env-only.
	addCommonFlags(cmd)
	cmd.Flags().String(flagClientID, "", "OAuth client ID (env: "+envOAuthClientID+")")
	cmd.Flags().String(flagListen, defaultBrokerListen,
		"Listen address (env: "+envBrokerListen+")")
	cmd.Flags().String(flagTLSCert, "",
		"TLS cert file to serve HTTPS directly (env: "+envBrokerTLSCert+
			"); requires --"+flagTLSKey)
	cmd.Flags().String(flagTLSKey, "",
		"TLS key file to serve HTTPS directly (env: "+envBrokerTLSKey+
			"); requires --"+flagTLSCert)
	return cmd
}

func runBrokerServe(cmd *cobra.Command) error {
	if err := loadEnvFromCmd(cmd); err != nil {
		return err
	}
	config := loadConfig(cmd)

	// Fail-fast on the required configuration so a misconfigured broker never
	// starts and silently fails every refresh.
	if err := requireBaseURL(config); err != nil {
		return err
	}
	if config.oauthClientID == "" {
		return errors.New("broker: OAuth client ID required: set " +
			envOAuthClientID + " or pass --client-id")
	}
	secret := os.Getenv(envOAuthClientSecret)
	if secret == "" {
		return errors.New("broker: client secret required: set " + envOAuthClientSecret)
	}

	// The broker's oauth.Config carries the secret and leaves BrokerURL empty, so
	// its Refresh calls Jira DC directly with the secret on the body. It reuses
	// oauthHTTPClient so it trusts the internal CA (and honours --insecure).
	oc := &oauth.Config{
		BaseURL:      config.baseURL,
		ClientID:     config.oauthClientID,
		ClientSecret: secret,
		HTTPClient:   oauthHTTPClient(config),
	}

	srv, err := broker.NewServer(broker.Options{
		Refresh:     brokerRefreshFunc(oc),
		ClientID:    config.oauthClientID,
		CallerToken: config.brokerToken,
		Ready: func() error {
			if oc.ClientSecret == "" {
				return errors.New("client secret not configured")
			}
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("broker: %w", err)
	}

	listen := resolveWithEnv(envBrokerListen, flagStringValue(cmd, flagListen), defaultBrokerListen)
	tlsCert := resolveWithEnv(envBrokerTLSCert, flagStringValue(cmd, flagTLSCert), "")
	tlsKey := resolveWithEnv(envBrokerTLSKey, flagStringValue(cmd, flagTLSKey), "")
	if (tlsCert == "") != (tlsKey == "") {
		return errors.New("broker: both --" + flagTLSCert + " and --" + flagTLSKey +
			" are required to serve HTTPS")
	}

	httpSrv := &http.Server{
		Addr:              listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: brokerReadHeaderTimeout,
	}
	return serveBroker(cmdContext(cmd), httpSrv, config, tlsCert, tlsKey)
}

// serveBroker runs httpSrv until a termination signal (or the parent context)
// fires, then shuts it down gracefully.
func serveBroker(
	parent context.Context, httpSrv *http.Server, config Config, tlsCert, tlsKey string,
) error {
	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	useTLS := tlsCert != "" && tlsKey != ""
	if !useTLS {
		slog.Warn("broker serving plain HTTP; terminate TLS at the ingress for transport security",
			"listen", httpSrv.Addr)
	}
	slog.Info("broker listening",
		"listen", httpSrv.Addr, "base_url", config.baseURL,
		"caller_token_required", config.brokerToken != "", "tls", useTLS)

	errCh := make(chan error, 1)
	go func() {
		if useTLS {
			errCh <- httpSrv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			errCh <- httpSrv.ListenAndServe()
		}
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("broker: serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		slog.Info("broker shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), brokerShutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("broker: shutdown: %w", err)
		}
		return nil
	}
}

// brokerRefreshFunc adapts oauth.Config.Refresh into a broker.RefreshFunc,
// translating oauth's sentinel errors into the HTTP status / error code the
// broker should return. This is the ONLY place the broker's status mapping lives
// (pkg/broker stays free of any pkg/oauth dependency to avoid an import cycle).
func brokerRefreshFunc(oc *oauth.Config) broker.RefreshFunc {
	return func(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
		tok, err := oc.Refresh(ctx, refreshToken)
		if err == nil {
			return tok, nil
		}
		switch {
		case errors.Is(err, oauth.ErrInvalidGrant):
			// Caller's refresh token is expired/revoked — their problem.
			return nil, &broker.APIError{
				Status: http.StatusBadRequest, Code: "invalid_grant", Err: err,
			}
		case errors.Is(err, oauth.ErrInvalidClient):
			// The broker's own secret is misconfigured — a broker problem.
			return nil, &broker.APIError{
				Status: http.StatusBadGateway, Code: "invalid_client", Err: err,
			}
		default:
			// invalid (upstream) server errors, timeouts, transport failures.
			return nil, &broker.APIError{
				Status: http.StatusServiceUnavailable, Code: "server_error", Err: err,
			}
		}
	}
}
