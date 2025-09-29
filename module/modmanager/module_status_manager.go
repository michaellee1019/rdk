package modmanager

import (
	"context"
	"fmt"
	"sync"

	"go.viam.com/rdk/config"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	robotstatus "go.viam.com/rdk/robot/status"
)

// ModuleStatusManager manages module resources in the resource graph for status tracking.
// It also implements the StatusReporter interface for package managers.
type ModuleStatusManager struct {
	resourceGraph ResourceGraphInterface
	modules       map[string]*ModuleResource
	mu            sync.RWMutex
	logger        logging.Logger
}

// Ensure ModuleStatusManager implements StatusReporter interface.
var _ robotstatus.StatusReporter = (*ModuleStatusManager)(nil)

// ResourceGraphInterface defines the interface we need from the resource graph.
type ResourceGraphInterface interface {
	AddNode(name resource.Name, node *resource.GraphNode) error
	Node(name resource.Name) (*resource.GraphNode, bool)
}

// NewModuleStatusManager creates a new module status manager.
func NewModuleStatusManager(resourceGraph ResourceGraphInterface, logger logging.Logger) *ModuleStatusManager {
	return &ModuleStatusManager{
		resourceGraph: resourceGraph,
		modules:       make(map[string]*ModuleResource),
		logger:        logger,
	}
}

// CreateModuleResource creates a module resource and adds it to the resource graph.
func (msm *ModuleStatusManager) CreateModuleResource(ctx context.Context, cfg config.Module) error {
	msm.mu.Lock()
	defer msm.mu.Unlock()

	moduleName := resource.NewName(ModuleAPI, cfg.Name)

	// Check if module resource already exists
	if existingResource, exists := msm.modules[cfg.Name]; exists {
		// Update the existing resource with new config if needed
		existingResource.cfg = cfg
		msm.logger.Debugw("Updated existing module resource", "module", cfg.Name)
		return nil
	}

	// Check if node already exists in resource graph
	if existingNode, exists := msm.resourceGraph.Node(moduleName); exists {
		// If node exists, create new resource and swap it
		moduleResource := NewModuleResource(cfg, msm.logger.Sublogger("module_"+cfg.Name))
		msm.modules[cfg.Name] = moduleResource

		existingNode.SwapResource(moduleResource, resource.DefaultModelFamily.WithModel("builtin"), nil)
		msm.logger.Debugw("Swapped existing module resource", "module", cfg.Name)
		return nil
	}

	// Create new module resource and node
	moduleResource := NewModuleResource(cfg, msm.logger.Sublogger("module_"+cfg.Name))
	msm.modules[cfg.Name] = moduleResource

	// Add to resource graph
	node := resource.NewConfiguredGraphNode(
		resource.Config{Name: cfg.Name, API: ModuleAPI},
		moduleResource,
		resource.DefaultModelFamily.WithModel("builtin"),
	)

	if err := msm.resourceGraph.AddNode(moduleName, node); err != nil {
		delete(msm.modules, cfg.Name)
		return fmt.Errorf("failed to add module resource %s to graph: %w", cfg.Name, err)
	}

	msm.logger.Debugw("Created module resource", "module", cfg.Name)
	return nil
}

// UpdatePackageStatus updates the package status for a module.
func (msm *ModuleStatusManager) UpdatePackageStatus(moduleName string, packageStatus robotstatus.PackageLifecycleStatus) error {
	msm.mu.RLock()
	moduleResource, exists := msm.modules[moduleName]
	msm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("module resource %s not found", moduleName)
	}

	moduleResource.UpdatePackageStatus(packageStatus)

	// Update the resource in the resource graph to trigger status change
	return msm.updateResourceInGraph(moduleName, moduleResource)
}

// UpdateModuleStatus updates the module status for a module.
func (msm *ModuleStatusManager) UpdateModuleStatus(moduleName string, moduleStatus robotstatus.ModuleLifecycleStatus) error {
	msm.mu.RLock()
	moduleResource, exists := msm.modules[moduleName]
	msm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("module resource %s not found", moduleName)
	}

	moduleResource.UpdateModuleStatus(moduleStatus)

	// Update the resource in the resource graph to trigger status change
	return msm.updateResourceInGraph(moduleName, moduleResource)
}

// GetModuleResource returns the module resource for a given module name.
func (msm *ModuleStatusManager) GetModuleResource(moduleName string) (*ModuleResource, bool) {
	msm.mu.RLock()
	defer msm.mu.RUnlock()

	moduleResource, exists := msm.modules[moduleName]
	return moduleResource, exists
}

// ListModuleResources returns all module resources.
func (msm *ModuleStatusManager) ListModuleResources() map[string]*ModuleResource {
	msm.mu.RLock()
	defer msm.mu.RUnlock()

	result := make(map[string]*ModuleResource, len(msm.modules))
	for name, resource := range msm.modules {
		result[name] = resource
	}
	return result
}

// RemoveModuleResource removes a module resource from tracking and marks it for removal from the resource graph.
func (msm *ModuleStatusManager) RemoveModuleResource(moduleName string) {
	msm.mu.Lock()
	defer msm.mu.Unlock()

	// Mark the resource for removal in the resource graph
	resourceName := resource.NewName(ModuleAPI, moduleName)
	if node, exists := msm.resourceGraph.Node(resourceName); exists {
		node.MarkForRemoval()
		msm.logger.Debugw("Marked module resource for removal", "module", moduleName)
	}

	delete(msm.modules, moduleName)
	msm.logger.Debugw("Removed module resource from tracking", "module", moduleName)
}

// ReportPackageStatus implements the StatusReporter interface for package managers.
func (msm *ModuleStatusManager) ReportPackageStatus(moduleName string, packageStatus robotstatus.PackageLifecycleStatus) error {
	return msm.UpdatePackageStatus(moduleName, packageStatus)
}

// updateResourceInGraph updates the resource in the resource graph to trigger status updates.
func (msm *ModuleStatusManager) updateResourceInGraph(moduleName string, moduleResource *ModuleResource) error {
	resourceName := resource.NewName(ModuleAPI, moduleName)

	// Get the existing node and swap the resource to trigger status update
	if node, exists := msm.resourceGraph.Node(resourceName); exists {
		// Use SwapResource to update the resource in the existing node
		node.SwapResource(moduleResource, resource.DefaultModelFamily.WithModel("builtin"), nil)
	}

	return nil
}
