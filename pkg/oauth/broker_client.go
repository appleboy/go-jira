package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/appleboy/go-jira/pkg/broker"

	"golang.org/x/oauth2"
)

// maxBrokerResponse caps how much of the broker response body is read, both for
// success and error bodies; the JSON token/error payloads are small.
const maxBrokerResponse = 1 << 20 // 1 MiB

// refreshViaBroker performs the refresh through the token refresh broker instead
// of calling Jira DC directly. The client never holds the client_secret: it
// sends only its refresh_token (and, optionally, its bearer token), and the
// broker adds the secret upstream and returns the rotated pair.
//
// Broker responses are mapped back to this package's sentinel errors so the
// auto-refresh path and the CLI's recovery hints behave identically to the
// direct path (e.g. invalid_grant still tells the user to run `go-jira login`).
func (c *Config) refreshViaBroker(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	//nolint:gosec // G117: serialising the refresh token is this function's
	// purpose — it is the payload the broker needs to perform the refresh.
	payload, err := json.Marshal(broker.RefreshRequest{
		RefreshToken: refreshToken,
		ClientID:     c.ClientID,
	})
	if err != nil {
		return nil, fmt.Errorf("oauth: marshal broker request: %w", err)
	}

	url := strings.TrimRight(c.BrokerURL, "/") + broker.RefreshPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("oauth: build broker request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.BrokerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.BrokerToken)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		// A transport-level failure (DNS, connection refused, timeout) is an
		// upstream/availability problem, not a credential one.
		return nil, fmt.Errorf("%w: broker request failed: %v", ErrServerError, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBrokerResponse))

	if resp.StatusCode == http.StatusOK {
		var tr broker.TokenResponse
		if err := json.Unmarshal(body, &tr); err != nil {
			return nil, fmt.Errorf("oauth: decode broker response: %w", err)
		}
		if tr.AccessToken == "" {
			return nil, errors.New("oauth: broker returned an empty access token")
		}
		return brokerTokenToOAuth2(tr, time.Now()), nil
	}
	return nil, mapBrokerError(resp.StatusCode, body)
}

// brokerTokenToOAuth2 converts the broker's wire response into an *oauth2.Token,
// reconstructing the absolute expiry from the relative expires_in so callers
// (storage.NewStoredToken) record the correct expiry and rotated refresh token.
func brokerTokenToOAuth2(tr broker.TokenResponse, now time.Time) *oauth2.Token {
	tok := &oauth2.Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
	}
	if tr.ExpiresIn > 0 {
		tok.Expiry = now.Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tok
}

// mapBrokerError translates a non-200 broker response into a sentinel error
// where one applies, mirroring mapError's mapping for the direct path:
//
//   - 400 invalid_grant → ErrInvalidGrant (refresh token expired/revoked)
//   - 401              → caller-credential error (broker token missing/wrong)
//   - 502 invalid_client → ErrInvalidClient (broker's own secret is misconfigured)
//   - any other 5xx    → ErrServerError (upstream timeout / broker fault), matching
//     mapError, which maps every direct-path 5xx to ErrServerError
func mapBrokerError(status int, body []byte) error {
	var er broker.ErrorResponse
	_ = json.Unmarshal(body, &er)

	switch status {
	case http.StatusBadRequest:
		if er.Error == codeInvalidGrant {
			return fmt.Errorf("%w: %s", ErrInvalidGrant, er.ErrorDescription)
		}
		return fmt.Errorf("oauth: broker rejected the request (%s): %s",
			brokerErrCode(er, status), er.ErrorDescription)
	case http.StatusUnauthorized:
		return fmt.Errorf(
			"oauth: broker rejected the caller credential (status 401); " +
				"set or correct the broker token")
	case http.StatusBadGateway:
		return fmt.Errorf("%w: broker upstream reported invalid_client "+
			"(the broker's client secret is misconfigured)", ErrInvalidClient)
	}
	// Any remaining 5xx (503 upstream-unavailable, 500 broker-internal, 504
	// gateway-timeout, …) is a server/availability fault, classified like the
	// direct path so callers and CLI hints behave consistently.
	if status >= http.StatusInternalServerError {
		return fmt.Errorf("%w: broker returned status %d (%s)",
			ErrServerError, status, brokerErrCode(er, status))
	}
	return fmt.Errorf("oauth: broker returned unexpected status %d (%s)",
		status, brokerErrCode(er, status))
}

// brokerErrCode returns the OAuth2 error code from the body, or a placeholder.
func brokerErrCode(er broker.ErrorResponse, status int) string {
	if er.Error != "" {
		return er.Error
	}
	return fmt.Sprintf("http_%d", status)
}
