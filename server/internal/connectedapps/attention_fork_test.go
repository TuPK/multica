package connectedapps

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type fakeAttentionGate struct {
	answeredID      pgtype.UUID
	answeredSeconds *int
	calls           int
}

func (f *fakeAttentionGate) AdmitAppRequest(_ context.Context, _ db.AppActionRequest) error {
	return nil
}

func (f *fakeAttentionGate) AnswerAppRequest(_ context.Context, id pgtype.UUID, actualSeconds *int) error {
	f.calls++
	f.answeredID = id
	f.answeredSeconds = actualSeconds
	return nil
}

func TestAnswerAttentionForwardsActualSeconds(t *testing.T) {
	gate := &fakeAttentionGate{}
	svc := &Service{attention: gate}
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	secs := 42
	svc.answerAttention(context.Background(), id, &secs)
	if gate.calls != 1 || gate.answeredID != id || gate.answeredSeconds == nil || *gate.answeredSeconds != 42 {
		t.Fatalf("with seconds: calls=%d id=%v secs=%v, want 1, %v, 42", gate.calls, gate.answeredID, gate.answeredSeconds, id)
	}

	svc.answerAttention(context.Background(), id)
	if gate.calls != 2 || gate.answeredSeconds != nil {
		t.Fatalf("omitted seconds: calls=%d secs=%v, want 2, nil", gate.calls, gate.answeredSeconds)
	}

	svc.answerAttention(context.Background(), id, nil)
	if gate.calls != 3 || gate.answeredSeconds != nil {
		t.Fatalf("explicit nil: calls=%d secs=%v, want 3, nil", gate.calls, gate.answeredSeconds)
	}
}
