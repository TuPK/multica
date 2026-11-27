package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/service"
)

// attestationKeysMaxBytes caps the key document fetched from an issuer.
const attestationKeysMaxBytes = 256 << 10

// attestationKeysSource fetches an issuer's published key document. The default is the
// SSRF-safe fetcher; tests swap it for a fake.
type attestationKeysSource interface {
	Get(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, error)
}

var attestationKeysGetter attestationKeysSource = hub.NewSafeFetcher(hub.FetcherOptions{})

var attestationCmd = &cobra.Command{
	Use:   "attestation",
	Short: "Issue and verify signed track records of agents",
	Long: `A track record attestation is a signed statement of an agent's scored work over a period.
An owner or an admin issues one for an agent with at least 30 scored outcomes in the period.
Anyone can verify it against the published keys of the host that signed it.

  multica attestation issue --agent <agent-id> --from 2026-08-01 --to 2026-10-31 --out record.json
  multica attestation verify record.json

verify exits 0 when the signature checks out and 1 when it does not. It fetches the keys over
HTTPS from the host the document names, so the server you talk to is not trusted to say so.`,
}

var attestationIssueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Sign an agent's track record for a period",
	Args:  cobra.NoArgs,
	RunE:  runAttestationIssue,
}

var attestationVerifyCmd = &cobra.Command{
	Use:   "verify <file>",
	Short: "Check an attestation's signature against its issuer's published keys",
	Args:  cobra.ExactArgs(1),
	RunE:  runAttestationVerify,
}

func runAttestationIssue(cmd *cobra.Command, _ []string) error {
	agent, _ := cmd.Flags().GetString("agent")
	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	template, _ := cmd.Flags().GetString("template-ref")
	showWorkspace, _ := cmd.Flags().GetBool("show-workspace")
	out, _ := cmd.Flags().GetString("out")
	if strings.TrimSpace(agent) == "" || strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return fmt.Errorf("--agent, --from and --to are required; dates are YYYY-MM-DD")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	body := map[string]any{
		"agent_id": agent, "from": from, "to": to, "template_ref": template, "show_workspace": showWorkspace,
	}
	var doc json.RawMessage
	if err := client.PostJSON(ctx, "/api/attestations/issue", body, &doc); err != nil {
		return fmt.Errorf("issue attestation: %w", err)
	}
	if out == "" {
		_, err := fmt.Fprintln(os.Stdout, string(doc))
		return err
	}
	if err := os.WriteFile(out, append(doc, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	fmt.Fprintf(os.Stdout, "wrote %s\n", out)
	return nil
}

func runAttestationVerify(_ *cobra.Command, args []string) error {
	raw, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("read %s: %w", args[0], err)
	}
	var probe struct {
		Issuer struct {
			Host  string `json:"host"`
			KeyID string `json:"key_id"`
		} `json:"issuer"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return fmt.Errorf("not an attestation: %w", err)
	}
	host := probe.Issuer.Host
	if host == "" {
		return fmt.Errorf("the attestation names no issuer host")
	}
	keysBody, _, err := attestationKeysGetter.Get(context.Background(), "https://"+host+service.AttestationKeysPath, attestationKeysMaxBytes)
	if err != nil {
		return fmt.Errorf("fetch the keys of %s: %w", host, err)
	}
	keys, err := service.ParsePublicKeys(host, keysBody)
	if err != nil {
		return err
	}
	if err := service.Verify(raw, keys); err != nil {
		return fmt.Errorf("attestation does not verify against %s: %w", host, err)
	}
	fmt.Fprintf(os.Stdout, "valid: issued by %s (key %s)\n", host, probe.Issuer.KeyID)
	return nil
}

func init() {
	attestationIssueCmd.Flags().String("agent", "", "the agent's id")
	attestationIssueCmd.Flags().String("from", "", "first day of the period, YYYY-MM-DD")
	attestationIssueCmd.Flags().String("to", "", "last day of the period, YYYY-MM-DD")
	attestationIssueCmd.Flags().String("template-ref", "", "the Hub item the agent came from, as hub:<item>@<version>")
	attestationIssueCmd.Flags().Bool("show-workspace", false, "name the workspace in the attestation")
	attestationIssueCmd.Flags().String("out", "", "write the attestation to this file instead of printing it")
	attestationCmd.AddCommand(attestationIssueCmd, attestationVerifyCmd)
}
