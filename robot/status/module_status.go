// Package status defines status types shared between packages and modules to avoid import cycles.
package status

import (
	"time"

	"go.viam.com/rdk/config"
)

// PackageLifecycleStatus tracks the package sync status for a module.
type PackageLifecycleStatus struct {
	State       PackageState
	LastUpdated time.Time
	Error       error
	Progress    *PackageProgress
}

// ModuleLifecycleStatus tracks the module process lifecycle status.
type ModuleLifecycleStatus struct {
	State       ModuleState
	LastUpdated time.Time
	Error       error
}

// PackageProgress tracks download progress for packages.
type PackageProgress struct {
	BytesDownloaded int64
	TotalBytes      int64
	Percentage      float64
}

// PackageState represents the current state of package sync for a module.
type PackageState string

const (
	// PackageStateNotNeeded indicates the module doesn't require package sync (local non-tarball).
	PackageStateNotNeeded PackageState = "not_needed"
	// PackageStatePending indicates package sync is queued but not started.
	PackageStatePending PackageState = "pending"
	// PackageStateDownloading indicates package is currently being downloaded.
	PackageStateDownloading PackageState = "downloading"
	// PackageStateReady indicates package sync completed successfully.
	PackageStateReady PackageState = "ready"
	// PackageStateFailed indicates package sync failed.
	PackageStateFailed PackageState = "failed"
)

// ModuleState represents the current state of the module process lifecycle.
type ModuleState string

const (
	// ModuleStatePending indicates module is configured but not yet started.
	ModuleStatePending ModuleState = "pending"
	// ModuleStateFirstRun indicates module is executing first run script.
	ModuleStateFirstRun ModuleState = "first_run"
	// ModuleStateStarting indicates module process is starting, waiting for ready.
	ModuleStateStarting ModuleState = "starting"
	// ModuleStateRunning indicates module is ready and serving resources.
	ModuleStateRunning ModuleState = "running"
	// ModuleStateStopping indicates module is shutting down.
	ModuleStateStopping ModuleState = "stopping"
	// ModuleStateFailed indicates module failed to start or crashed.
	ModuleStateFailed ModuleState = "failed"
)

// ModuleDetailedStatus provides comprehensive status information for a module.
type ModuleDetailedStatus struct {
	ModuleName    string
	ModuleType    config.ModuleType
	ModuleID      string
	PackageStatus PackageLifecycleStatus
	ModuleStatus  ModuleLifecycleStatus
}

// StatusReporter is an interface for reporting package sync status to module resources.
type StatusReporter interface {
	// ReportPackageStatus reports the package sync status for a module.
	ReportPackageStatus(moduleName string, status PackageLifecycleStatus) error
}

// NoOpStatusReporter is a status reporter that does nothing.
type NoOpStatusReporter struct{}

// ReportPackageStatus does nothing.
func (n NoOpStatusReporter) ReportPackageStatus(moduleName string, status PackageLifecycleStatus) error {
	return nil
}

// NewNoOpStatusReporter creates a new no-op status reporter.
func NewNoOpStatusReporter() StatusReporter {
	return NoOpStatusReporter{}
}
