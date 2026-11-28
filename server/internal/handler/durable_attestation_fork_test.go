package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
)

func TestVerifyAttestationMalformedBody(t *testing.T) {
	h := &Handler{}
	withFeatureFlag(t, h, featureflags.DurableOpen, true)

	cases := []struct {
		name       string
		body       []byte
		wantStatus int
		wantReason string
	}{
		{
			name:       "not json",
			body:       []byte("not json"),
			wantStatus: http.StatusOK,
			wantReason: "malformed",
		},
		{
			name:       "oversized body",
			body:       bytes.Repeat([]byte("a"), attestationBodyLimit+1),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "json array",
			body:       []byte("[1,2]"),
			wantStatus: http.StatusOK,
			wantReason: "malformed",
		},
		{
			name:       "json null",
			body:       []byte("null"),
			wantStatus: http.StatusOK,
			wantReason: "keys_unavailable",
		},
		{
			name:       "empty object without issuer host",
			body:       []byte("{}"),
			wantStatus: http.StatusOK,
			wantReason: "keys_unavailable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/attestations/verify", bytes.NewReader(tc.body))
			rec := httptest.NewRecorder()
			h.VerifyAttestation(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			var got VerifyAttestationResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response JSON: %v (body: %s)", err, rec.Body.String())
			}
			if got.Valid {
				t.Fatalf("valid = true, want false (resp: %+v)", got)
			}
			if tc.wantReason != "" && got.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q (resp: %+v)", got.Reason, tc.wantReason, got)
			}
		})
	}
}

func TestVerifyAttestationFlagOffReturns404(t *testing.T) {
	h := &Handler{}
	withFeatureFlag(t, h, featureflags.DurableOpen, false)

	req := httptest.NewRequest(http.MethodPost, "/api/attestations/verify", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.VerifyAttestation(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
