package usage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/billing"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
)

// maxRecipients bounds the addresses of one email; Resend takes at most 50.
const maxRecipients = 50

// noticeEighty emails the workspace's owners and admins that its uninvoiced
// usage is at 80 percent of the credit limit. The month's billing_notice row is
// recorded first and removed again when nobody can be written to or the send
// fails, as the lifecycle notices are, so the month has at most one email.
func (r *Rater) noticeEighty(ctx context.Context, ws pgtype.UUID, accrued, limit int64, now time.Time) error {
	id := uuidText(ws)
	terms, err := db.New(r.pool).GetUsageTerms(ctx, ws)
	if err != nil {
		return err
	}
	n := lifecycle.Notice{WorkspaceID: id, Key: "usage_80:" + now.UTC().Format("2006-01"), Kind: billing.NoticeKindUsage80}
	inserted, err := r.notices.InsertNotice(ctx, n)
	if err != nil || !inserted {
		return err
	}
	c, err := r.notices.Contact(ctx, id)
	if err == nil && len(c.Emails) > 0 {
		subject, body := noticeText(c, r.cfg.AppURL, terms.Currency, accrued, limit)
		err = r.cfg.Mail.Send(ctx, lifecycle.Message{To: c.Emails[:min(len(c.Emails), maxRecipients)], Subject: subject, Body: body})
	}
	if err != nil || len(c.Emails) == 0 {
		// The caller's context may be what ended, so the delete has its own.
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return errors.Join(err, r.notices.DeleteNotice(dctx, id, n.Key))
	}
	return nil
}

// noticeText is a first draft of the copy, for the owner to review: a Vietnamese
// body and a subject in English and in Vietnamese.
func noticeText(c lifecycle.Contact, appURL, currency string, accrued, limit int64) (subject, body string) {
	name := oneLine(c.Name)
	code := strings.ToUpper(currency)
	settings := strings.TrimRight(appURL, "/") + "/" + url.PathEscape(c.Slug) + "/settings"
	subject = fmt.Sprintf("[Delegora] %s: usage reached 80%% of the credit limit / Mức dùng đạt 80%% hạn mức công nợ", name)
	body = fmt.Sprintf(`Xin chào,

Mức dùng chưa lập hóa đơn của workspace "%s" là %d %s, đã đạt 80%% hạn mức công nợ %d %s.

Khi đạt hạn mức, hệ thống lập ngay một hóa đơn tạm cho phần đã dùng. Việc dùng không bị chặn.

Xem mức dùng tại: %s

Bạn nhận thư này vì bạn là chủ sở hữu hoặc quản trị viên của workspace.
Delegora
`, name, accrued, code, limit, code, settings)
	return subject, body
}

// oneLine makes a workspace name safe for a subject line.
func oneLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
