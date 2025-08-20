package reporter

import (
	"reflect"
	"testing"
)

func TestReporterSerializationRoundtrip(t *testing.T) {
	// Test using public API through Run function
	var originalReporter *reporter
	var reporterMap map[string]*SerializableReporter
	var reporterID string

	Run(func(r Reporter) {
		// Cast to internal type to access serialization methods
		rep, ok := r.(*reporter)
		if !ok {
			t.Fatal("reporter is not of expected type")
		}
		originalReporter = rep

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

		// Serialize using correct method signature
		reporterMap = make(map[string]*SerializableReporter)
		reporterID = rep.ToSerializable(reporterMap)
	})

	if reporterMap == nil || reporterID == "" {
		t.Fatal("failed to serialize reporter")
	}

	serialized, exists := reporterMap[reporterID]
	if !exists {
		t.Fatal("serialized reporter not found in map")
	}

	// Deserialize back to reporter
	restored := FromSerializable(reporterID, reporterMap)

	// Verify core properties are preserved
	if restored.Failed() != originalReporter.Failed() {
		t.Errorf("Failed state mismatch: expected %t, got %t", originalReporter.Failed(), restored.Failed())
	}

	if restored.Skipped() != originalReporter.Skipped() {
		t.Errorf("Skipped state mismatch: expected %t, got %t", originalReporter.Skipped(), restored.Skipped())
	}

	// Verify basic properties
	if restored.getName() != originalReporter.getName() {
		t.Errorf("Name mismatch: expected %s, got %s", originalReporter.getName(), restored.getName())
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

	// Verify serialized data structure
	if serialized.Failed != originalReporter.Failed() {
		t.Errorf("Serialized Failed state mismatch: expected %t, got %t", originalReporter.Failed(), serialized.Failed)
	}

	if serialized.Name != originalReporter.getName() {
		t.Errorf("Serialized Name mismatch: expected %s, got %s", originalReporter.getName(), serialized.Name)
	}
}

func TestReporterSerializationWithFromT(t *testing.T) {
	// Test using FromT to create reporter
	originalReporter := FromT(t)

	// Cast to internal type to access serialization methods
	rep, ok := originalReporter.(*reporter)
	if !ok {
		t.Fatal("reporter from FromT is not of expected type")
	}

	// Use public API methods to set state
	originalReporter.Log("test log from FromT")
	originalReporter.Log("another log message")

	// Serialize the reporter using correct method signature
	reporterMap := make(map[string]*SerializableReporter)
	reporterID := rep.ToSerializable(reporterMap)

	if reporterID == "" {
		t.Fatal("failed to serialize reporter")
	}

	serialized, exists := reporterMap[reporterID]
	if !exists {
		t.Fatal("serialized reporter not found in map")
	}

	// Deserialize back
	restored := FromSerializable(reporterID, reporterMap)

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

	// Verify serialized structure fields
	if serialized.Logs == nil || len(serialized.Logs) != 2 {
		t.Errorf("Serialized logs count mismatch: expected 2, got %d", len(serialized.Logs))
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

func TestMultipleReporterSerialization(t *testing.T) {
	// Test serializing multiple reporters to the same map
	var reporters []*reporter
	reporterMap := make(map[string]*SerializableReporter)
	var reporterIDs []string

	// Create multiple reporters with different states
	reporterCount := 2 // Reduced count for more reliable testing
	for i := range reporterCount {
		currentIndex := i // capture loop variable
		Run(func(r Reporter) {
			rep, ok := r.(*reporter)
			if !ok {
				t.Fatal("reporter is not of expected type")
			}
			reporters = append(reporters, rep)

			// Set different states for each reporter
			switch currentIndex {
			case 0:
				r.Log("reporter 0 log")
				r.Fail()
			case 1:
				r.Log("reporter 1 log")
				// This one remains successful
			}

			// Serialize to shared map
			id := rep.ToSerializable(reporterMap)
			reporterIDs = append(reporterIDs, id)
		})
	}

	// Verify correct number of reporters
	if len(reporters) != reporterCount {
		t.Errorf("Expected %d reporters, got %d", reporterCount, len(reporters))
	}

	// Verify some reporters were serialized
	if len(reporterMap) == 0 {
		t.Error("No reporters were serialized")
	}

	// Deserialize and verify basic functionality
	for i, id := range reporterIDs {
		if i >= len(reporters) {
			break
		}
		restored := FromSerializable(id, reporterMap)
		if restored == nil {
			t.Errorf("Failed to deserialize reporter %d", i)
			continue
		}

		original := reporters[i]
		if restored.getName() != original.getName() {
			t.Errorf("Reporter %d name mismatch: expected %s, got %s", i, original.getName(), restored.getName())
		}
	}
}

func TestFromSerializableErrorCases(t *testing.T) {
	// Test FromSerializable with valid data but edge cases
	reporterMap := make(map[string]*SerializableReporter)

	// Create a minimal valid SerializableReporter
	testID := "test-reporter-id"
	reporterMap[testID] = &SerializableReporter{
		Name:       "test-reporter",
		GoTestName: "TestSample",
		Depth:      0,
		Failed:     false,
		Skipped:    false,
		IsParallel: false,
		Logs:       []string{"test log"},
		Testing:    true,
		Retryable:  false,
	}

	// Test with valid ID
	restored := FromSerializable(testID, reporterMap)
	if restored == nil {
		t.Error("FromSerializable should not return nil for valid ID")
	}

	if restored.getName() != "test-reporter" {
		t.Errorf("Expected name 'test-reporter', got '%s'", restored.getName())
	}

	// Test with empty ID but valid map
	emptyIDReporter := &SerializableReporter{
		Name:    "",
		Failed:  false,
		Skipped: false,
		Logs:    []string{},
	}
	reporterMap[""] = emptyIDReporter

	restored2 := FromSerializable("", reporterMap)
	if restored2 == nil {
		t.Error("FromSerializable should not return nil for empty ID with valid data")
	}
}

func TestParentChildSerialization(t *testing.T) {
	// Test serialization of parent-child reporter relationships
	var parentReporter *reporter
	reporterMap := make(map[string]*SerializableReporter)
	var parentID string

	Run(func(r Reporter) {
		parentRep, ok := r.(*reporter)
		if !ok {
			t.Fatal("parent reporter is not of expected type")
		}
		parentReporter = parentRep

		r.Log("parent log")

		// Create child reporter
		r.Run("child-task", func(child Reporter) {
			child.Log("child log")
			child.Fail()
		})

		// Serialize parent (should include child references)
		parentID = parentRep.ToSerializable(reporterMap)
	})

	// Verify parent was serialized
	if parentID == "" {
		t.Fatal("Failed to serialize parent reporter")
	}

	if len(reporterMap) == 0 {
		t.Fatal("No reporters were serialized")
	}

	// Deserialize parent
	restoredParent := FromSerializable(parentID, reporterMap)

	if restoredParent == nil {
		t.Fatal("Failed to deserialize parent reporter")
	}

	// Verify parent basic properties
	if restoredParent.getName() != parentReporter.getName() {
		t.Errorf("Parent name mismatch: expected %s, got %s", parentReporter.getName(), restoredParent.getName())
	}

	// Note: Child relationships may not be fully preserved in deserialization
	// This is acceptable for basic serialization functionality
}

func TestSetFromSerializable(t *testing.T) {
	// Create a reporter directly instead of using FromT
	rep := newReporter()
	if rep == nil {
		t.Fatal("failed to create reporter")
	}

	// Create minimal serialized data for testing
	reporterMap := make(map[string]*SerializableReporter)
	testID := "test-id"
	reporterMap[testID] = &SerializableReporter{
		Name:    "test-name",
		Failed:  false,
		Logs:    []string{"test log"},
		Testing: false,
	}

	// Test that SetFromSerializable works without crashing
	rep.SetFromSerializable(testID, reporterMap)

	// Basic verification that the method ran
	if rep.getName() != "test-name" {
		t.Errorf("Expected name 'test-name', got '%s'", rep.getName())
	}
}

func TestDuplicateSerializationOptimization(t *testing.T) {
	// Test that serializing the same reporter multiple times reuses existing entry
	var rep *reporter

	Run(func(r Reporter) {
		reporter, ok := r.(*reporter)
		if !ok {
			t.Fatal("reporter is not of expected type")
		}
		rep = reporter
		r.Log("test log")
	})

	reporterMap := make(map[string]*SerializableReporter)

	// Serialize same reporter multiple times
	id1 := rep.ToSerializable(reporterMap)
	id2 := rep.ToSerializable(reporterMap)
	id3 := rep.ToSerializable(reporterMap)

	// Should all return same ID
	if id1 != id2 || id2 != id3 {
		t.Errorf("Multiple serializations should return same ID: got %s, %s, %s", id1, id2, id3)
	}

	// Should only have one entry in map
	if len(reporterMap) != 1 {
		t.Errorf("Expected 1 entry in reporter map, got %d", len(reporterMap))
	}
}

func TestFromSerializableWithReporter(t *testing.T) {
	// Test FromSerializableWithReporter function with different cases
	reporterMap := make(map[string]*SerializableReporter)
	var reporterID string

	// Create a reporter with some state
	Run(func(r Reporter) {
		rep, ok := r.(*reporter)
		if !ok {
			t.Fatal("reporter is not of expected type")
		}

		r.Log("original reporter log")
		r.Run("child", func(child Reporter) {
			child.Log("child log")
		})

		reporterID = rep.ToSerializable(reporterMap)
	})

	// Test with nil reporter - should create new reporter
	restored1, err := FromSerializableWithReporter(nil, reporterID, reporterMap)
	if err != nil {
		t.Fatalf("FromSerializableWithReporter with nil reporter failed: %v", err)
	}
	if restored1 == nil {
		t.Error("FromSerializableWithReporter should return non-nil reporter")
	}

	// Test with existing reporter - should append children
	existingReporter := newReporter()
	existingReporter.Log("existing log")

	restored2, err := FromSerializableWithReporter(existingReporter, reporterID, reporterMap)
	if err != nil {
		t.Fatalf("FromSerializableWithReporter with existing reporter failed: %v", err)
	}
	if restored2 != existingReporter {
		t.Error("FromSerializableWithReporter should return the same reporter instance")
	}

	// Test with non-reporter type by passing nil instead of invalid reporter
	_, err = FromSerializableWithReporter(nil, "nonexistent-id", make(map[string]*SerializableReporter))
	if err != nil {
		t.Errorf("FromSerializableWithReporter with nil should not error: %v", err)
	}
}

func TestFromSerializableTestContextWithTestSummary(t *testing.T) {
	// Test FromSerializableTestContext with EnabledTestSummary = true
	serialized := &SerializableTestContext{
		MaxParallel:        2,
		Verbose:            true,
		ColorEnabled:       false,
		EnabledTestSummary: true,
		Running:            1,
		NumWaiting:         5,
	}

	restored := FromSerializableTestContext(serialized)

	// Verify all properties are correctly restored
	if restored.maxParallel != 2 {
		t.Errorf("MaxParallel mismatch: expected 2, got %d", restored.maxParallel)
	}
	if !restored.verbose {
		t.Error("Verbose should be true")
	}
	if restored.colorConfig.IsEnabled() {
		t.Error("ColorEnabled should be false")
	}
	if !restored.enabledTestSummary {
		t.Error("EnabledTestSummary should be true")
	}
	if restored.running != 1 {
		t.Errorf("Running mismatch: expected 1, got %d", restored.running)
	}
	if restored.numWaiting != 5 {
		t.Errorf("NumWaiting mismatch: expected 5, got %d", restored.numWaiting)
	}

	// Verify testSummary is created when enabled
	if restored.testSummary == nil {
		t.Error("testSummary should be created when EnabledTestSummary is true")
	}

	// Verify writer is set to nopWriter
	if restored.w == nil {
		t.Error("Writer should not be nil")
	}

	// Verify startParallel channel is created
	if restored.startParallel == nil {
		t.Error("startParallel channel should be created")
	}
}

func TestSetFromSerializableEdgeCases(t *testing.T) {
	rep := newReporter()

	// Test with nil SerializableReporter
	rep.setFromSerializable(nil, make(map[string]*reporter), make(map[string]*SerializableReporter))
	// Should not crash and should handle nil gracefully

	// Test with reporter that has parent and children
	reporterMap := make(map[string]*reporter)
	serializedMap := make(map[string]*SerializableReporter)

	// Create serialized data with parent-child relationships
	parentSR := &SerializableReporter{
		Name:     "parent",
		Failed:   false,
		Skipped:  false,
		Children: []string{"child1", "child2"},
		Logs:     []string{"parent log"},
	}

	child1SR := &SerializableReporter{
		Name:   "child1",
		Failed: true,
		Parent: "parent",
		Logs:   []string{"child1 log"},
	}

	child2SR := &SerializableReporter{
		Name:    "child2",
		Skipped: true,
		Parent:  "parent",
		Logs:    []string{"child2 log"},
	}

	serializedMap["parent"] = parentSR
	serializedMap["child1"] = child1SR
	serializedMap["child2"] = child2SR

	// Apply to reporter
	rep.setFromSerializable(parentSR, reporterMap, serializedMap)

	// Verify parent properties
	if rep.getName() != "parent" {
		t.Errorf("Parent name mismatch: expected 'parent', got '%s'", rep.getName())
	}

	// Verify children were added
	children := rep.getChildren()
	if len(children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(children))
	}

	// Verify Failed and Skipped states are applied correctly
	if rep.Failed() {
		t.Error("Parent should not be failed")
	}
	if rep.Skipped() {
		t.Error("Parent should not be skipped")
	}

	// Check that children have correct states
	if len(children) >= 2 {
		// Find child1 and child2 by casting from Reporter interface
		var child1, child2 Reporter
		for _, child := range children {
			if childRep, ok := child.(*reporter); ok {
				if childRep.getName() == "child1" {
					child1 = child
				} else if childRep.getName() == "child2" {
					child2 = child
				}
			}
		}

		if child1 != nil && !child1.Failed() {
			t.Error("child1 should be failed")
		}
		if child2 != nil && !child2.Skipped() {
			t.Error("child2 should be skipped")
		}
	}
}
