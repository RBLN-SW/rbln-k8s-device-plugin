package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	ca25BridgeIndex = 2
	ca22BridgeIndex = 1
)

var (
	topologyPCISysfsDevicesPath = "/sys/bus/pci/devices"
	topologyPCIAddressPattern   = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)
)

type topologyAllocator struct {
	groups        []deviceGroup
	groupsByID    map[string]deviceGroup
	deviceToGroup map[string]string
}

type deviceGroup struct {
	id        string
	devices   []string
	hasNUMA   bool
	numa      int
	hasBridge bool
	bridge    string
}

type groupChoice struct {
	devices    []string
	groups     []deviceGroup
	newGroups  int
	fullGroups int
	leftover   int
}

func (p *ResourcePlugin) selectPreferredDeviceIDs(availableDeviceIDs, mustIncludeDeviceIDs []string, allocationSize int) ([]string, error) {
	if allocationSize < 0 {
		return nil, fmt.Errorf("allocation size must be non-negative")
	}
	if allocationSize == 0 {
		return []string{}, nil
	}
	if len(availableDeviceIDs) == 0 {
		return nil, fmt.Errorf("preferred allocation request does not include any available device IDs")
	}

	availableDevices, err := p.availableDevices(availableDeviceIDs)
	if err != nil {
		return nil, err
	}
	if allocationSize > len(availableDevices) {
		return nil, fmt.Errorf("requested %d devices, but only %d are available", allocationSize, len(availableDevices))
	}
	if err := validateMustInclude(availableDevices, mustIncludeDeviceIDs, allocationSize); err != nil {
		return nil, err
	}

	selected := newTopologyAllocator(availableDevices).SelectDevices(mustIncludeDeviceIDs, allocationSize)
	if len(selected) != allocationSize {
		return nil, fmt.Errorf("selected %d devices for a request of %d", len(selected), allocationSize)
	}
	return selected, nil
}

func newTopologyAllocator(devices map[string]NPUDevice) *topologyAllocator {
	groupMap := make(map[string]*deviceGroup)
	deviceToGroup := make(map[string]string, len(devices))

	for _, deviceID := range sortedDeviceIDs(devices) {
		device := devices[deviceID]
		groupID := device.Info.SID
		if groupID == "" {
			groupID = deviceID
		}

		group := groupMap[groupID]
		if group == nil {
			group = &deviceGroup{id: groupID}
			if numa, err := strconv.Atoi(device.Info.PCINumaNode); err == nil && numa >= 0 {
				group.hasNUMA = true
				group.numa = numa
			}
			if bridgeIndex, ok := bridgeIndexForDevice(device); ok && device.Info.PCIBusID != "" {
				if bridge, err := getPCIBridge(device.Info.PCIBusID, bridgeIndex); err == nil {
					group.hasBridge = true
					group.bridge = bridge
				}
			}
			groupMap[groupID] = group
		}

		group.devices = append(group.devices, deviceID)
		deviceToGroup[deviceID] = groupID
	}

	groupIDs := make([]string, 0, len(groupMap))
	groupsByID := make(map[string]deviceGroup, len(groupMap))
	for groupID := range groupMap {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)

	groups := make([]deviceGroup, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		group := *groupMap[groupID]
		groups = append(groups, group)
		groupsByID[groupID] = group
	}

	return &topologyAllocator{
		groups:        groups,
		groupsByID:    groupsByID,
		deviceToGroup: deviceToGroup,
	}
}

func (a *topologyAllocator) SelectDevices(mustIncludeDeviceIDs []string, allocationSize int) []string {
	selected, excluded := normalizeMustInclude(mustIncludeDeviceIDs)
	remaining := allocationSize - len(selected)
	if remaining <= 0 {
		return selected
	}

	baseGroupIDs := make(map[string]struct{}, len(selected))
	baseGroups := make([]deviceGroup, 0, len(selected))
	for _, deviceID := range selected {
		groupID, ok := a.deviceToGroup[deviceID]
		if !ok {
			continue
		}
		if _, exists := baseGroupIDs[groupID]; exists {
			continue
		}
		baseGroupIDs[groupID] = struct{}{}
		baseGroups = append(baseGroups, a.groupsByID[groupID])
	}

	candidates := make([]deviceGroup, 0, len(a.groups))
	for _, group := range a.groups {
		freeDevices := make([]string, 0, len(group.devices))
		for _, deviceID := range group.devices {
			if _, used := excluded[deviceID]; !used {
				freeDevices = append(freeDevices, deviceID)
			}
		}
		if len(freeDevices) == 0 {
			continue
		}
		group.devices = freeDevices
		candidates = append(candidates, group)
	}

	var best *groupChoice
	var dfs func(int, []deviceGroup)
	dfs = func(start int, chosen []deviceGroup) {
		if len(chosen) > 0 {
			if choice := buildChoice(chosen, remaining, baseGroupIDs); betterChoice(choice, best, baseGroups) {
				best = choice
			}
		}
		for i := start; i < len(candidates); i++ {
			chosen = append(chosen, candidates[i])
			dfs(i+1, chosen)
			chosen = chosen[:len(chosen)-1]
		}
	}
	dfs(0, nil)

	if best == nil {
		return selected
	}
	return append(selected, best.devices...)
}

