package packages

import (
	"testing"
	"time"

	"go.viam.com/test"

	"go.viam.com/rdk/module/modmanager"
)

func TestNoOpStatusReporter(t *testing.T) {
	reporter := NewNoOpStatusReporter()
	test.That(t, reporter, test.ShouldNotBeNil)

	// Test that it doesn't error when reporting status
	err := reporter.ReportPackageStatus("test-module", modmanager.PackageLifecycleStatus{
		State:       modmanager.PackageStateDownloading,
		LastUpdated: time.Now(),
	})
	test.That(t, err, test.ShouldBeNil)

	err = reporter.ReportPackageStatus("test-module", modmanager.PackageLifecycleStatus{
		State:       modmanager.PackageStateReady,
		LastUpdated: time.Now(),
	})
	test.That(t, err, test.ShouldBeNil)
}

// mockStatusReporter is a test implementation of StatusReporter.
type mockStatusReporter struct {
	reportedStatuses []mockReportedStatus
}

type mockReportedStatus struct {
	ModuleName string
	Status     modmanager.PackageLifecycleStatus
}

func (m *mockStatusReporter) ReportPackageStatus(moduleName string, status modmanager.PackageLifecycleStatus) error {
	m.reportedStatuses = append(m.reportedStatuses, mockReportedStatus{
		ModuleName: moduleName,
		Status:     status,
	})
	return nil
}

func TestMockStatusReporter(t *testing.T) {
	reporter := &mockStatusReporter{}

	// Report some statuses
	status1 := modmanager.PackageLifecycleStatus{
		State:       modmanager.PackageStateDownloading,
		LastUpdated: time.Now(),
	}
	err := reporter.ReportPackageStatus("module1", status1)
	test.That(t, err, test.ShouldBeNil)

	status2 := modmanager.PackageLifecycleStatus{
		State:       modmanager.PackageStateReady,
		LastUpdated: time.Now(),
	}
	err = reporter.ReportPackageStatus("module2", status2)
	test.That(t, err, test.ShouldBeNil)

	// Verify statuses were recorded
	test.That(t, len(reporter.reportedStatuses), test.ShouldEqual, 2)
	test.That(t, reporter.reportedStatuses[0].ModuleName, test.ShouldEqual, "module1")
	test.That(t, reporter.reportedStatuses[0].Status.State, test.ShouldEqual, modmanager.PackageStateDownloading)
	test.That(t, reporter.reportedStatuses[1].ModuleName, test.ShouldEqual, "module2")
	test.That(t, reporter.reportedStatuses[1].Status.State, test.ShouldEqual, modmanager.PackageStateReady)
}
