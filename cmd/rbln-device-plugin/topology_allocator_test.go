package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	rblndevice "github.com/rbln-sw/rblnlib-go/pkg/device"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func TestSelectPreferredDeviceIDsPrefersSingleSID(t *testing.T) {
	plugin := testPluginWithDevices(map[string]NPUDevice{
		"rbln0": testDevice("rbln0", "sid-a", "0000:06:00.0", "0"),
		"rbln1": testDevice("rbln1", "sid-a", "0000:06:00.1", "0"),
		"rbln2": testDevice("rbln2", "sid-a", "0000:06:00.2", "0"),
		"rbln3": testDevice("rbln3", "sid-a", "0000:06:00.3", "0"),
		"rbln4": testDevice("rbln4", "sid-b", "0000:07:00.0", "0"),
		"rbln5": testDevice("rbln5", "sid-b", "0000:07:00.1", "0"),
		"rbln6": testDevice("rbln6", "sid-b", "0000:07:00.2", "0"),
		"rbln7": testDevice("rbln7", "sid-b", "0000:07:00.3", "0"),
	})

	selected, err := plugin.selectPreferredDeviceIDs(
		[]string{"rbln0", "rbln1", "rbln2", "rbln3", "rbln4", "rbln5", "rbln6", "rbln7"},
		nil,
		4,
	)
	if err != nil {
		t.Fatalf("select preferred devices: %v", err)
	}

	if len(selected) != 4 {
		t.Fatalf("expected 4 devices, got %d", len(selected))
	}
	assertSameSID(t, plugin.devices, selected, "sid-a")
}

func TestSelectPreferredDeviceIDsWorksWithoutTopology(t *testing.T) {
	plugin := testPluginWithDevices(map[string]NPUDevice{
		"rbln0": testDevice("rbln0", "sid-a", "0000:06:00.0", ""),
		"rbln1": testDevice("rbln1", "sid-a", "0000:06:00.1", ""),
		"rbln2": testDevice("rbln2", "sid-a", "0000:06:00.2", ""),
		"rbln3": testDevice("rbln3", "sid-a", "0000:06:00.3", ""),
		"rbln4": testDevice("rbln4", "sid-b", "0000:07:00.0", ""),
		"rbln5": testDevice("rbln5", "sid-b", "0000:07:00.1", ""),
		"rbln6": testDevice("rbln6", "sid-b", "0000:07:00.2", ""),
		"rbln7": testDevice("rbln7", "sid-b", "0000:07:00.3", ""),
	})

	selected, err := plugin.selectPreferredDeviceIDs(
		[]string{"rbln0", "rbln1", "rbln2", "rbln3", "rbln4", "rbln5", "rbln6", "rbln7"},
		nil,
		4,
	)
	if err != nil {
		t.Fatalf("select preferred devices: %v", err)
	}

	assertSameSID(t, plugin.devices, selected, "sid-a")
}

func TestSelectPreferredDeviceIDsFillsExistingSIDBeforeOpeningAnother(t *testing.T) {
	plugin := testPluginWithDevices(map[string]NPUDevice{
		"rbln0":  testDevice("rbln0", "sid-a", "0000:06:00.0", ""),
		"rbln1":  testDevice("rbln1", "sid-a", "0000:06:00.1", ""),
		"rbln2":  testDevice("rbln2", "sid-a", "0000:06:00.2", ""),
		"rbln3":  testDevice("rbln3", "sid-a", "0000:06:00.3", ""),
		"rbln4":  testDevice("rbln4", "sid-b", "0000:07:00.0", ""),
		"rbln5":  testDevice("rbln5", "sid-b", "0000:07:00.1", ""),
		"rbln6":  testDevice("rbln6", "sid-b", "0000:07:00.2", ""),
		"rbln7":  testDevice("rbln7", "sid-b", "0000:07:00.3", ""),
		"rbln8":  testDevice("rbln8", "sid-c", "0000:08:00.0", ""),
		"rbln9":  testDevice("rbln9", "sid-c", "0000:08:00.1", ""),
		"rbln10": testDevice("rbln10", "sid-c", "0000:08:00.2", ""),
		"rbln11": testDevice("rbln11", "sid-c", "0000:08:00.3", ""),
	})

	selected, err := plugin.selectPreferredDeviceIDs(
		[]string{"rbln0", "rbln1", "rbln2", "rbln3", "rbln4", "rbln5", "rbln6", "rbln7", "rbln8", "rbln9", "rbln10", "rbln11"},
		[]string{"rbln1"},
		8,
	)
	if err != nil {
		t.Fatalf("select preferred devices: %v", err)
	}

	if !contains(selected, "rbln1") {
		t.Fatalf("selected devices %v do not include must-include device", selected)
	}
	if got := countSID(plugin.devices, selected, "sid-a"); got != 4 {
		t.Fatalf("expected to fill sid-a first, got %d devices from sid-a in %v", got, selected)
	}
	if got := uniqueSIDCount(plugin.devices, selected); got != 2 {
		t.Fatalf("expected 2 SID groups, got %d in %v", got, selected)
	}
}

