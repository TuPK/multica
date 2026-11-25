package service

import "testing"

func TestSurfaceOf(t *testing.T) {
	cases := []struct {
		name string
		in   EffectRef
		want string
	}{
		{"connected app splits toolkit before first underscore", EffectRef{"connected_app", "GMAIL_SEND_EMAIL"}, "app:GMAIL"},
		{"connected app without underscore keeps whole target", EffectRef{"connected_app", "github"}, "app:github"},
		{"connected app with leading underscore keeps whole target", EffectRef{"connected_app", "_X"}, "app:_X"},
		{"channel uses target as type", EffectRef{"channel", "slack"}, "channel:slack"},
		{"declared URL uses host without port", EffectRef{"declared", "https://api.example.com:8443/v1/x"}, "external:api.example.com"},
		{"declared smtp host without port", EffectRef{"declared", "smtp:mail.example.com:587"}, "external:mail.example.com"},
		{"declared host prefix", EffectRef{"declared", "host:api.example.com:443"}, "external:api.example.com"},
		{"declared unparseable target", EffectRef{"declared", "not a url"}, "external:unknown"},
		{"declared empty target", EffectRef{"declared", ""}, "external:unknown"},
		{"other source keeps first two work_class segments", EffectRef{"cli", "flow:abc:stage_1"}, "area:flow:abc"},
		{"other source with one segment keeps it", EffectRef{"webhook", "general"}, "area:general"},
		{"other source with squad segment", EffectRef{"publish_channel", "squad:123"}, "area:squad:123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SurfaceOf(tc.in); got != tc.want {
				t.Fatalf("SurfaceOf(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReachStateTable(t *testing.T) {
	cases := []struct {
		name string
		in   SurfaceStats
		want string
	}{
		{
			"agent operable: allowed agent effects and no gap asks",
			SurfaceStats{EffectsAgent30d: 5, Effects30d: 5, AllowedAgent: true},
			"agent_operable",
		},
		{
			"gap asks block operable and fall through to confirm",
			SurfaceStats{EffectsAgent30d: 4, Effects30d: 4, AgentConfirm30d: 3, AllowedAgent: true, GapAsks30d: 2},
			"agent_with_confirm",
		},
		{
			"agent mostly through confirm",
			SurfaceStats{EffectsAgent30d: 4, Effects30d: 4, AgentConfirm30d: 3},
			"agent_with_confirm",
		},
		{
			"exactly half through confirm is not a majority",
			SurfaceStats{EffectsAgent30d: 4, Effects30d: 4, AgentConfirm30d: 2},
			"unknown",
		},
		{
			"only declared effects",
			SurfaceStats{Effects30d: 2, DeclaredEffects30d: 2},
			"declared_only",
		},
		{
			"declared only beats missing access",
			SurfaceStats{Effects30d: 2, DeclaredEffects30d: 2, MissingAccess: true},
			"declared_only",
		},
		{
			"missing access with no agent effects is human only",
			SurfaceStats{Effects30d: 3, MissingAccess: true},
			"human_only",
		},
		{
			"missing access with agent effects is not human only",
			SurfaceStats{EffectsAgent30d: 1, Effects30d: 1, MissingAccess: true},
			"unknown",
		},
		{
			"no data is unknown",
			SurfaceStats{},
			"unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReachState(tc.in); got != tc.want {
				t.Fatalf("ReachState(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
