package modmanager

import (
	"context"
	"sync"
	"time"

	"go.viam.com/rdk/config"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	robotstatus "go.viam.com/rdk/robot/status"
)

// ModuleAPI is the API for module resources.
var ModuleAPI = resource.APINamespaceRDKInternal.WithServiceType("module")

// ModuleResource represents a module as a resource in the resource graph.
// This allows tracking module lifecycle status including package sync status.
type ModuleResource struct {
	resource.Named
	resource.TriviallyReconfigurable
	resource.TriviallyCloseable

	cfg           config.Module
	packageStatus robotstatus.PackageLifecycleStatus
	moduleStatus  robotstatus.ModuleLifecycleStatus
	mu            sync.RWMutex
	logger        logging.Logger
}

// ModuleDetailedStatus provides comprehensive status information for a module.
type ModuleDetailedStatus struct {
	resource.NodeStatus
	robotstatus.ModuleDetailedStatus
}

// NewModuleResource creates a new module resource.
func NewModuleResource(cfg config.Module, logger logging.Logger) *ModuleResource {
	name := resource.NewName(ModuleAPI, cfg.Name)

	// Initialize package status based on module type
	packageStatus := robotstatus.PackageLifecycleStatus{
		State:       robotstatus.PackageStateNotNeeded,
		LastUpdated: time.Now(),
	}
	if cfg.Type == config.ModuleTypeRegistry || cfg.NeedsSyntheticPackage() {
		packageStatus.State = robotstatus.PackageStatePending
	}

	return &ModuleResource{
		Named:         name.AsNamed(),
		cfg:           cfg,
		logger:        logger,
		packageStatus: packageStatus,
		moduleStatus: robotstatus.ModuleLifecycleStatus{
			State:       robotstatus.ModuleStatePending,
			LastUpdated: time.Now(),
		},
	}
}

// UpdatePackageStatus updates the package sync status for this module.
func (mr *ModuleResource) UpdatePackageStatus(packageStatus robotstatus.PackageLifecycleStatus) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.packageStatus = packageStatus
	mr.logger.Debugw("Module package status updated",
		"module", mr.cfg.Name,
		"package_state", packageStatus.State,
		"error", packageStatus.Error)
}

// UpdateModuleStatus updates the module process status for this module.
func (mr *ModuleResource) UpdateModuleStatus(moduleStatus robotstatus.ModuleLifecycleStatus) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	mr.moduleStatus = moduleStatus
	mr.logger.Debugw("Module status updated",
		"module", mr.cfg.Name,
		"module_state", moduleStatus.State,
		"error", moduleStatus.Error)
}

// GetPackageStatus returns the current package status.
func (mr *ModuleResource) GetPackageStatus() robotstatus.PackageLifecycleStatus {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return mr.packageStatus
}

// GetModuleStatus returns the current module status.
func (mr *ModuleResource) GetModuleStatus() robotstatus.ModuleLifecycleStatus {
	mr.mu.RLock()
	defer mr.mu.RUnlock()
	return mr.moduleStatus
}

// DetailedStatus returns comprehensive status information for this module.
func (mr *ModuleResource) DetailedStatus() ModuleDetailedStatus {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	return ModuleDetailedStatus{
		NodeStatus: mr.nodeStatus(),
		ModuleDetailedStatus: robotstatus.ModuleDetailedStatus{
			ModuleName:    mr.cfg.Name,
			ModuleType:    mr.cfg.Type,
			ModuleID:      mr.cfg.ModuleID,
			PackageStatus: mr.packageStatus,
			ModuleStatus:  mr.moduleStatus,
		},
	}
}

// nodeStatus determines the overall NodeStatus based on package and module status.
func (mr *ModuleResource) nodeStatus() resource.NodeStatus {
	var state resource.NodeState
	var err error
	var lastUpdated time.Time

	// Priority order: package failures > module failures > package downloading > module states > ready
	switch {
	case mr.packageStatus.State == robotstatus.PackageStateFailed:
		state = resource.NodeStateUnhealthy
		err = mr.packageStatus.Error
		lastUpdated = mr.packageStatus.LastUpdated
	case mr.moduleStatus.State == robotstatus.ModuleStateFailed:
		state = resource.NodeStateUnhealthy
		err = mr.moduleStatus.Error
		lastUpdated = mr.moduleStatus.LastUpdated
	case mr.packageStatus.State == robotstatus.PackageStateDownloading:
		state = resource.NodeStateConfiguring // Use existing state for downloading
		lastUpdated = mr.packageStatus.LastUpdated
	case mr.moduleStatus.State == robotstatus.ModuleStateFirstRun:
		state = resource.NodeStateConfiguring
		lastUpdated = mr.moduleStatus.LastUpdated
	case mr.moduleStatus.State == robotstatus.ModuleStateStarting:
		state = resource.NodeStateConfiguring
		lastUpdated = mr.moduleStatus.LastUpdated
	case mr.moduleStatus.State == robotstatus.ModuleStateRunning:
		state = resource.NodeStateReady
		lastUpdated = mr.moduleStatus.LastUpdated
	case mr.moduleStatus.State == robotstatus.ModuleStateStopping:
		state = resource.NodeStateRemoving
		lastUpdated = mr.moduleStatus.LastUpdated
	default:
		state = resource.NodeStateConfiguring
		lastUpdated = time.Now()
	}

	return resource.NodeStatus{
		Name:        mr.Name(),
		State:       state,
		LastUpdated: lastUpdated,
		Revision:    mr.cfg.LocalVersion,
		Error:       err,
	}
}

// DoCommand implements the DoCommand interface for modules.
func (mr *ModuleResource) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	if cmd["get_detailed_status"] != nil {
		status := mr.DetailedStatus()
		return map[string]interface{}{
			"detailed_status": status,
		}, nil
	}

	return nil, resource.ErrDoUnimplemented
}
