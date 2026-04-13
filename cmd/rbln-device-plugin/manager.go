package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"
)

type Manager struct {
	mu      sync.Mutex
	config  *Config
	cdi     *CDIHandler
	health  *healthServer
	plugins map[string]*ResourcePlugin
}

func NewManager(ctx context.Context, config *Config) (*Manager, error) {
	cdi, err := NewCDIHandler(config.flags.cdiRoot)
	if err != nil {
		return nil, err
	}
	if err := cdi.Initialize(); err != nil {
		return nil, err
	}

	health, err := startHealthcheck(ctx, config.flags.healthcheckPort)
	if err != nil {
		return nil, err
	}

	return &Manager{
		config:  config,
		cdi:     cdi,
		health:  health,
		plugins: make(map[string]*ResourcePlugin),
	}, nil
}

func (m *Manager) Run(ctx context.Context) error {
	if err := m.reconcile(ctx); err != nil {
		return err
	}
	if m.health != nil {
		m.health.SetServing(true)
	}

	deviceTicker := time.NewTicker(m.config.flags.deviceScanInterval)
	defer deviceTicker.Stop()

	kubeletWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create kubelet socket watcher: %w", err)
	}
	defer func() {
		if err := kubeletWatcher.Close(); err != nil {
			klog.ErrorS(err, "failed to close kubelet socket watcher")
		}
	}()

	if err := kubeletWatcher.Add(m.config.flags.kubeletDevicePluginPath); err != nil {
		return fmt.Errorf("watch kubelet device-plugin directory %s: %w", m.config.flags.kubeletDevicePluginPath, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deviceTicker.C:
			if err := m.reconcile(ctx); err != nil {
				klog.ErrorS(err, "device reconciliation failed")
			}
		case event, ok := <-kubeletWatcher.Events:
			if !ok {
				return fmt.Errorf("kubelet socket watcher closed unexpectedly")
			}
			if !isKubeletRestartEvent(m.config.KubeletSocketPath(), event) {
				continue
			}

			klog.InfoS("detected kubelet socket recreation; restarting device plugins", "event", event)
			if err := m.restart(ctx); err != nil {
				return err
			}
		case err, ok := <-kubeletWatcher.Errors:
			if !ok {
				return fmt.Errorf("kubelet socket watcher closed unexpectedly")
			}
			klog.ErrorS(err, "kubelet socket watcher error")
		}
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.health != nil {
		m.health.SetServing(false)
		m.health.Stop()
	}

	for resourceName, plugin := range m.plugins {
		if err := plugin.Stop(); err != nil {
			klog.ErrorS(err, "failed to stop device plugin", "resourceName", resourceName)
		}
	}
	m.plugins = make(map[string]*ResourcePlugin)
}

func (m *Manager) restart(ctx context.Context) error {
	m.mu.Lock()
	for resourceName, plugin := range m.plugins {
		if err := plugin.Stop(); err != nil {
			m.mu.Unlock()
			return fmt.Errorf("stop device plugin %s: %w", resourceName, err)
		}
	}
	m.plugins = make(map[string]*ResourcePlugin)
	if err := m.cdi.Initialize(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	return m.reconcile(ctx)
}

func (m *Manager) reconcile(ctx context.Context) error {
	groups, err := discoverDeviceGroups(ctx, m.config.flags.useGenericResourceName)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for resourceName, plugin := range m.plugins {
		group, ok := groups[resourceName]
		if !ok {
			if err := plugin.Stop(); err != nil {
				return fmt.Errorf("stop device plugin %s: %w", resourceName, err)
			}
			delete(m.plugins, resourceName)
			continue
		}
		plugin.UpdateDevices(group.Devices)
		delete(groups, resourceName)
	}

	for resourceName, group := range groups {
		socketPath := filepath.Join(m.config.flags.kubeletDevicePluginPath, socketNameForResource(resourceName))
		plugin := NewResourcePlugin(resourceName, socketPath, m.config.KubeletSocketPath(), m.cdi, group.Devices)
		if err := plugin.Start(ctx); err != nil {
			return err
		}
		m.plugins[resourceName] = plugin
	}

	return nil
}

func socketNameForResource(resourceName string) string {
	return fmt.Sprintf("rbln-device-plugin-%s.sock", resourceSlug(resourceName))
}

func isKubeletRestartEvent(kubeletSocketPath string, event fsnotify.Event) bool {
	if filepath.Clean(event.Name) != filepath.Clean(kubeletSocketPath) {
		return false
	}

	return event.Has(fsnotify.Create)
}
