package modmanager

import (
	"context"
	"testing"
	"time"

	"go.viam.com/test"

	"go.viam.com/rdk/config"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

// mockResourceGraph implements ResourceGraphInterface for testing.
type mockResourceGraph struct {
	nodes map[resource.Name]*resource.GraphNode
}

func newMockResourceGraph() *mockResourceGraph {
	return &mockResourceGraph{
		nodes: make(map[resource.Name]*resource.GraphNode),
	}
}

func (m *mockResourceGraph) AddNode(name resource.Name, node *resource.GraphNode) error {
	m.nodes[name] = node
	return nil
}

func (m *mockResourceGraph) Node(name resource.Name) (*resource.GraphNode, bool) {
	node, exists := m.nodes[name]
	return node, exists
}

func (m *mockResourceGraph) ReplaceNodesResource(name resource.Name, res resource.Resource) error {
	if node, exists := m.nodes[name]; exists {
		// In a real implementation, this would update the resource in the graph node
		// For testing, we'll just verify the resource is the expected type
		if _, ok := res.(*ModuleResource); !ok {
			return test.ErrFail
		}
		// Update the node (simplified for testing)
		m.nodes[name] = node
	}
	return nil
}

func TestNewModuleStatusManager(t *testing.T) {
	logger := logging.NewTestLogger(t)
	resourceGraph := newMockResourceGraph()

	manager := NewModuleStatusManager(resourceGraph, logger)

	test.That(t, manager, test.ShouldNotBeNil)
	test.That(t, manager.resourceGraph, test.ShouldEqual, resourceGraph)
	test.That(t, manager.logger, test.ShouldEqual, logger)
	test.That(t, len(manager.modules), test.ShouldEqual, 0)
}

func TestModuleStatusManagerCreateModuleResource(t *testing.T) {
	logger := logging.NewTestLogger(t)
	resourceGraph := newMockResourceGraph()
	manager := NewModuleStatusManager(resourceGraph, logger)

	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	err := manager.CreateModuleResource(context.Background(), cfg)
	test.That(t, err, test.ShouldBeNil)

	// Verify module resource was created and added to graph
	moduleResource, exists := manager.GetModuleResource("test-module")
	test.That(t, exists, test.ShouldBeTrue)
	test.That(t, moduleResource.cfg.Name, test.ShouldEqual, "test-module")

	// Verify it was added to the resource graph
	moduleName := resource.NewName(ModuleAPI, "test-module")
	_, exists = resourceGraph.Node(moduleName)
	test.That(t, exists, test.ShouldBeTrue)
}

func TestModuleStatusManagerUpdatePackageStatus(t *testing.T) {
	logger := logging.NewTestLogger(t)
	resourceGraph := newMockResourceGraph()
	manager := NewModuleStatusManager(resourceGraph, logger)

	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	err := manager.CreateModuleResource(context.Background(), cfg)
	test.That(t, err, test.ShouldBeNil)

	// Update package status
	newStatus := PackageLifecycleStatus{
		State:       PackageStateDownloading,
		LastUpdated: time.Now(),
	}

	err = manager.UpdatePackageStatus("test-module", newStatus)
	test.That(t, err, test.ShouldBeNil)

	// Verify status was updated
	moduleResource, exists := manager.GetModuleResource("test-module")
	test.That(t, exists, test.ShouldBeTrue)

	updatedStatus := moduleResource.GetPackageStatus()
	test.That(t, updatedStatus.State, test.ShouldEqual, PackageStateDownloading)
}

func TestModuleStatusManagerUpdateModuleStatus(t *testing.T) {
	logger := logging.NewTestLogger(t)
	resourceGraph := newMockResourceGraph()
	manager := NewModuleStatusManager(resourceGraph, logger)

	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	err := manager.CreateModuleResource(context.Background(), cfg)
	test.That(t, err, test.ShouldBeNil)

	// Update module status
	newStatus := ModuleLifecycleStatus{
		State:       ModuleStateRunning,
		LastUpdated: time.Now(),
	}

	err = manager.UpdateModuleStatus("test-module", newStatus)
	test.That(t, err, test.ShouldBeNil)

	// Verify status was updated
	moduleResource, exists := manager.GetModuleResource("test-module")
	test.That(t, exists, test.ShouldBeTrue)

	updatedStatus := moduleResource.GetModuleStatus()
	test.That(t, updatedStatus.State, test.ShouldEqual, ModuleStateRunning)
}

func TestModuleStatusManagerReportPackageStatus(t *testing.T) {
	logger := logging.NewTestLogger(t)
	resourceGraph := newMockResourceGraph()
	manager := NewModuleStatusManager(resourceGraph, logger)

	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	err := manager.CreateModuleResource(context.Background(), cfg)
	test.That(t, err, test.ShouldBeNil)

	// Test StatusReporter interface
	newStatus := PackageLifecycleStatus{
		State:       PackageStateReady,
		LastUpdated: time.Now(),
	}

	err = manager.ReportPackageStatus("test-module", newStatus)
	test.That(t, err, test.ShouldBeNil)

	// Verify status was updated
	moduleResource, exists := manager.GetModuleResource("test-module")
	test.That(t, exists, test.ShouldBeTrue)

	updatedStatus := moduleResource.GetPackageStatus()
	test.That(t, updatedStatus.State, test.ShouldEqual, PackageStateReady)
}

func TestModuleStatusManagerListModuleResources(t *testing.T) {
	logger := logging.NewTestLogger(t)
	resourceGraph := newMockResourceGraph()
	manager := NewModuleStatusManager(resourceGraph, logger)

	// Create multiple module resources
	modules := []config.Module{
		{Name: "module1", Type: config.ModuleTypeRegistry},
		{Name: "module2", Type: config.ModuleTypeLocal},
	}

	for _, cfg := range modules {
		err := manager.CreateModuleResource(context.Background(), cfg)
		test.That(t, err, test.ShouldBeNil)
	}

	// List all module resources
	allModules := manager.ListModuleResources()
	test.That(t, len(allModules), test.ShouldEqual, 2)
	test.That(t, allModules["module1"], test.ShouldNotBeNil)
	test.That(t, allModules["module2"], test.ShouldNotBeNil)
}

func TestModuleStatusManagerRemoveModuleResource(t *testing.T) {
	logger := logging.NewTestLogger(t)
	resourceGraph := newMockResourceGraph()
	manager := NewModuleStatusManager(resourceGraph, logger)

	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	err := manager.CreateModuleResource(context.Background(), cfg)
	test.That(t, err, test.ShouldBeNil)

	// Verify module exists
	_, exists := manager.GetModuleResource("test-module")
	test.That(t, exists, test.ShouldBeTrue)

	// Remove module
	manager.RemoveModuleResource("test-module")

	// Verify module was removed
	_, exists = manager.GetModuleResource("test-module")
	test.That(t, exists, test.ShouldBeFalse)
}

func TestModuleStatusManagerNonExistentModule(t *testing.T) {
	logger := logging.NewTestLogger(t)
	resourceGraph := newMockResourceGraph()
	manager := NewModuleStatusManager(resourceGraph, logger)

	// Try to update status for non-existent module
	err := manager.UpdatePackageStatus("non-existent", PackageLifecycleStatus{
		State:       PackageStateReady,
		LastUpdated: time.Now(),
	})
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, "module resource non-existent not found")

	err = manager.UpdateModuleStatus("non-existent", ModuleLifecycleStatus{
		State:       ModuleStateRunning,
		LastUpdated: time.Now(),
	})
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, "module resource non-existent not found")
}
