package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	rblndevice "github.com/rbln-sw/rblnlib-go/pkg/device"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/RBLN-SW/rbln-k8s-device-plugin/pkg/consts"
)

var getDevices = rblndevice.GetDevices

type NPUDevice struct {
	Info rblndevice.Device
}

type DeviceGroup struct {
	ResourceName string
	Devices      map[string]NPUDevice
}

func discoverDeviceGroups(ctx context.Context, useGenericResourceName bool) (map[string]DeviceGroup, error) {
	devices, err := getDevices(ctx)
	if err != nil {
		return nil, err
	}

	groups := make(map[string]DeviceGroup)
	for _, device := range devices {
		resourceName, err := resourceNameForProduct(device.ProductName, useGenericResourceName)
		if err != nil {
			return nil, err
		}
		group, ok := groups[resourceName]
		if !ok {
			group = DeviceGroup{
				ResourceName: resourceName,
				Devices:      make(map[string]NPUDevice),
			}
		}
		group.Devices[device.Name] = NPUDevice{
			Info: device,
		}
		groups[resourceName] = group
	}

	return groups, nil
}

func resourceNameForProduct(productName string, useGenericResourceName bool) (string, error) {
	if useGenericResourceName {
		return consts.GenericResourceName, nil
	}

	switch {
	case strings.HasPrefix(productName, "RBLN-CR"):
		return consts.RebelResourceName, nil
	case strings.HasPrefix(productName, "RBLN-CA"):
		return consts.AtomResourceName, nil
	default:
		return "", fmt.Errorf("unsupported ProductName %q: expected prefix RBLN-CR or RBLN-CA", productName)
	}
}

func toPluginDevice(device rblndevice.Device) *pluginapi.Device {
	return &pluginapi.Device{
		ID:       device.Name,
		Health:   pluginapi.Healthy,
		Topology: topologyForDevice(device.PCINumaNode),
	}
}

func topologyForDevice(numaNode string) *pluginapi.TopologyInfo {
	if numaNode == "" {
		return nil
	}
	id, err := strconv.ParseInt(numaNode, 10, 64)
	if err != nil {
		return nil
	}
	return &pluginapi.TopologyInfo{
		Nodes: []*pluginapi.NUMANode{{ID: id}},
	}
}

func clonePluginDevices(devices map[string]NPUDevice) []*pluginapi.Device {
	ids := sortedDeviceIDs(devices)
	pluginDevices := make([]*pluginapi.Device, 0, len(ids))
	for _, id := range ids {
		pluginDevices = append(pluginDevices, toPluginDevice(devices[id].Info))
	}
	return pluginDevices
}

func sortedDeviceIDs(devices map[string]NPUDevice) []string {
	ids := make([]string, 0, len(devices))
	for id := range devices {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func resourceSlug(resourceName string) string {
	slug := strings.ToLower(resourceName)
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	return slug
}
