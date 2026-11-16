package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/integrations/dingtalk"
	"github.com/multica-ai/multica/server/internal/integrations/lark"
	"github.com/multica-ai/multica/server/internal/integrations/slack"
	"github.com/multica-ai/multica/server/internal/integrations/telegram"
	"github.com/multica-ai/multica/server/internal/integrations/wecom"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Spec 6.6: a chat-app binding redeem writes a binding to the workspace, so a
// viewer is refused with 403 viewer_read_only and no binding row is written.
// A member redeeming the same kind of token is still admitted.
func TestBindingRedeemRefusesAViewerAndAdmitsAMember(t *testing.T) {
	if testPool == nil {
		t.Skip("integration database not available")
	}
	ctx := context.Background()
	queries := db.New(testPool)
	h := &Handler{
		Queries:               queries,
		Bus:                   events.New(),
		LarkBindingTokens:     lark.NewBindingTokenService(queries, testPool),
		SlackBindingTokens:    slack.NewBindingTokenService(queries, testPool),
		DingTalkBindingTokens: dingtalk.NewBindingTokenService(queries, testPool),
		TelegramBindingTokens: telegram.NewBindingTokenService(queries, testPool),
		WecomBindingTokens:    wecom.NewBindingTokenService(queries, testPool),
	}

	cases := []struct {
		name        string
		channelType string
		redeem      http.HandlerFunc
		mint        func(ws, installation pgtype.UUID, channelUser string) (string, error)
	}{
		{"lark", "feishu", h.RedeemLarkBindingToken, func(ws, in pgtype.UUID, u string) (string, error) {
			tok, err := h.LarkBindingTokens.Mint(ctx, ws, in, lark.OpenID(u))
			return tok.Raw, err
		}},
		{"slack", "slack", h.RedeemSlackBindingToken, func(ws, in pgtype.UUID, u string) (string, error) {
			tok, err := h.SlackBindingTokens.Mint(ctx, ws, in, u)
			return tok.Raw, err
		}},
		{"dingtalk", "dingtalk", h.RedeemDingTalkBindingToken, func(ws, in pgtype.UUID, u string) (string, error) {
			tok, err := h.DingTalkBindingTokens.Mint(ctx, ws, in, u)
			return tok.Raw, err
		}},
		{"telegram", "telegram", h.RedeemTelegramBindingToken, func(ws, in pgtype.UUID, u string) (string, error) {
			tok, err := h.TelegramBindingTokens.Mint(ctx, ws, in, u)
			return tok.Raw, err
		}},
		{"wecom", "wecom", h.RedeemWecomBindingToken, func(ws, in pgtype.UUID, u string) (string, error) {
			tok, err := h.WecomBindingTokens.(*wecom.BindingTokenService).Mint(ctx, ws, in, u)
			return tok.Raw, err
		}},
	}

	viewerID := dbfx.User(t, "Binding Viewer", testutil.UniqueEmail("binding-viewer"))
	dbfx.Member(t, testWorkspaceID, viewerID, workspaceguard.RoleViewer)
	memberID := dbfx.User(t, "Binding Member", testutil.UniqueEmail("binding-member"))
	dbfx.Member(t, testWorkspaceID, memberID, "member")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wsUUID := parseUUID(testWorkspaceID)
			installation := parseUUID(uuid.NewString())
			redeem := func(userID string) (*httptest.ResponseRecorder, string) {
				channelUser := "u" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
				raw, err := tc.mint(wsUUID, installation, channelUser)
				if err != nil {
					t.Fatalf("mint token: %v", err)
				}
				dbfx.Cleanup(t, `DELETE FROM channel_binding_token WHERE installation_id = $1`, uuidToString(installation))
				dbfx.Cleanup(t, `DELETE FROM channel_user_binding WHERE installation_id = $1`, uuidToString(installation))
				req := httptest.NewRequest(http.MethodPost, "/api/"+tc.name+"/binding/redeem", strings.NewReader(`{"token":"`+raw+`"}`))
				req.Header.Set("X-User-ID", userID)
				rec := httptest.NewRecorder()
				tc.redeem(rec, req)
				return rec, channelUser
			}
			bindings := func() int {
				var n int
				dbfx.QueryRow(t, `SELECT count(*) FROM channel_user_binding WHERE installation_id = $1 AND channel_type = $2`,
					uuidToString(installation), tc.channelType).Scan(&n)
				return n
			}

			rec, _ := redeem(viewerID)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("viewer: status %d, want 403 (body %s)", rec.Code, rec.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != workspaceguard.CodeViewerReadOnly {
				t.Fatalf("viewer: body %s, want code %q", rec.Body.String(), workspaceguard.CodeViewerReadOnly)
			}
			if n := bindings(); n != 0 {
				t.Fatalf("viewer: %d binding rows written, want 0", n)
			}

			rec, _ = redeem(memberID)
			if rec.Code != http.StatusOK {
				t.Fatalf("member: status %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			if n := bindings(); n != 1 {
				t.Fatalf("member: %d binding rows written, want 1", n)
			}
		})
	}
}
