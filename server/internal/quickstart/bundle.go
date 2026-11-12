// Package quickstart installs the team's Claude Code setup on a machine:
// the claude CLI, plugin marketplaces, plugins and workspace skills. The
// server sends the bundle and the daemon or the `multica quickstart` command
// runs the install, so both paths share this code.
package quickstart

import (
	"encoding/json"
	"fmt"

	"github.com/multica-ai/multica/server/internal/catalog"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// bundleJSON lists what a fresh machine should have. Its source is
// catalog/bundles/quickstart/template.json; `pnpm catalog:build` writes the file the
// server embeds. Edit the catalog to add or remove skills and plugins.
var bundleJSON = catalog.QuickstartBundleJSON()

// LoadBundle returns the embedded quickstart bundle.
func LoadBundle() (protocol.QuickstartBundle, error) {
	var bundle protocol.QuickstartBundle
	if len(bundleJSON) == 0 {
		return protocol.QuickstartBundle{}, fmt.Errorf("quickstart bundle: %s", catalog.BuildHint)
	}
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		return protocol.QuickstartBundle{}, fmt.Errorf("parse quickstart bundle: %w", err)
	}
	return bundle, nil
}
