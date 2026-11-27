package handler

import (
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/service"
)

type durableHandlers struct {
	Effects      connectedapps.EffectGate
	Intents      *service.IntentService
	EffectPolicy *service.EffectPolicyStore
}

func (h *Handler) WireDurableServices() {
	h.durable.Effects = &service.EffectService{Queries: h.Queries, Flags: h.FeatureFlags}
	h.durable.Intents = &service.IntentService{Queries: h.Queries, TxStarter: h.TxStarter, Flags: h.FeatureFlags, Tasks: h.TaskService}
	h.durable.EffectPolicy = &service.EffectPolicyStore{Queries: h.Queries, TxStarter: h.TxStarter}
}

func (h *Handler) DurableEffects() connectedapps.EffectGate { return h.durable.Effects }
func (h *Handler) DurableIntents() *service.IntentService   { return h.durable.Intents }