func buildChoice(chosen []deviceGroup, remaining int, baseGroupIDs map[string]struct{}) *groupChoice {
	total := 0
	groups := append([]deviceGroup(nil), chosen...)
	for _, group := range groups {
		total += len(group.devices)
	}
	if total < remaining {
		return nil
	}

	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].devices) != len(groups[j].devices) {
			return len(groups[i].devices) > len(groups[j].devices)
		}
		return groups[i].id < groups[j].id
	})

	choice := &groupChoice{}
	left := remaining
	for i, group := range groups {
		if len(group.devices) <= left {
			choice.devices = append(choice.devices, group.devices...)
			choice.groups = append(choice.groups, group)
			if _, exists := baseGroupIDs[group.id]; !exists {
				choice.newGroups++
			}
			choice.fullGroups++
			left -= len(group.devices)
			continue
		}

		partial := group
		for j := i + 1; j < len(groups); j++ {
			if size := len(groups[j].devices); size >= left && size < len(partial.devices) {
				partial = groups[j]
			}
		}
		choice.devices = append(choice.devices, partial.devices[:left]...)
		choice.groups = append(choice.groups, partial)
		if _, exists := baseGroupIDs[partial.id]; !exists {
			choice.newGroups++
		}
		choice.leftover = len(partial.devices) - left
		left = 0
		break
	}

	if left > 0 {
		return nil
	}
	return choice
}

func betterChoice(candidate, current *groupChoice, baseGroups []deviceGroup) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if candidate.newGroups != current.newGroups {
		return candidate.newGroups < current.newGroups
	}
	if candidate.fullGroups != current.fullGroups {
		return candidate.fullGroups > current.fullGroups
	}
	if candidate.leftover != current.leftover {
		return candidate.leftover < current.leftover
	}

	cUnknownNUMA, cUniqueNUMA, cUnknownBridge, cUniqueBridge := topologyMetrics(baseGroups, candidate.groups)
	bUnknownNUMA, bUniqueNUMA, bUnknownBridge, bUniqueBridge := topologyMetrics(baseGroups, current.groups)
	if cUnknownNUMA != bUnknownNUMA {
		return cUnknownNUMA < bUnknownNUMA
	}
	if cUniqueNUMA != bUniqueNUMA {
		return cUniqueNUMA < bUniqueNUMA
	}
	if cUnknownBridge != bUnknownBridge {
		return cUnknownBridge < bUnknownBridge
	}
	if cUniqueBridge != bUniqueBridge {
		return cUniqueBridge < bUniqueBridge
	}

	return strings.Join(candidate.devices, ",") < strings.Join(current.devices, ",")
}

func topologyMetrics(baseGroups, chosenGroups []deviceGroup) (unknownNUMA, uniqueNUMA, unknownBridge, uniqueBridge int) {
	numas := make(map[int]struct{})
	bridges := make(map[string]struct{})
	seen := make(map[string]struct{})

	for _, group := range append(append([]deviceGroup(nil), baseGroups...), chosenGroups...) {
		if _, exists := seen[group.id]; exists {
			continue
		}
		seen[group.id] = struct{}{}
		if group.hasNUMA {
			numas[group.numa] = struct{}{}
		} else {
			unknownNUMA++
		}
		if group.hasBridge {
			bridges[group.bridge] = struct{}{}
		} else {
			unknownBridge++
		}
	}

	return unknownNUMA, len(numas), unknownBridge, len(bridges)
}

func validateMustInclude(availableDevices map[string]NPUDevice, mustIncludeDeviceIDs []string, allocationSize int) error {
	seen := make(map[string]struct{}, len(mustIncludeDeviceIDs))
	for _, deviceID := range mustIncludeDeviceIDs {
		if _, exists := seen[deviceID]; exists {
			continue
		}
		if _, ok := availableDevices[deviceID]; !ok {
			return fmt.Errorf("preferred allocation must include unknown device %q", deviceID)
		}
		seen[deviceID] = struct{}{}
	}
	if len(seen) > allocationSize {
		return fmt.Errorf("preferred allocation must include %d devices, which exceeds allocation size %d", len(seen), allocationSize)
	}
	return nil
}

func normalizeMustInclude(mustIncludeDeviceIDs []string) ([]string, map[string]struct{}) {
	selected := make([]string, 0, len(mustIncludeDeviceIDs))
	excluded := make(map[string]struct{}, len(mustIncludeDeviceIDs))
	for _, deviceID := range mustIncludeDeviceIDs {
		if _, exists := excluded[deviceID]; exists {
			continue
		}
		excluded[deviceID] = struct{}{}
		selected = append(selected, deviceID)
	}
	return selected, excluded
}

func bridgeIndexForDevice(device NPUDevice) (int, bool) {
	switch device.Info.PCIDeviceID {
	case "1251", "1250":
		return ca25BridgeIndex, true
	case "1221", "1220":
		return ca22BridgeIndex, true
	}

	switch {
	case strings.HasPrefix(device.Info.ProductName, "RBLN-CA25"):
		return ca25BridgeIndex, true
	case strings.HasPrefix(device.Info.ProductName, "RBLN-CA22"):
		return ca22BridgeIndex, true
	default:
		return 0, false
	}
}

func getPCIBridge(pciAddr string, bridgeIndex int) (string, error) {
	devicePath := filepath.Join(topologyPCISysfsDevicesPath, pciAddr)
	realPath, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return "", err
	}

	var pciAddresses []string
	for _, segment := range strings.Split(filepath.Clean(realPath), string(os.PathSeparator)) {
		if topologyPCIAddressPattern.MatchString(segment) {
			pciAddresses = append(pciAddresses, segment)
		}
	}
	if len(pciAddresses) <= bridgeIndex {
		return "", fmt.Errorf("PCI hierarchy does not contain bridge at index %d", bridgeIndex)
	}
	return pciAddresses[bridgeIndex], nil
}
