package main

import (
	"context"
	"testing"

	rblndevice "github.com/rbln-sw/rblnlib-go/pkg/device"

	"github.com/RBLN-SW/rbln-k8s-device-plugin/pkg/consts"
)

func TestResourceNameForProduct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		productName string
		useGeneric  bool
		expected    string
		wantErr     bool
	}{
		{name: "generic mode", productName: "RBLN-CA25", useGeneric: true, expected: consts.GenericResourceName},
		{name: "legacy atom", productName: "RBLN-CA25", expected: consts.AtomResourceName},
		{name: "legacy rebel", productName: "RBLN-CR03", expected: consts.RebelResourceName},
		{name: "unsupported product", productName: "RBLN-XX01", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual, err := resourceNameForProduct(tc.productName, tc.useGeneric)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.productName)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if actual != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestDiscoverDeviceGroups(t *testing.T) {
	t.Parallel()

	originalGetDevices := getDevices
	t.Cleanup(func() {
		getDevices = originalGetDevices
	})

	getDevices = func(context.Context) ([]rblndevice.Device, error) {
		return []rblndevice.Device{
			{Name: "rbln0", ProductName: "RBLN-CA25"},
			{Name: "rbln1", ProductName: "RBLN-CR03"},
		}, nil
	}

	groups, err := discoverDeviceGroups(context.Background(), false)
	if err != nil {
		t.Fatalf("discover legacy groups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 legacy groups, got %d", len(groups))
	}
	if _, ok := groups[consts.AtomResourceName].Devices["rbln0"]; !ok {
		t.Fatalf("missing ATOM device group entry")
	}
	if _, ok := groups[consts.RebelResourceName].Devices["rbln1"]; !ok {
		t.Fatalf("missing REBEL device group entry")
	}

	groups, err = discoverDeviceGroups(context.Background(), true)
	if err != nil {
		t.Fatalf("discover generic groups: %v", err)
	}
	group, ok := groups[consts.GenericResourceName]
	if !ok {
		t.Fatalf("missing generic resource group")
	}
	if len(group.Devices) != 2 {
		t.Fatalf("expected 2 devices in generic group, got %d", len(group.Devices))
	}
}
