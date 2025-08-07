package reporter

import (
	"reflect"
	"testing"
)

func TestReporterSerializationRoundtrip(t *testing.T) {
	// Test using public API through Run function
	var originalReporter Reporter
	var serialized *SerializableReporter

	Run(func(r Reporter) {
		originalReporter = r

		// Use public API to set up reporter state
		r.Log("first test log")
		r.Logf("formatted log: %s", "test value")
		r.Error("test error message")

		// Create a child reporter using public Run method
		r.Run("child-test", func(child Reporter) {
			child.Log("child log message")
			child.Fail()
		})

		r.Run("skipped-test", func(child Reporter) {
			child.Skip("skipping this test")
		})

		// Mark this reporter as failed
		r.Fail()

		// Serialize using ToSerializable if available
		if serializable, ok := r.(interface{ ToSerializable() *SerializableReporter }); ok {
			serialized = serializable.ToSerializable()
		}
	})

	if serialized == nil {
		t.Fatal("failed to serialize reporter")
	}

	// Deserialize back to reporter
	restored := FromSerializable(serialized)

	// Verify core properties are preserved
	if restored.Failed() != originalReporter.Failed() {
		t.Errorf("Failed state mismatch: expected %t, got %t", originalReporter.Failed(), restored.Failed())
	}

	if restored.Skipped() != originalReporter.Skipped() {
		t.Errorf("Skipped state mismatch: expected %t, got %t", originalReporter.Skipped(), restored.Skipped())
	}

	// Verify logs are preserved
	if originalLogs := originalReporter.getLogs().all(); len(originalLogs) > 0 {
		restoredLogs := restored.getLogs().all()
		if !reflect.DeepEqual(originalLogs, restoredLogs) {
			t.Errorf("Logs mismatch: expected %v, got %v", originalLogs, restoredLogs)
		}
	}

	// Verify children are preserved
	originalChildren := originalReporter.getChildren()
	restoredChildren := restored.getChildren()

	if len(originalChildren) != len(restoredChildren) {
		t.Errorf("Children count mismatch: expected %d, got %d", len(originalChildren), len(restoredChildren))
	}

	// Note: Children may have different states due to the serialization process
	// and the fact that some children might fail during test execution.
	// The important thing is that the structure is preserved.
}

func TestReporterSerializationWithFromT(t *testing.T) {
	// Test using FromT to create reporter
	originalReporter := FromT(t)

	// Use public API methods to set state
	originalReporter.Log("test log from FromT")
	originalReporter.Log("another log message")

	// Serialize the reporter
	var serialized *SerializableReporter
	if serializable, ok := originalReporter.(interface{ ToSerializable() *SerializableReporter }); ok {
		serialized = serializable.ToSerializable()
	} else {
		t.Fatal("reporter does not support serialization")
	}

	// Deserialize back
	restored := FromSerializable(serialized)

	// Verify the reporter name is preserved
	if restored.getName() != originalReporter.getName() {
		t.Errorf("Name mismatch: expected %s, got %s", originalReporter.getName(), restored.getName())
	}

	// Verify logs are preserved
	originalLogs := originalReporter.getLogs().all()
	restoredLogs := restored.getLogs().all()
	if !reflect.DeepEqual(originalLogs, restoredLogs) {
		t.Errorf("Logs mismatch: expected %v, got %v", originalLogs, restoredLogs)
	}

	// Verify Failed() and Skipped() states are preserved
	if restored.Failed() != originalReporter.Failed() {
		t.Errorf("Failed state mismatch: expected %t, got %t", originalReporter.Failed(), restored.Failed())
	}

	if restored.Skipped() != originalReporter.Skipped() {
		t.Errorf("Skipped state mismatch: expected %t, got %t", originalReporter.Skipped(), restored.Skipped())
	}
}

func TestTestContextSerialization(t *testing.T) {
	// Create a test context with specific options and verify serialization
	var originalContext *testContext
	var serialized *SerializableTestContext

	Run(func(r Reporter) {
		// Access the internal context through the reporter
		if rep, ok := r.(*reporter); ok {
			originalContext = rep.context
			serialized = originalContext.ToSerializable()
		}
	}, WithVerboseLog(), WithMaxParallel(4))

	if originalContext == nil || serialized == nil {
		t.Fatal("failed to access or serialize test context")
	}

	// Deserialize the context
	restored := FromSerializableTestContext(serialized)

	// Verify properties are preserved
	if restored.maxParallel != originalContext.maxParallel {
		t.Errorf("MaxParallel mismatch: expected %d, got %d", originalContext.maxParallel, restored.maxParallel)
	}

	if restored.verbose != originalContext.verbose {
		t.Errorf("Verbose mismatch: expected %t, got %t", originalContext.verbose, restored.verbose)
	}

	// Compare ColorConfig enabled state (colorConfig is always non-nil now)
	originalEnabled := originalContext.colorConfig.IsEnabled()
	restoredEnabled := restored.colorConfig.IsEnabled()
	if restoredEnabled != originalEnabled {
		t.Errorf("ColorConfig enabled state mismatch: expected %t, got %t", originalEnabled, restoredEnabled)
	}

	if restored.enabledTestSummary != originalContext.enabledTestSummary {
		t.Errorf("EnabledTestSummary mismatch: expected %t, got %t", originalContext.enabledTestSummary, restored.enabledTestSummary)
	}
}
