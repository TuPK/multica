package lifecycle

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/multica-ai/multica/platform-management/internal/subscription"
)

// The email copy is a first draft for the owner to review. The templates are
// Go text templates: Vietnamese body, and a subject line in English and in
// Vietnamese.
//
//go:embed templates/*.tmpl
var templateFS embed.FS

// Why a subscription went to free.
const (
	causeTrial    = "trial"
	causeCanceled = "canceled"
	causeLapsed   = "lapsed"
)

// maxNameRunes bounds the workspace name in a subject line.
const maxNameRunes = 80

// maxRecipients bounds the addresses of one email; Resend takes at most 50.
const maxRecipients = 50

// vietnamTime is the zone dates are written in.
var vietnamTime = time.FixedZone("ICT", 7*60*60)

// noticeData is what the email templates see.
type noticeData struct {
	Workspace string
	// URL is the workspace's settings page, where the plan is renewed.
	URL  string
	Plan string
	// DaysLeft is the days until the period ends, for a reminder.
	DaysLeft int
	// The dates are formatted, day first, in Vietnam time. An unknown date is "".
	PeriodEnd, TrialEnd, GraceUntil, FreeOn string
	GraceDays, ReadOnlyDays                 int
	// Cause is why the subscription went to free: causeTrial, causeCanceled or causeLapsed.
	Cause string
}

var templates = func() map[string]*template.Template {
	out := map[string]*template.Template{}
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		body, err := templateFS.ReadFile("templates/" + e.Name())
		if err != nil {
			panic(err)
		}
		out[e.Name()] = template.Must(template.New(e.Name()).Parse(string(body)))
	}
	return out
}()

func render(file string, d noticeData) (subject, body string, err error) {
	t, ok := templates[file]
	if !ok {
		return "", "", fmt.Errorf("lifecycle: no template %s", file)
	}
	var s, b strings.Builder
	if err := t.ExecuteTemplate(&s, "subject", d); err != nil {
		return "", "", fmt.Errorf("lifecycle: %s subject: %w", file, err)
	}
	if err := t.ExecuteTemplate(&b, "body", d); err != nil {
		return "", "", fmt.Errorf("lifecycle: %s body: %w", file, err)
	}
	// A subject is one line whatever the template produced.
	return strings.Join(strings.Fields(s.String()), " "), strings.TrimSpace(b.String()) + "\n", nil
}

func renderReminder(d noticeData) (subject, body string, err error) {
	return render("remind.txt.tmpl", d)
}

func renderState(status string, d noticeData) (subject, body string, err error) {
	switch status {
	case StatusPastDue, StatusReadOnly, StatusFree:
		return render("state_"+status+".txt.tmpl", d)
	}
	return "", "", fmt.Errorf("lifecycle: no state notice for status %q", status)
}

// cleanName makes a workspace name safe for a subject line: control and
// formatting characters become spaces, runs of spaces become one, and the
// result is cut to maxNameRunes.
func cleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxNameRunes {
		s = string([]rune(s)[:maxNameRunes])
	}
	return s
}

func formatDate(t time.Time) string { return t.In(vietnamTime).Format("02/01/2006") }

func formatDatePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatDate(*t)
}

func planLabel(key string) string {
	if key == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(key)
	return string(unicode.ToUpper(r)) + key[n:]
}

// data builds the template data from the subscription the notice is about.
func (s *Service) data(c Contact, sub subscription.Subscription) noticeData {
	d := noticeData{
		Workspace: cleanName(c.Name),
		URL:       strings.TrimRight(s.d.AppURL, "/") + "/" + url.PathEscape(c.Slug) + "/settings",
		Plan:      planLabel(sub.PlanKey),
		PeriodEnd: formatDatePtr(sub.CurrentPeriodEnd), TrialEnd: formatDatePtr(sub.TrialEnd),
		GraceUntil: formatDatePtr(sub.GraceUntil),
		GraceDays:  GraceDays, ReadOnlyDays: ReadOnlyDays,
	}
	if sub.GraceUntil != nil {
		d.FreeOn = formatDate(sub.GraceUntil.Add(ReadOnlyPeriod))
	}
	return d
}

// outcome is what notify did with a notice.
type outcome int

const (
	outcomeSent outcome = iota
	// outcomeDuplicate: the key was already recorded, so the notice went out
	// before.
	outcomeDuplicate
	// outcomeNoRecipients: the workspace has no owner or admin with an email.
	outcomeNoRecipients
)

// compose writes the email for a workspace's contact.
type compose func(c Contact) (subject, body string, err error)

// notify sends a notice once. It records the key first, with ON CONFLICT DO
// NOTHING semantics: a run that finds the key already recorded sends nothing.
// Any failure after the record, and a workspace with no owner or admin
// address, removes the row again, so that a later run can try again.
func (s *Service) notify(ctx context.Context, n Notice, write compose) (outcome, error) {
	inserted, err := s.d.Store.InsertNotice(ctx, n)
	if err != nil {
		return outcomeDuplicate, fmt.Errorf("record notice %s: %w", n.Key, err)
	}
	if !inserted {
		return outcomeDuplicate, nil
	}
	c, err := s.d.Store.Contact(ctx, n.WorkspaceID)
	if err != nil {
		return outcomeDuplicate, s.release(ctx, n, fmt.Errorf("read contact for notice %s: %w", n.Key, err))
	}
	if len(c.Emails) == 0 {
		s.d.Log.Info("lifecycle: no owner or admin with an email; notice not sent",
			"workspace_id", n.WorkspaceID, "notice", n.Key)
		return outcomeNoRecipients, s.release(ctx, n, nil)
	}
	subject, body, err := write(c)
	if err != nil {
		return outcomeDuplicate, s.release(ctx, n, fmt.Errorf("write notice %s: %w", n.Key, err))
	}
	to := c.Emails
	if len(to) > maxRecipients {
		to = to[:maxRecipients]
	}
	if err := s.d.Mail.Send(ctx, Message{To: to, Subject: subject, Body: body}); err != nil {
		return outcomeDuplicate, s.release(ctx, n, fmt.Errorf("send notice %s: %w", n.Key, err))
	}
	return outcomeSent, nil
}

// release removes the notice row and returns cause, joined with the failure to
// remove it. The caller's context may be what ended, so the delete has its own.
func (s *Service) release(ctx context.Context, n Notice, cause error) error {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.d.Store.DeleteNotice(dctx, n.WorkspaceID, n.Key); err != nil {
		s.d.Log.Error("lifecycle: notice row not removed; the notice will not be retried",
			"workspace_id", n.WorkspaceID, "notice", n.Key, "error", err)
		return errors.Join(cause, fmt.Errorf("remove notice %s: %w", n.Key, err))
	}
	return cause
}
