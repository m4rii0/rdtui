package realdebrid

import (
	"context"
	"net/http"
)

// TokenSource obtains the current bearer token for an authenticated request.
type TokenSource func(context.Context) (string, error)

// ClientOption configures a client at construction time.
type ClientOption func(*Client)

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
