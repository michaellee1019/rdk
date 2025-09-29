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

func TestNewModuleResource(t *testing.T) {
	logger := logging.NewTestLogger(t)
	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	moduleResource := NewModuleResource(cfg, logger)

	test.That(t, moduleResource.Name().Name, test.ShouldEqual, "test-module")
	test.That(t, moduleResource.Name().API, test.ShouldEqual, ModuleAPI)
	test.That(t, moduleResource.cfg.Name, test.ShouldEqual, "test-module")

	// Check initial status
	packageStatus := moduleResource.GetPackageStatus()
	test.That(t, packageStatus.State, test.ShouldEqual, PackageStatePending) // Registry module needs package

	moduleStatus := moduleResource.GetModuleStatus()
	test.That(t, moduleStatus.State, test.ShouldEqual, ModuleStatePending)
}

func TestNewModuleResourceLocalNonTarball(t *testing.T) {
	logger := logging.NewTestLogger(t)
	cfg := config.Module{
		Name:    "test-local-module",
		Type:    config.ModuleTypeLocal,
		ExePath: "/path/to/binary",
	}

	moduleResource := NewModuleResource(cfg, logger)

	// Local non-tarball modules don't need packages
	packageStatus := moduleResource.GetPackageStatus()
	test.That(t, packageStatus.State, test.ShouldEqual, PackageStateNotNeeded)
}

func TestModuleResourceStatusUpdates(t *testing.T) {
	logger := logging.NewTestLogger(t)
	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	moduleResource := NewModuleResource(cfg, logger)

	// Test package status update
	newPackageStatus := PackageLifecycleStatus{
		State:       PackageStateDownloading,
		LastUpdated: time.Now(),
	}
	moduleResource.UpdatePackageStatus(newPackageStatus)

	updatedStatus := moduleResource.GetPackageStatus()
	test.That(t, updatedStatus.State, test.ShouldEqual, PackageStateDownloading)

	// Test module status update
	newModuleStatus := ModuleLifecycleStatus{
		State:       ModuleStateStarting,
		LastUpdated: time.Now(),
	}
	moduleResource.UpdateModuleStatus(newModuleStatus)

	updatedModuleStatus := moduleResource.GetModuleStatus()
	test.That(t, updatedModuleStatus.State, test.ShouldEqual, ModuleStateStarting)
}

func TestModuleResourceNodeStatus(t *testing.T) {
	logger := logging.NewTestLogger(t)
	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	moduleResource := NewModuleResource(cfg, logger)

	// Test package failed status takes priority
	moduleResource.UpdatePackageStatus(PackageLifecycleStatus{
		State:       PackageStateFailed,
		LastUpdated: time.Now(),
		Error:       test.ErrFail,
	})

	status := moduleResource.nodeStatus()
	test.That(t, status.State, test.ShouldEqual, resource.NodeStateUnhealthy)
	test.That(t, status.Error, test.ShouldEqual, test.ErrFail)

	// Test package downloading status
	moduleResource.UpdatePackageStatus(PackageLifecycleStatus{
		State:       PackageStateDownloading,
		LastUpdated: time.Now(),
	})

	status = moduleResource.nodeStatus()
	test.That(t, status.State, test.ShouldEqual, resource.NodeStateConfiguring)

	// Test module running status
	moduleResource.UpdatePackageStatus(PackageLifecycleStatus{
		State:       PackageStateReady,
		LastUpdated: time.Now(),
	})
	moduleResource.UpdateModuleStatus(ModuleLifecycleStatus{
		State:       ModuleStateRunning,
		LastUpdated: time.Now(),
	})

	status = moduleResource.nodeStatus()
	test.That(t, status.State, test.ShouldEqual, resource.NodeStateReady)
}

func TestModuleResourceDetailedStatus(t *testing.T) {
	logger := logging.NewTestLogger(t)
	cfg := config.Module{
		Name:     "test-module",
		Type:     config.ModuleTypeRegistry,
		ModuleID: "test-org:test-module",
	}

	moduleResource := NewModuleResource(cfg, logger)

	detailedStatus := moduleResource.DetailedStatus()
	test.That(t, detailedStatus.ModuleName, test.ShouldEqual, "test-module")
	test.That(t, detailedStatus.ModuleType, test.ShouldEqual, config.ModuleTypeRegistry)
	test.That(t, detailedStatus.ModuleID, test.ShouldEqual, "test-org:test-module")
	test.That(t, detailedStatus.PackageStatus.State, test.ShouldEqual, PackageStatePending)
	test.That(t, detailedStatus.ModuleStatus.State, test.ShouldEqual, ModuleStatePending)
}

func TestModuleResourceDoCommand(t *testing.T) {
	logger := logging.NewTestLogger(t)
	cfg := config.Module{
		Name: "test-module",
		Type: config.ModuleTypeRegistry,
	}

	moduleResource := NewModuleResource(cfg, logger)

	// Test get detailed status command
	cmd := map[string]interface{}{
		"get_detailed_status": true,
	}

	result, err := moduleResource.DoCommand(context.Background(), cmd)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, result["detailed_status"], test.ShouldNotBeNil)

	// Test unimplemented command
	cmd = map[string]interface{}{
		"unknown_command": true,
	}

	result, err = moduleResource.DoCommand(context.Background(), cmd)
	test.That(t, err, test.ShouldEqual, resource.ErrDoUnimplemented)
	test.That(t, result, test.ShouldBeNil)
}
