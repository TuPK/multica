package pricechange

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
)

var (
	in30Days  = t0.Add(30 * 24 * time.Hour)
	in31Days  = t0.Add(31 * 24 * time.Hour)
	midnight  = time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	notMidnig = time.Date(2026, 4, 20, 0, 0, 1, 0, time.UTC)
)

func auditsOf(st *fakeStore, action string) []audit.Entry {
	var out []audit.Entry
	for _, e := range st.audits {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func priceOf(st *fakeStore, key, field string) string { return string(st.plans[key].Prices[field]) }

// --- Test 3: a plan has at most one scheduled change.

func TestSecondScheduleIsRefusedWithPriceChangePending(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	if _, err := s.Schedule(context.Background(), admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	_, err := s.Schedule(context.Background(), admin, "pro", seatReq(in31Days, "130000", "1300000"), false, t0)
	e := apiErr(t, err)
	if e.Status != http.StatusConflict || e.Code != "price_change_pending" {
		t.Fatalf("error = %d %s, want 409 price_change_pending", e.Status, e.Code)
	}
	if n := len(st.changes); n != 1 {
		t.Fatalf("%d changes stored, want 1", n)
	}
}

// --- Test 4: the dry run saves nothing.

func TestDryRunReturnsTheImpactAndSavesNothing(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	res, err := s.Schedule(context.Background(), admin, "pro", seatReq(in31Days, "120000", "1200000"), true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Affected != 3 {
		t.Errorf("affected = %d, want 3 paid subscriptions", res.Affected)
	}
	if res.NoticeDate != "2026-03-10" {
		t.Errorf("notice date = %q, want 2026-03-10", res.NoticeDate)
	}
	if len(st.changes) != 0 || len(st.audits) != 0 || st.plans["pro"].Version != 3 {
		t.Errorf("a dry run wrote: %d changes, %d audits, version %d", len(st.changes), len(st.audits), st.plans["pro"].Version)
	}
}

func TestDryRunOfADecreaseHasNoNoticeDate(t *testing.T) {
	s := newService(t, newFakeStore(), &fakeMail{}, nil)
	res, err := s.Schedule(context.Background(), admin, "pro", seatReq(t0, "90000", "900000"), true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if res.NoticeDate != "" || res.Affected != 3 {
		t.Errorf("notice date %q affected %d, want none and 3", res.NoticeDate, res.Affected)
	}
}

// --- Test 7: each schedule, apply and cancel writes one audit row with old and new.

func TestScheduleApplyAndCancelEachWriteOneAuditRow(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	ctx := context.Background()

	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	rows := auditsOf(st, audit.ActionPlanPriceScheduled)
	if len(rows) != 1 {
		t.Fatalf("%d plan.price_scheduled rows, want 1", len(rows))
	}
	e := rows[0]
	if e.ActorType != audit.ActorStaff || e.ActorID != admin.ID || e.TargetType != "plan" || e.TargetID != "pro" {
		t.Errorf("scheduled row = %+v", e)
	}
	// "plan_key" would be redacted by audit.Redact (the key contains "key"), so
	// the detail names the plan "plan"; the row's target_id holds it too.
	wantDetail(t, e, "plan", "pro")
	wantDetail(t, e, "reason", "annual review")
	wantDetail(t, e, "effective_from", in31Days.Format(time.RFC3339))
	wantPrices(t, e, "old", `{"seat_price_month":100000,"seat_price_year":1000000}`)
	wantPrices(t, e, "new", `{"seat_price_month":120000,"seat_price_year":1200000}`)

	if err := s.Run(ctx, in31Days); err != nil {
		t.Fatal(err)
	}
	rows = auditsOf(st, audit.ActionPlanPriceApplied)
	if len(rows) != 1 {
		t.Fatalf("%d plan.price_applied rows, want 1", len(rows))
	}
	if rows[0].ActorType != audit.ActorSystem {
		t.Errorf("applied row actor type = %q, want system", rows[0].ActorType)
	}
	wantPrices(t, rows[0], "old", `{"seat_price_month":100000,"seat_price_year":1000000}`)
	wantPrices(t, rows[0], "new", `{"seat_price_month":120000,"seat_price_year":1200000}`)

	// A new change on the same plan, then canceled.
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days.Add(31*24*time.Hour), "150000", "1500000"), false, in31Days); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cancel(ctx, admin, "pro", in31Days); err != nil {
		t.Fatal(err)
	}
	rows = auditsOf(st, audit.ActionPlanPriceCanceled)
	if len(rows) != 1 {
		t.Fatalf("%d plan.price_canceled rows, want 1", len(rows))
	}
	wantPrices(t, rows[0], "old", `{"seat_price_month":120000,"seat_price_year":1200000}`)
	wantPrices(t, rows[0], "new", `{"seat_price_month":150000,"seat_price_year":1500000}`)
	if rows[0].ActorID != admin.ID {
		t.Errorf("canceled row actor = %q, want the admin", rows[0].ActorID)
	}
}

func wantDetail(t *testing.T, e audit.Entry, key, want string) {
	t.Helper()
	if got, _ := e.Detail[key].(string); got != want {
		t.Errorf("audit detail %s = %v, want %q", key, e.Detail[key], want)
	}
}

func wantPrices(t *testing.T, e audit.Entry, key, want string) {
	t.Helper()
	got, err := json.Marshal(e.Detail[key])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("audit detail %s = %s, want %s", key, got, want)
	}
}

// --- Test 5: the apply job.

func TestApplyJobAppliesOnceAtTheEffectiveDate(t *testing.T) {
	st := newFakeStore()
	cache := &cacheCount{}
	s := newService(t, st, &fakeMail{}, cache)
	ctx := context.Background()
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}

	// One nanosecond early: nothing happens.
	if err := s.Run(ctx, in31Days.Add(-time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if got := priceOf(st, "pro", FieldSeatPriceMonth); got != "100000" || st.plans["pro"].Version != 3 {
		t.Fatalf("applied early: price %s version %d", got, st.plans["pro"].Version)
	}

	// At the effective instant: applied, version +1, state applied.
	if err := s.Run(ctx, in31Days); err != nil {
		t.Fatal(err)
	}
	if got := priceOf(st, "pro", FieldSeatPriceMonth); got != "120000" {
		t.Errorf("month price = %s, want 120000", got)
	}
	if got := priceOf(st, "pro", FieldSeatPriceYear); got != "1200000" {
		t.Errorf("year price = %s, want 1200000", got)
	}
	if v := st.plans["pro"].Version; v != 4 {
		t.Errorf("version = %d, want 4", v)
	}
	if c := st.changes[0]; c.State != StateApplied || c.AppliedAt == nil || !c.AppliedAt.Equal(in31Days) {
		t.Errorf("change = state %s applied_at %v", c.State, c.AppliedAt)
	}
	if cache.n == 0 {
		t.Error("the policy cache was not cleared")
	}
}

func TestApplyJobTwiceInSequenceChangesNothing(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	ctx := context.Background()
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.Run(ctx, in31Days); err != nil {
			t.Fatal(err)
		}
	}
	if v := st.plans["pro"].Version; v != 4 {
		t.Errorf("version = %d after two runs, want 4", v)
	}
	if n := len(auditsOf(st, audit.ActionPlanPriceApplied)); n != 1 {
		t.Errorf("%d applied audit rows, want 1", n)
	}
}

func TestADecreaseTakesEffectInTheRequest(t *testing.T) {
	st := newFakeStore()
	cache := &cacheCount{}
	s := newService(t, st, &fakeMail{}, cache)
	res, err := s.Schedule(context.Background(), admin, "pro", seatReq(t0.Add(-time.Hour), "90000", "900000"), false, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Change.State != StateApplied || !res.Change.EffectiveFrom.Equal(t0) {
		t.Fatalf("result = applied %v state %s effective %v, want applied at now", res.Applied, res.Change.State, res.Change.EffectiveFrom)
	}
	if got := priceOf(st, "pro", FieldSeatPriceMonth); got != "90000" || st.plans["pro"].Version != 4 {
		t.Errorf("price %s version %d, want 90000 and 4", got, st.plans["pro"].Version)
	}
	if len(auditsOf(st, audit.ActionPlanPriceScheduled)) != 1 || len(auditsOf(st, audit.ActionPlanPriceApplied)) != 1 {
		t.Errorf("audit rows: %d scheduled, %d applied, want 1 each",
			len(auditsOf(st, audit.ActionPlanPriceScheduled)), len(auditsOf(st, audit.ActionPlanPriceApplied)))
	}
	if cache.n != 1 {
		t.Errorf("cache cleared %d times, want 1", cache.n)
	}
}

func TestAnIncreaseForAPlanWithoutPaidSubscribersTakesEffectInTheRequest(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	res, err := s.Schedule(context.Background(), admin, "empty", seatReq(t0, "60000", "600000"), false, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || priceOf(st, "empty", FieldSeatPriceMonth) != "60000" {
		t.Fatalf("applied %v price %s", res.Applied, priceOf(st, "empty", FieldSeatPriceMonth))
	}
}

func TestAFutureDecreaseStaysScheduled(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	res, err := s.Schedule(context.Background(), admin, "pro", seatReq(in31Days, "90000", "900000"), false, t0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied || res.Change.State != StateScheduled || priceOf(st, "pro", FieldSeatPriceMonth) != "100000" {
		t.Fatalf("applied %v state %s", res.Applied, res.Change.State)
	}
}

// --- Rule 7: notice before an increase.

func TestIncreaseWithPaidSubscribersNeedsTheNoticePeriod(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	ctx := context.Background()
	_, err := s.Schedule(ctx, admin, "pro", seatReq(in30Days.Add(-time.Second), "120000", "1200000"), false, t0)
	e := apiErr(t, err)
	if e.Status != 409 || e.Code != "price_notice_too_short" {
		t.Fatalf("error = %d %s, want 409 price_notice_too_short", e.Status, e.Code)
	}
	if got := e.Detail["min_effective_from"]; got != in30Days.Format(time.RFC3339) {
		t.Errorf("min_effective_from = %v, want %s", got, in30Days.Format(time.RFC3339))
	}
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in30Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatalf("exactly the notice period: %v", err)
	}
}

// --- Test 6: cancel.

func TestCancelBeforeTheEffectiveDateKeepsThePrice(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMail{}
	s := newService(t, st, mail, nil)
	ctx := context.Background()
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	c, err := s.Cancel(ctx, admin, "pro", t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if c.State != StateCanceled || c.CanceledBy != admin.ID {
		t.Errorf("change = state %s canceled_by %q", c.State, c.CanceledBy)
	}
	// The job later finds nothing to apply.
	if err := s.Run(ctx, in31Days.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := priceOf(st, "pro", FieldSeatPriceMonth); got != "100000" || st.plans["pro"].Version != 3 {
		t.Errorf("price %s version %d, want 100000 and 3", got, st.plans["pro"].Version)
	}
	if len(mail.sent) != 0 {
		t.Errorf("%d emails sent, want none: no notice had gone out", len(mail.sent))
	}
}

func TestCancelWithNothingScheduledIs404(t *testing.T) {
	s := newService(t, newFakeStore(), &fakeMail{}, nil)
	_, err := s.Cancel(context.Background(), admin, "pro", t0)
	if e := apiErr(t, err); e.Status != 404 || e.Code != "price_change_not_found" {
		t.Fatalf("error = %d %s, want 404 price_change_not_found", e.Status, e.Code)
	}
}

func TestCancelAfterTheNoticeSendsOneCorrection(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMail{}
	s := newService(t, st, mail, nil)
	ctx := context.Background()
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(ctx, t0); err != nil {
		t.Fatal(err)
	}
	if got := mail.recipients(); strings.Join(got, ",") != "owner1@example.com,owner3@example.com" {
		t.Fatalf("notice recipients = %v, want the owners of ws1 and ws3, not the workspace pending deletion", got)
	}
	mail.sent = nil

	if _, err := s.Cancel(ctx, admin, "pro", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := mail.recipients(); strings.Join(got, ",") != "owner1@example.com,owner3@example.com" {
		t.Fatalf("correction recipients = %v", got)
	}
	for _, m := range mail.sent {
		if !strings.Contains(m.Body, "10/04/2026") {
			t.Errorf("the correction does not name the canceled date: %q", m.Body)
		}
	}
	// A second cancel is a 404 and sends nothing; neither does a later run.
	if _, err := s.Cancel(ctx, admin, "pro", t0.Add(2*time.Hour)); err == nil {
		t.Fatal("second cancel succeeded")
	}
	if err := s.Run(ctx, t0.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(mail.sent) != 2 {
		t.Errorf("%d emails after the cancel, want 2 (one correction per owner)", len(mail.sent))
	}
}

// --- The notice goes once per owner.

func TestNoticeIsSentOnceEvenWhenTheJobRunsAgain(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMail{}
	s := newService(t, st, mail, nil)
	ctx := context.Background()
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if err := s.Run(ctx, t0.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if len(mail.sent) != 2 {
		t.Fatalf("%d emails, want 2 (ws1 and ws3 owners)", len(mail.sent))
	}
	if st.changes[0].NoticeSentAt == nil {
		t.Error("notice_sent_at not recorded")
	}
}

func TestAFailedSendIsRetriedByTheNextRunWithoutDuplicates(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMail{}
	s := newService(t, st, mail, nil)
	ctx := context.Background()
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0); err != nil {
		t.Fatal(err)
	}
	mail.fail = context.DeadlineExceeded
	if err := s.Run(ctx, t0); err == nil {
		t.Fatal("the run hid a failed send")
	}
	if st.changes[0].NoticeSentAt != nil {
		t.Fatal("notice_sent_at recorded after a failed send")
	}
	mail.fail = nil
	if err := s.Run(ctx, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(mail.sent) != 2 {
		t.Fatalf("%d emails, want 2", len(mail.sent))
	}
}

func TestADecreaseSendsNoNotice(t *testing.T) {
	st := newFakeStore()
	mail := &fakeMail{}
	s := newService(t, st, mail, nil)
	if _, err := s.Schedule(context.Background(), admin, "pro", seatReq(in31Days, "90000", "900000"), false, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Run(context.Background(), t0); err != nil {
		t.Fatal(err)
	}
	if len(mail.sent) != 0 {
		t.Fatalf("%d emails for a decrease, want none", len(mail.sent))
	}
}

// --- Rules 4, 5 and 9(d): validation.

func TestScheduleValidation(t *testing.T) {
	usageReq := func(from time.Time, field, v string) Request {
		return Request{EffectiveFrom: from, Reason: "r", New: Prices{field: raw(v)}}
	}
	cases := []struct {
		name  string
		plan  string
		req   Request
		code  string
		field string
	}{
		{"negative", "pro", seatReq(in31Days, "-1", "1000000"), "price_invalid", FieldSeatPriceMonth},
		{"decimal", "pro", seatReq(in31Days, "100000.5", "1000000"), "price_invalid", FieldSeatPriceMonth},
		{"string", "pro", seatReq(in31Days, `"100000"`, "1000000"), "price_invalid", FieldSeatPriceMonth},
		{"null", "pro", seatReq(in31Days, "null", "1000000"), "price_invalid", FieldSeatPriceMonth},
		{"year above 12 months", "pro", seatReq(in31Days, "100000", "1200001"), "price_invalid", FieldSeatPriceYear},
		{"above the maximum", "pro", seatReq(in31Days, "1000000001", "1000000001"), "price_invalid", FieldSeatPriceMonth},
		{"zero month on a paid seat plan", "pro", seatReq(in31Days, "0", "0"), "price_invalid", FieldSeatPriceMonth},
		{"free plan not zero", "free", seatReq(t0, "1", "1"), "price_invalid", FieldSeatPriceMonth},
		{"seat price on a usage plan", "payg", seatReq(in31Days, "1000", "10000"), "price_invalid", FieldSeatPriceMonth},
		{"unknown meter", "payg", usageReq(midnight, FieldUsageRates, `{"gpu_hours":{"per":1,"price":5}}`), "price_invalid", FieldUsageRates},
		{"per zero", "payg", usageReq(midnight, FieldUsageRates, `{"tasks_run":{"per":0,"price":5}}`), "price_invalid", FieldUsageRates},
		{"negative rate price", "payg", usageReq(midnight, FieldUsageRates, `{"tasks_run":{"per":1,"price":-5}}`), "price_invalid", FieldUsageRates},
		{"discount above 100", "payg", usageReq(midnight, FieldUsageTerms, `{"discount_pct":{"business":101}}`), "price_invalid", FieldUsageTerms},
		{"discount below 0", "payg", usageReq(midnight, FieldUsageTerms, `{"discount_pct":{"business":-1}}`), "price_invalid", FieldUsageTerms},
		{"usage change not at 00:00 UTC", "payg", usageReq(notMidnig, FieldUsageRates, `{"tasks_run":{"per":1000,"price":31000}}`), "price_invalid", "effective_from"},
		{"usage change at a midnight already past", "payg", usageReq(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), FieldUsageRates, `{"tasks_run":{"per":1000,"price":25000}}`), "price_invalid", "effective_from"},
		{"unknown field", "pro", Request{EffectiveFrom: in31Days, Reason: "r", New: Prices{"name": raw(`"x"`)}}, "price_invalid", "name"},
		{"no field", "pro", Request{EffectiveFrom: in31Days, Reason: "r", New: Prices{}}, "price_invalid", "new"},
		{"no reason", "pro", Request{EffectiveFrom: in31Days, New: Prices{FieldSeatPriceMonth: raw("1"), FieldSeatPriceYear: raw("1")}}, "price_invalid", "reason"},
		{"reason over 500", "pro", Request{EffectiveFrom: in31Days, Reason: strings.Repeat("x", 501), New: Prices{FieldSeatPriceMonth: raw("1"), FieldSeatPriceYear: raw("1")}}, "price_invalid", "reason"},
		{"no date", "pro", seatReq(time.Time{}, "120000", "1200000"), "price_invalid", "effective_from"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeStore()
			s := newService(t, st, &fakeMail{}, nil)
			_, err := s.Schedule(context.Background(), admin, tc.plan, tc.req, false, t0)
			e := apiErr(t, err)
			if e.Status != 400 || e.Code != tc.code || e.Detail["field"] != tc.field {
				t.Fatalf("error = %d %s field %v, want 400 %s field %s", e.Status, e.Code, e.Detail["field"], tc.code, tc.field)
			}
			if len(st.changes) != 0 || len(st.audits) != 0 {
				t.Error("a refused change wrote rows")
			}
		})
	}
}

func TestUsageChangeAtMidnightIsAccepted(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	req := Request{EffectiveFrom: midnight, Reason: "r", New: Prices{FieldUsageRates: raw(`{"tasks_run":{"per":1000,"price":25000}}`)}}
	if _, err := s.Schedule(context.Background(), admin, "payg", req, false, t0); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownPlanIs404(t *testing.T) {
	s := newService(t, newFakeStore(), &fakeMail{}, nil)
	_, err := s.Schedule(context.Background(), admin, "nope", seatReq(in31Days, "1", "1"), false, t0)
	if e := apiErr(t, err); e.Status != 404 {
		t.Fatalf("status = %d, want 404", e.Status)
	}
}

func TestLargeChangeNeedsConfirmation(t *testing.T) {
	st := newFakeStore()
	s := newService(t, st, &fakeMail{}, nil)
	ctx := context.Background()
	// 100000 to 160000 is +60 percent.
	req := seatReq(in31Days, "160000", "1600000")
	_, err := s.Schedule(ctx, admin, "pro", req, false, t0)
	e := apiErr(t, err)
	if e.Status != 409 || e.Code != "price_change_confirm_required" || e.Detail["change_pct"] != 60.0 {
		t.Fatalf("error = %d %s %v, want 409 price_change_confirm_required with change_pct 60", e.Status, e.Code, e.Detail)
	}
	// Exactly 50 percent needs no confirmation.
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in31Days, "150000", "1500000"), true, t0); err != nil {
		t.Fatalf("+50 percent: %v", err)
	}
	req.ConfirmLargeChange = true
	if _, err := s.Schedule(ctx, admin, "pro", req, false, t0); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
}

func newServiceWith(t *testing.T, st Store, settings Settings) *Service {
	t.Helper()
	s, err := New(Deps{Store: st, Settings: settings, AppURL: "https://app.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestConfiguredNoticeDaysDecideTheNoticeCheck(t *testing.T) {
	s := newServiceWith(t, newFakeStore(), Settings{NoticeDays: 10})
	ctx := context.Background()
	in10Days := t0.Add(10 * 24 * time.Hour)
	_, err := s.Schedule(ctx, admin, "pro", seatReq(in10Days.Add(-time.Second), "100001", "1000010"), false, t0)
	e := apiErr(t, err)
	if e.Status != 409 || e.Code != "price_notice_too_short" || e.Detail["min_effective_from"] != in10Days.Format(time.RFC3339) {
		t.Fatalf("error = %d %s %v, want 409 price_notice_too_short with a 10 day minimum", e.Status, e.Code, e.Detail)
	}
	if _, err := s.Schedule(ctx, admin, "pro", seatReq(in10Days, "100001", "1000010"), false, t0); err != nil {
		t.Fatalf("exactly 10 days: %v", err)
	}
}

func TestConfiguredConfirmPctDecidesTheConfirmCheck(t *testing.T) {
	s := newServiceWith(t, newFakeStore(), Settings{ConfirmPct: 10})
	// 100000 to 120000 is +20 percent.
	_, err := s.Schedule(context.Background(), admin, "pro", seatReq(in31Days, "120000", "1200000"), false, t0)
	if e := apiErr(t, err); e.Status != 409 || e.Code != "price_change_confirm_required" {
		t.Fatalf("error = %d %s, want 409 price_change_confirm_required", e.Status, e.Code)
	}
}

func TestConfiguredMaxPriceIsRefusedAsInvalid(t *testing.T) {
	s := newServiceWith(t, newFakeStore(), Settings{Max: 150000})
	req := seatReq(in31Days, "150001", "1500010")
	req.ConfirmLargeChange = true
	_, err := s.Schedule(context.Background(), admin, "pro", req, false, t0)
	if e := apiErr(t, err); e.Status != 400 || e.Code != "price_invalid" {
		t.Fatalf("error = %d %s, want 400 price_invalid", e.Status, e.Code)
	}
}