func TestSelectPreferredDeviceIDsUsesNUMAToBreakSIDTie(t *testing.T) {
	plugin := testPluginWithDevices(map[string]NPUDevice{
		"rbln0":  testDevice("rbln0", "sid-a", "0000:06:00.0", "0"),
		"rbln1":  testDevice("rbln1", "sid-a", "0000:06:00.1", "0"),
		"rbln2":  testDevice("rbln2", "sid-a", "0000:06:00.2", "0"),
		"rbln3":  testDevice("rbln3", "sid-a", "0000:06:00.3", "0"),
		"rbln4":  testDevice("rbln4", "sid-b", "0000:07:00.0", "1"),
		"rbln5":  testDevice("rbln5", "sid-b", "0000:07:00.1", "1"),
		"rbln6":  testDevice("rbln6", "sid-b", "0000:07:00.2", "1"),
		"rbln7":  testDevice("rbln7", "sid-b", "0000:07:00.3", "1"),
		"rbln8":  testDevice("rbln8", "sid-c", "0000:08:00.0", "0"),
		"rbln9":  testDevice("rbln9", "sid-c", "0000:08:00.1", "0"),
		"rbln10": testDevice("rbln10", "sid-c", "0000:08:00.2", "0"),
		"rbln11": testDevice("rbln11", "sid-c", "0000:08:00.3", "0"),
		"rbln12": testDevice("rbln12", "sid-d", "0000:09:00.0", "1"),
		"rbln13": testDevice("rbln13", "sid-d", "0000:09:00.1", "1"),
		"rbln14": testDevice("rbln14", "sid-d", "0000:09:00.2", "1"),
		"rbln15": testDevice("rbln15", "sid-d", "0000:09:00.3", "1"),
	})

	selected, err := plugin.selectPreferredDeviceIDs(
		[]string{"rbln0", "rbln1", "rbln2", "rbln3", "rbln4", "rbln5", "rbln6", "rbln7", "rbln8", "rbln9", "rbln10", "rbln11", "rbln12", "rbln13", "rbln14", "rbln15"},
		nil,
		8,
	)
	if err != nil {
		t.Fatalf("select preferred devices: %v", err)
	}

	if got := uniqueSIDCount(plugin.devices, selected); got != 2 {
		t.Fatalf("expected 2 SID groups, got %d in %v", got, selected)
	}
	if got := uniqueNUMACount(plugin.devices, selected); got != 1 {
		t.Fatalf("expected one NUMA node after SID tie-break, got %d in %v", got, selected)
	}
	if got := countSID(plugin.devices, selected, "sid-a"); got != 4 {
		t.Fatalf("expected sid-a to be selected, got %v", selected)
	}
	if got := countSID(plugin.devices, selected, "sid-c"); got != 4 {
		t.Fatalf("expected sid-c to be selected, got %v", selected)
	}
}

func TestGetPreferredAllocationAndOptions(t *testing.T) {
	plugin := testPluginWithDevices(map[string]NPUDevice{
		"rbln0": testDevice("rbln0", "sid-a", "0000:06:00.0", ""),
		"rbln1": testDevice("rbln1", "sid-a", "0000:06:00.1", ""),
		"rbln2": testDevice("rbln2", "sid-a", "0000:06:00.2", ""),
		"rbln3": testDevice("rbln3", "sid-a", "0000:06:00.3", ""),
	})

	options, err := plugin.GetDevicePluginOptions(context.Background(), &pluginapi.Empty{})
	if err != nil {
		t.Fatalf("get device plugin options: %v", err)
	}
	if !options.GetPreferredAllocationAvailable {
		t.Fatalf("preferred allocation must be enabled")
	}

	response, err := plugin.GetPreferredAllocation(context.Background(), &pluginapi.PreferredAllocationRequest{
		ContainerRequests: []*pluginapi.ContainerPreferredAllocationRequest{
			{
				AvailableDeviceIDs: []string{"rbln0", "rbln1", "rbln2", "rbln3"},
				AllocationSize:     4,
			},
		},
	})
	if err != nil {
		t.Fatalf("get preferred allocation: %v", err)
	}
	if len(response.ContainerResponses) != 1 {
		t.Fatalf("expected 1 container response, got %d", len(response.ContainerResponses))
	}
	if len(response.ContainerResponses[0].DeviceIDs) != 4 {
		t.Fatalf("expected 4 device IDs, got %v", response.ContainerResponses[0].DeviceIDs)
	}
}

func testPluginWithDevices(devices map[string]NPUDevice) *ResourcePlugin {
	return NewResourcePlugin("rebellions.ai/ATOM", filepath.Join(os.TempDir(), "rbln-test.sock"), filepath.Join(os.TempDir(), "kubelet.sock"), nil, devices)
}

func testDevice(name, sid, pciBusID, numaNode string) NPUDevice {
	return NPUDevice{
		Info: rblndevice.Device{
			Name:        name,
			ProductName: "RBLN-CA25",
			SID:         sid,
			PCIDeviceID: "1251",
			PCIBusID:    pciBusID,
			PCINumaNode: numaNode,
		},
	}
}

func assertSameSID(t *testing.T, devices map[string]NPUDevice, selected []string, expectedSID string) {
	t.Helper()

	if len(selected) == 0 {
		t.Fatalf("expected selected devices, got none")
	}
	for _, deviceID := range selected {
		if devices[deviceID].Info.SID != expectedSID {
			t.Fatalf("device %q has SID %q, expected %q; selected=%v", deviceID, devices[deviceID].Info.SID, expectedSID, selected)
		}
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func countSID(devices map[string]NPUDevice, selected []string, sid string) int {
	count := 0
	for _, deviceID := range selected {
		if devices[deviceID].Info.SID == sid {
			count++
		}
	}
	return count
}

func uniqueSIDCount(devices map[string]NPUDevice, selected []string) int {
	seen := make(map[string]struct{})
	for _, deviceID := range selected {
		seen[devices[deviceID].Info.SID] = struct{}{}
	}
	return len(seen)
}

func uniqueNUMACount(devices map[string]NPUDevice, selected []string) int {
	seen := make(map[string]struct{})
	for _, deviceID := range selected {
		seen[devices[deviceID].Info.PCINumaNode] = struct{}{}
	}
	return len(seen)
}
