package handler

import "github.com/multica-ai/multica/server/internal/service"

// durableActorTypes are the governance actor types a filter may name, durable features
// included. The autonomy envelope writes track_record (durable spec 3.4).
func durableActorTypes() []string {
	return []string{
		service.GovernanceActorMember, service.GovernanceActorAgent,
		service.GovernanceActorSystem, service.GovernanceActorPolicy, service.GovernanceActorTrackRecord,
	}
}
