// Package download defines the lifecycle contract shared by download backends.
package download

import (
	"context"
	"errors"

	"github.com/m4rii0/rdtui/pkg/models"
)

var (
	ErrClosed         = errors.New("download backend is closed")
	ErrUncertainStart = errors.New("download start outcome is uncertain; restart the application")
	// ErrBackendLost indicates confirmed backend termination, not a query failure.
	ErrBackendLost = errors.New("download backend terminated")
)

type Backend interface {
	// StartDownload's context owns the transfer lifetime, not just admission.
	StartDownload(context.Context, models.ManagedDownloadRequest) (models.ManagedDownload, error)
	DownloadStatus(context.Context, string) (models.ManagedDownload, error)
	// Shutdown closes admission once. A caller deadline does not abandon cleanup;
	// another caller can wait for the same completion.
	Shutdown(context.Context) error
}
