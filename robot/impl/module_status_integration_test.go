package robotimpl

import (
	"context"
	"testing"
	"time"

	"go.viam.com/test"

	"go.viam.com/rdk/config"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/module/modmanager"
	"go.viam.com/rdk/resource"
	robotstatus "go.viam.com/rdk/robot/status"
)

func TestModuleStatusInGetMachineStatus(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)

	// Create a resource graph
	resourceGraph := resource.NewGraph(logger)

	// Create a module status manager
	statusManager := modmanager.NewModuleStatusManager(resourceGraph, logger)

	// Create a test module configuration
	moduleConfig := config.Module{
		Name:     "test-module",
		Type:     config.ModuleTypeRegistry,
		ModuleID: "test-org:test-module",
	}

	// Create a module resource
	err := statusManager.CreateModuleResource(ctx, moduleConfig)
	test.That(t, err, test.ShouldBeNil)

	// Update the module status to simulate lifecycle changes
	err = statusManager.UpdatePackageStatus("test-module", robotstatus.PackageLifecycleStatus{
		State:       robotstatus.PackageStateDownloading,
		LastUpdated: time.Now(),
	})
	test.That(t, err, test.ShouldBeNil)

	err = statusManager.UpdateModuleStatus("test-module", robotstatus.ModuleLifecycleStatus{
		State:       robotstatus.ModuleStateStarting,
		LastUpdated: time.Now(),
	})
	test.That(t, err, test.ShouldBeNil)

	// Get all resource statuses from the graph (this is what MachineStatus does)
	statuses := resourceGraph.Status()

	// Verify that our module resource appears in the status list
	var moduleStatus *resource.NodeStatus
	for _, status := range statuses {
		if status.Name.API == modmanager.ModuleAPI && status.Name.Name == "test-module" {
			moduleStatus = &status
			break
		}
	}

	test.That(t, moduleStatus, test.ShouldNotBeNil)
	test.That(t, moduleStatus.Name.Name, test.ShouldEqual, "test-module")
	test.That(t, moduleStatus.State, test.ShouldEqual, resource.NodeStateConfiguring) // Should be configuring due to starting state

	// Update to running state
	err = statusManager.UpdatePackageStatus("test-module", robotstatus.PackageLifecycleStatus{
		State:       robotstatus.PackageStateReady,
		LastUpdated: time.Now(),
	})
	test.That(t, err, test.ShouldBeNil)

	err = statusManager.UpdateModuleStatus("test-module", robotstatus.ModuleLifecycleStatus{
		State:       robotstatus.ModuleStateRunning,
		LastUpdated: time.Now(),
	})
	test.That(t, err, test.ShouldBeNil)

	// Get updated statuses
	statuses = resourceGraph.Status()
	for _, status := range statuses {
		if status.Name.API == modmanager.ModuleAPI && status.Name.Name == "test-module" {
			moduleStatus = &status
			break
		}
	}

	test.That(t, moduleStatus, test.ShouldNotBeNil)
	test.That(t, moduleStatus.State, test.ShouldEqual, resource.NodeStateReady) // Should be ready due to running state
}

func TestModuleStatusWithFailures(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)

	// Create a resource graph
	resourceGraph := resource.NewGraph(logger)

	// Create a module status manager
	statusManager := modmanager.NewModuleStatusManager(resourceGraph, logger)

	// Create a test module configuration
	moduleConfig := config.Module{
		Name:     "failing-module",
		Type:     config.ModuleTypeRegistry,
		ModuleID: "test-org:failing-module",
	}

	// Create a module resource
	err := statusManager.CreateModuleResource(ctx, moduleConfig)
	test.That(t, err, test.ShouldBeNil)

	// Simulate a package download failure
	err = statusManager.UpdatePackageStatus("failing-module", robotstatus.PackageLifecycleStatus{
		State:       robotstatus.PackageStateFailed,
		LastUpdated: time.Now(),
		Error:       test.ErrFail,
	})
	test.That(t, err, test.ShouldBeNil)

	// Get resource statuses
	statuses := resourceGraph.Status()

	// Find our module resource
	var moduleStatus *resource.NodeStatus
	for _, status := range statuses {
		if status.Name.API == modmanager.ModuleAPI && status.Name.Name == "failing-module" {
			moduleStatus = &status
			break
		}
	}

	test.That(t, moduleStatus, test.ShouldNotBeNil)
	test.That(t, moduleStatus.Name.Name, test.ShouldEqual, "failing-module")
	test.That(t, moduleStatus.State, test.ShouldEqual, resource.NodeStateUnhealthy) // Should be unhealthy due to package failure
	test.That(t, moduleStatus.Error, test.ShouldEqual, test.ErrFail)
}

func TestModuleResourceRemoval(t *testing.T) {
	ctx := context.Background()
	logger := logging.NewTestLogger(t)

	// Create a resource graph
	resourceGraph := resource.NewGraph(logger)

	// Create a module status manager
	statusManager := modmanager.NewModuleStatusManager(resourceGraph, logger)

	// Create a test module configuration
	moduleConfig := config.Module{
		Name: "removable-module",
		Type: config.ModuleTypeLocal,
	}

	// Create a module resource
	err := statusManager.CreateModuleResource(ctx, moduleConfig)
	test.That(t, err, test.ShouldBeNil)

	// Verify the resource exists in the graph
	statuses := resourceGraph.Status()
	var found bool
	for _, status := range statuses {
		if status.Name.API == modmanager.ModuleAPI && status.Name.Name == "removable-module" {
			found = true
			break
		}
	}
	test.That(t, found, test.ShouldBeTrue)

	// Remove the module resource
	statusManager.RemoveModuleResource("removable-module")

	// Verify it's marked for removal
	moduleName := resource.NewName(modmanager.ModuleAPI, "removable-module")
	if node, exists := resourceGraph.Node(moduleName); exists {
		test.That(t, node.MarkedForRemoval(), test.ShouldBeTrue)
	}

	// Verify it's removed from tracking
	_, exists := statusManager.GetModuleResource("removable-module")
	test.That(t, exists, test.ShouldBeFalse)
}
