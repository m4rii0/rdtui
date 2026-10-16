package realdebrid

import (
	"context"
	"net/http"
	"time"
)

// TokenSource obtains the current bearer token for an authenticated request.
type TokenSource func(context.Context) (string, error)

// ClientOption configures a client at construction time.
type ClientOption func(*Client)

// WithNow supplies the clock used to timestamp OAuth credentials/device codes.
func WithNow(now func() time.Time) ClientOption {
	return func(c *Client) {
		if now != nil {
			c.now = now
		}
	}
}

// WithTokenSource replaces the static token with a request-time token source.
func WithTokenSource(source TokenSource) ClientOption {
	return func(c *Client) { c.tokenSource = source }
}

// WithHTTPClient supplies the HTTP client. A nil client retains the default.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithAuthErrorHandler observes authenticated HTTP 401 responses and the token
// sent. Its error is joined with the API error; requests are never replayed.
func WithAuthErrorHandler(handler func(token string, err *APIError) error) ClientOption {
	return func(c *Client) { c.authErrorHandler = handler }
}
