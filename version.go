package main

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/ry023/semdiff/internal/groups"
	"github.com/ry023/semdiff/internal/versioning"
)

//go:embed VERSION
var embeddedProductVersion string

func productVersion() string {
	return strings.TrimSpace(embeddedProductVersion)
}

type versionOutput struct {
	Version      string             `json:"version"`
	GroupsSchema groupsSchemaOutput `json:"groups_schema"`
}

type groupsSchemaOutput struct {
	Format     string               `json:"format"`
	ReadRanges []groups.SchemaRange `json:"read_ranges"`
	Write      string               `json:"write"`
}

func currentVersionOutput() versionOutput {
	return versionOutput{
		Version: productVersion(),
		GroupsSchema: groupsSchemaOutput{
			Format:     groups.Format,
			ReadRanges: groups.ReadRanges(),
			Write:      groups.WriteVersion,
		},
	}
}

func validateProductVersion() error {
	if _, err := versioning.Parse(productVersion()); err != nil {
		return fmt.Errorf("invalid embedded product version %q: %w", productVersion(), err)
	}
	return nil
}
