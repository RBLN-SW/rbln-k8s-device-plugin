package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/RBLN-SW/rbln-k8s-device-plugin/pkg/consts"
)

func TestCDIHandlerInitializeDoesNotRequireBaseSpec(t *testing.T) {
	t.Parallel()

	cdi, err := NewCDIHandler(t.TempDir())
	if err != nil {
		t.Fatalf("new CDI handler: %v", err)
	}
	if err := cdi.Initialize(); err != nil {
		t.Fatalf("initialize CDI handler: %v", err)
	}
}

func TestCDIHandlerRuntimeAnnotations(t *testing.T) {
	t.Parallel()

	cdi, err := NewCDIHandler(t.TempDir())
	if err != nil {
		t.Fatalf("new CDI handler: %v", err)
	}

	annotations, err := cdi.RuntimeAnnotations()
	if err != nil {
		t.Fatalf("runtime annotations: %v", err)
	}

	if got := annotations["cdi.k8s.io/rebellions.ai_npu"]; got != consts.CDIKind+"="+consts.BaseCDIDevice {
		t.Fatalf("unexpected runtime annotation value %q", got)
	}
}

func TestAllocateReturnsRuntimeAnnotationAndDeviceSpecs(t *testing.T) {
	t.Parallel()

	cdi, err := NewCDIHandler(t.TempDir())
	if err != nil {
		t.Fatalf("new CDI handler: %v", err)
	}
	if err := cdi.Initialize(); err != nil {
		t.Fatalf("initialize CDI handler: %v", err)
	}
	rsdPath := filepath.Join(t.TempDir(), "rsd0")
	if err := os.WriteFile(rsdPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write rsd device placeholder: %v", err)
	}

	plugin := NewResourcePlugin(
		consts.AtomResourceName,
		filepath.Join(t.TempDir(), "rbln.sock"),
		filepath.Join(t.TempDir(), "kubelet.sock"),
		cdi,
		map[string]NPUDevice{
			"rbln0": testDevice("null", "sid-a", "0000:06:00.0", "0"),
		},
	)
	plugin.rsdGroupFn = func([]string) (string, error) { return rsdPath, nil }

	response, err := plugin.Allocate(context.Background(), &pluginapi.AllocateRequest{
		ContainerRequests: []*pluginapi.ContainerAllocateRequest{
			{DevicesIds: []string{"rbln0"}},
		},
	})
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	if len(response.ContainerResponses) != 1 {
		t.Fatalf("expected 1 container response, got %d", len(response.ContainerResponses))
	}

	containerResponse := response.ContainerResponses[0]
	if got := containerResponse.Annotations["cdi.k8s.io/rebellions.ai_npu"]; got != consts.CDIKind+"="+consts.BaseCDIDevice {
		t.Fatalf("unexpected runtime annotation value %q", got)
	}
	if len(containerResponse.CdiDevices) != 0 {
		t.Fatalf("expected no allocation CDI devices, got %d", len(containerResponse.CdiDevices))
	}
	if len(containerResponse.Devices) != 2 {
		t.Fatalf("expected 2 device specs, got %d", len(containerResponse.Devices))
	}
	if got := containerResponse.Devices[0]; got.ContainerPath != "/dev/rsd0" || got.HostPath != rsdPath || got.Permissions != "rw" {
		t.Fatalf("unexpected rsd device spec: %+v", got)
	}
	if got := containerResponse.Devices[1]; got.ContainerPath != "/dev/null" || got.HostPath != "/dev/null" || got.Permissions != "rw" {
		t.Fatalf("unexpected device spec: %+v", got)
	}
}
