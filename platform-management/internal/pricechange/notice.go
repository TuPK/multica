package pricechange

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/multica-ai/multica/platform-management/internal/lifecycle"
)

// The email copy is a first draft for the owner to review: a Vietnamese body and
// a subject line in English and in Vietnamese, as the lifecycle notices have.

// maxRecipients bounds the addresses of one email; Resend takes at most 50.
const maxRecipients = 50

// vietnamTime is the zone dates are written in.
var vietnamTime = time.FixedZone("ICT", 7*60*60)

func formatDate(t time.Time) string { return t.In(vietnamTime).Format("02/01/2006") }

// label makes a plan key readable: "pro" is "Pro".
func label(key string) string {
	if key == "" {
		return key
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

// cleanName makes a workspace name safe for a subject line.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// priceLines lists what changes, for the body of the notice.
func priceLines(c Change) string {
	var b strings.Builder
	for _, f := range []string{FieldSeatPriceMonth, FieldSeatPriceYear} {
		if raw, ok := c.New[f]; ok {
			fmt.Fprintf(&b, "- %s: %s -> %s\n", f, string(c.Old[f]), string(raw))
		}
	}
	if touchesUsage(c.New) {
		b.WriteString("- Bảng giá hoặc điều khoản theo mức dùng thay đổi.\n")
	}
	return b.String()
}

func (s *Service) compose(kind string, ct lifecycle.Contact, c Change) (subject, body string) {
	name, plan := cleanName(ct.Name), label(c.PlanKey)
	settings := strings.TrimRight(s.d.AppURL, "/") + "/" + url.PathEscape(ct.Slug) + "/settings"
	date := formatDate(c.EffectiveFrom)
	if kind == KindPriceChangeCanceled {
		return fmt.Sprintf("[Multica] Price change of the %s plan canceled / Hủy thay đổi giá gói %s", plan, plan),
			fmt.Sprintf(`Xin chào,

Thay đổi giá của gói %s dự kiến từ ngày %s đã bị hủy. Giá hiện tại của workspace "%s" giữ nguyên.

Xem gói tại: %s

Bạn nhận thư này vì bạn là chủ sở hữu của workspace.
Multica
`, plan, date, name, settings)
	}
	return fmt.Sprintf("[Multica] Price change of the %s plan from %s / Gói %s đổi giá từ %s", plan, date, plan, date),
		fmt.Sprintf(`Xin chào,

Giá gói %s của workspace "%s" sẽ thay đổi từ ngày %s:
%s
Giá mới áp dụng cho các đơn tạo từ ngày đó. Đơn đã tạo và kỳ đã thanh toán không đổi.

Xem gói tại: %s

Bạn nhận thư này vì bạn là chủ sở hữu của workspace.
Multica
`, plan, name, date, priceLines(c), settings)
}

// announce emails the owner of each workspace with a paid subscription on the
// change's plan, except a workspace pending deletion. Each workspace gets a
// kind of notice for a change once: the billing_notice row is recorded first,
// and removed again when the send fails, so the next run tries again.
func (s *Service) announce(ctx context.Context, c Change, kind string) error {
	if s.d.Mail == nil {
		return nil
	}
	workspaces, err := s.d.Store.PaidWorkspaces(ctx, c.PlanKey)
	if err != nil {
		return err
	}
	var errs []error
	for _, w := range workspaces {
		if w.PendingDeletion {
			continue
		}
		n := lifecycle.Notice{WorkspaceID: w.ID, Key: kind + ":" + c.ID, Kind: kind}
		if err := s.notify(ctx, n, c); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) notify(ctx context.Context, n lifecycle.Notice, c Change) error {
	inserted, err := s.d.Store.InsertNotice(ctx, n)
	if err != nil {
		return fmt.Errorf("record notice %s: %w", n.Key, err)
	}
	if !inserted {
		return nil
	}
	ct, err := s.d.Store.Contact(ctx, n.WorkspaceID)
	if err == nil && len(ct.Emails) == 0 {
		s.d.Log.Info("pricechange: no owner with an email; notice not sent", "workspace_id", n.WorkspaceID, "notice", n.Key)
	}
	if err != nil || len(ct.Emails) == 0 {
		return errors.Join(err, s.release(ctx, n))
	}
	subject, body := s.compose(n.Kind, ct, c)
	to := ct.Emails
	if len(to) > maxRecipients {
		to = to[:maxRecipients]
	}
	if err := s.d.Mail.Send(ctx, lifecycle.Message{To: to, Subject: subject, Body: body}); err != nil {
		return errors.Join(fmt.Errorf("send notice %s: %w", n.Key, err), s.release(ctx, n))
	}
	return nil
}

// release removes the notice row so that a later run can try again. The
// caller's context may be what ended, so the delete has its own.
func (s *Service) release(ctx context.Context, n lifecycle.Notice) error {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.d.Store.DeleteNotice(dctx, n.WorkspaceID, n.Key); err != nil {
		return fmt.Errorf("remove notice %s: %w", n.Key, err)
	}
	return nil
}

// sendNotices tells the subscribers of each scheduled increase, once. A change
// is marked only when every workspace was handled.
func (s *Service) sendNotices(ctx context.Context, now time.Time) error {
	pending, err := s.d.Store.NoticeDue(ctx, BatchSize)
	if err != nil {
		return fmt.Errorf("list price changes to announce: %w", err)
	}
	var errs []error
	for _, c := range pending {
		if !isIncrease(c.Old, c.New) {
			continue
		}
		if err := s.announce(ctx, c, KindPriceChange); err != nil {
			errs = append(errs, fmt.Errorf("announce price change %s: %w", c.ID, err))
			continue
		}
		if err := s.d.Store.MarkNoticeSent(ctx, c.ID, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
