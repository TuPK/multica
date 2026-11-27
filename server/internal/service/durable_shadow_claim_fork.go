package service

import (
	"context"
	"encoding/json"
	"fmt"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A shadow task goes only to a daemon that advertises DaemonCapabilityShadowV1, and only
// for an issue whose project has no local directory: a shadow run never touches the user's
// working copy, in place or as a worktree (durable spec 9.4 (c), D27, D30). The claim gate
// refuses such a task and leaves it queued, so the next candidate is tried.

const (
	// VerdictReasonShadowCapability: the claimant does not advertise the shadow capability.
	VerdictReasonShadowCapability = "shadow_capability_required"
	// VerdictReasonShadowWorkspace: the issue's project has a local directory a shadow run
	// must not touch.
	VerdictReasonShadowWorkspace = "shadow_workspace"
)

// localDirectoryResourceType is the project resource type a user's working copy is.
const localDirectoryResourceType = "local_directory"

type shadowClaimKey struct{}

// WithShadowCapableClaim records on a claim's context whether the claiming daemon
// advertises the shadow capability.
func WithShadowCapableClaim(ctx context.Context, capable bool) context.Context {
	return context.WithValue(ctx, shadowClaimKey{}, capable)
}

func shadowCapableClaim(ctx context.Context) bool {
	capable, _ := ctx.Value(shadowClaimKey{}).(bool)
	return capable
}

// IsShadowTaskContext reports whether a task's context marks it a shadow task.
func IsShadowTaskContext(taskContext []byte) bool {
	return durableRunClassOf(taskContext) == RunClassShadow
}

// shadowClaimRefusal is the reason a claimant may not take a shadow task now, or "".
func shadowClaimRefusal(ctx context.Context, q *db.Queries, task db.AgentTaskQueue) (string, error) {
	if !shadowCapableClaim(ctx) {
		return VerdictReasonShadowCapability, nil
	}
	if !task.IssueID.Valid {
		return VerdictReasonShadowWorkspace, nil
	}
	issue, err := q.GetIssue(ctx, task.IssueID)
	if err != nil {
		return "", fmt.Errorf("load issue for shadow claim: %w", err)
	}
	if !issue.ProjectID.Valid {
		return "", nil
	}
	resources, err := q.ListProjectResources(ctx, issue.ProjectID)
	if err != nil {
		return "", fmt.Errorf("list project resources for shadow claim: %w", err)
	}
	for _, r := range resources {
		if r.ResourceType == localDirectoryResourceType {
			return VerdictReasonShadowWorkspace, nil
		}
	}
	return "", nil
}

// ShadowClaimFields are the claim fields a shadow task carries to the daemon: the shadow
// flag, and the omission of skills (D30). A daemon reads them only when it advertised the
// shadow capability, which the claim gate has already required.
type ShadowClaimFields struct {
	Shadow     bool `json:"shadow,omitempty"`
	OmitSkills bool `json:"omit_skills,omitempty"`
}

// ShadowClaimFieldsOf returns the claim fields for a task, or the zero value for any other. A shadow
// task omits its skills unless its context says omit_skills is false: a necessity test's shadow
// agent holds the skills it should keep (spec 10.2).
func ShadowClaimFieldsOf(taskContext []byte) ShadowClaimFields {
	if !IsShadowTaskContext(taskContext) {
		return ShadowClaimFields{}
	}
	omit := true
	var marks struct {
		OmitSkills *bool `json:"omit_skills"`
	}
	if json.Unmarshal(taskContext, &marks) == nil && marks.OmitSkills != nil {
		omit = *marks.OmitSkills
	}
	return ShadowClaimFields{Shadow: true, OmitSkills: omit}
}
