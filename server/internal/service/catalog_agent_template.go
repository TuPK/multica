package service

import (
	"fmt"
	"strings"
)

// AgentInstructionTemplate is the instruction text of a catalog agent template (catalog/agents/<id>), as
// `pnpm catalog:build` embeds it, trimmed the way an agent's instructions are stored. It is how a code path
// that makes an agent from a template, such as the generic lead of a pack, reads its instructions.
func AgentInstructionTemplate(id string) (string, error) {
	d, err := loadCompanyData()
	if err != nil {
		return "", err
	}
	text, ok := d.instructions(id)
	if !ok {
		return "", fmt.Errorf("the catalog has no agent template %q", id)
	}
	return strings.TrimSpace(text), nil
}
