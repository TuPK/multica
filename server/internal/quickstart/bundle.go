// Package quickstart installs the team's Claude Code setup on a machine:
// the claude CLI, plugin marketplaces, plugins and workspace skills. The
// server sends the bundle and the daemon or the `multica quickstart` command
// runs the install, so both paths share this code.
package quickstart

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// bundleJSON lists what a fresh machine should have. Edit bundle.json to add
// or remove skills and plugins.
//
//go:embed bundle.json
var bundleJSON []byte

// LoadBundle returns the embedded quickstart bundle.
func LoadBundle() (protocol.QuickstartBundle, error) {
	var bundle protocol.QuickstartBundle
	if err := json.Unmarshal(bundleJSON, &bundle); err != nil {
		return protocol.QuickstartBundle{}, fmt.Errorf("parse quickstart bundle: %w", err)
	}
	return bundle, nil
}
