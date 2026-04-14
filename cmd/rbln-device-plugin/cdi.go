package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"

	"github.com/RBLN-SW/rbln-k8s-device-plugin/pkg/consts"
)

type CDIHandler struct {
	root string
}

func NewCDIHandler(root string) (*CDIHandler, error) {
	return &CDIHandler{
		root: root,
	}, nil
}

func (cdi *CDIHandler) Initialize() error {
	return cdi.cleanupTransientSpecs()
}

func (cdi *CDIHandler) cleanupTransientSpecs() error {
	entries, err := os.ReadDir(cdi.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	prefix := fmt.Sprintf("%s-%s_", cdi.vendor(), cdi.class())
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		if err := os.Remove(filepath.Join(cdi.root, entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

func (cdi *CDIHandler) RuntimeAnnotations() (map[string]string, error) {
	return cdiapi.UpdateAnnotations(
		nil,
		cdi.vendor(),
		cdi.class(),
		[]string{cdiparser.QualifiedName(cdi.vendor(), cdi.class(), consts.BaseCDIDevice)},
	)
}

func (cdi *CDIHandler) class() string {
	return consts.CDIClass
}

func (cdi *CDIHandler) vendor() string {
	return consts.CDIVendor
}
