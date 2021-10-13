package plugins

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
)

const localGoPath = "./plugins-local/"

// SetupLocalPlugins setup local plugins environment.
func SetupLocalPlugins(plugins map[string]LocalDescriptor) error {
	if plugins == nil {
		return nil
	}

	uniq := make(map[string]struct{})

	var errs *multierror.Error
	for pAlias, descriptor := range plugins {
		if descriptor.ModuleName == "" {
			errs = multierror.Append(errs, fmt.Errorf("%s: plugin name is missing", pAlias))
		}

		if strings.HasPrefix(descriptor.ModuleName, "/") || strings.HasSuffix(descriptor.ModuleName, "/") {
			errs = multierror.Append(errs, fmt.Errorf("%s: plugin name should not start or end with a /", pAlias))
			continue
		}

		if _, ok := uniq[descriptor.ModuleName]; ok {
			errs = multierror.Append(errs, fmt.Errorf("only one version of a plugin is allowed, there is a duplicate of %s", descriptor.ModuleName))
			continue
		}

		uniq[descriptor.ModuleName] = struct{}{}

		err := checkLocalPluginManifest(descriptor)
		errs = multierror.Append(errs, err)
	}

	return errs.ErrorOrNil()
}

func checkLocalPluginManifest(descriptor LocalDescriptor) error {
	m, err := ReadManifest(localGoPath, descriptor.ModuleName)
	if err != nil {
		return err
	}

	var errs *multierror.Error

	switch m.Type {
	case "middleware", "provider":
		// noop
	default:
		errs = multierror.Append(errs, fmt.Errorf("%s: unsupported type %q", descriptor.ModuleName, m.Type))
	}

	if m.Import == "" {
		errs = multierror.Append(errs, fmt.Errorf("%s: missing import", descriptor.ModuleName))
	}

	if !strings.HasPrefix(m.Import, descriptor.ModuleName) {
		errs = multierror.Append(errs, fmt.Errorf("the import %q must be related to the module name %q", m.Import, descriptor.ModuleName))
	}

	if m.DisplayName == "" {
		errs = multierror.Append(errs, fmt.Errorf("%s: missing DisplayName", descriptor.ModuleName))
	}

	if m.Summary == "" {
		errs = multierror.Append(errs, fmt.Errorf("%s: missing Summary", descriptor.ModuleName))
	}

	if m.TestData == nil {
		errs = multierror.Append(errs, fmt.Errorf("%s: missing TestData", descriptor.ModuleName))
	}

	return errs.ErrorOrNil()
}
