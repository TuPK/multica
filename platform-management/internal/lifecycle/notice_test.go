package lifecycle

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
)

func sampleData() noticeData {
	return noticeData{
		Workspace: "Acme Studio", URL: "https://app.example.com/acme/settings", Plan: "Pro",
		DaysLeft: 7, PeriodEnd: "31/03/2026", TrialEnd: "15/03/2026", GraceUntil: "07/04/2026", FreeOn: "07/05/2026",
		GraceDays: 7, ReadOnlyDays: 30,
	}
}

func hasVietnamese(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII && unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// Every notice is Vietnamese in the body, has an English subject, and names
// the workspace and the page to act on.
func TestNoticeTemplatesRender(t *testing.T) {
	type render struct {
		name string
		do   func(d noticeData) (string, string, error)
		want []string // the facts the body must carry
	}
	cases := []render{
		{"reminder", func(d noticeData) (string, string, error) { return renderReminder(d) }, []string{"7", "31/03/2026"}},
		{"past_due", func(d noticeData) (string, string, error) { return renderState(StatusPastDue, d) }, []string{"07/04/2026"}},
		{"read_only", func(d noticeData) (string, string, error) { return renderState(StatusReadOnly, d) }, []string{"07/05/2026"}},
		{"free", func(d noticeData) (string, string, error) {
			d.Cause = causeLapsed
			return renderState(StatusFree, d)
		}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subject, body, err := c.do(sampleData())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(subject, "Acme Studio") {
				t.Errorf("subject %q does not name the workspace", subject)
			}
			// An English subject line, with the Vietnamese beside it.
			if !strings.ContainsAny(subject, "abcdefghijklmnopqrstuvwxyz") || !strings.Contains(subject, " / ") {
				t.Errorf("subject %q is not English / Vietnamese", subject)
			}
			if !hasVietnamese(body) {
				t.Errorf("body is not Vietnamese:\n%s", body)
			}
			for _, want := range append([]string{"Acme Studio", "https://app.example.com/acme/settings"}, c.want...) {
				if !strings.Contains(body, want) {
					t.Errorf("body lacks %q:\n%s", want, body)
				}
			}
			for _, bad := range []string{"<no value>", "%!", "{{", "}}"} {
				if strings.Contains(subject+body, bad) {
					t.Errorf("template left %q in the text:\n%s", bad, body)
				}
			}
			if strings.ContainsAny(subject, "\r\n") {
				t.Errorf("subject %q spans lines", subject)
			}
		})
	}
}

func TestReminderNamesTheDaysLeft(t *testing.T) {
	var subjects []string
	for _, days := range ReminderDays {
		d := sampleData()
		d.DaysLeft = days
		subject, body, err := renderReminder(d)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(subject+body, strconv.Itoa(days)) {
			t.Errorf("%d-day reminder does not say %d", days, days)
		}
		subjects = append(subjects, subject)
	}
	if subjects[0] == subjects[1] || subjects[1] == subjects[2] {
		t.Errorf("the reminders share a subject: %q", subjects)
	}
}

// The free notice says why the workspace is on the free plan.
func TestFreeNoticeDependsOnTheCause(t *testing.T) {
	bodies := map[string]string{}
	for _, cause := range []string{causeTrial, causeCanceled, causeLapsed} {
		d := sampleData()
		d.Cause = cause
		_, body, err := renderState(StatusFree, d)
		if err != nil {
			t.Fatalf("%s: %v", cause, err)
		}
		bodies[cause] = body
	}
	if bodies[causeTrial] == bodies[causeCanceled] || bodies[causeCanceled] == bodies[causeLapsed] || bodies[causeTrial] == bodies[causeLapsed] {
		t.Error("two causes share one text")
	}
}

func TestRenderStateRefusesOtherStatuses(t *testing.T) {
	for _, status := range []string{"active", "trialing", "canceled", ""} {
		if _, _, err := renderState(status, sampleData()); err == nil {
			t.Errorf("renderState(%q) succeeded", status)
		}
	}
}

// A workspace name is user input: it must not break a header or the text.
func TestWorkspaceNameCannotBreakTheSubject(t *testing.T) {
	d := sampleData()
	d.Workspace = cleanName("Acme\r\nBcc: victim@example.com\x00\u202e")
	for _, f := range []func() (string, error){
		func() (string, error) { s, _, err := renderReminder(d); return s, err },
		func() (string, error) { s, _, err := renderState(StatusPastDue, d); return s, err },
	} {
		subject, err := f()
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsFunc(subject, unicode.IsControl) || strings.ContainsRune(subject, '\u202e') {
			t.Errorf("subject %q holds a control character", subject)
		}
	}
	if got := cleanName("  A  b\tc "); got != "A b c" {
		t.Errorf("cleanName = %q", got)
	}
	if got := cleanName(strings.Repeat("x", 500)); len([]rune(got)) > 80 {
		t.Errorf("cleanName keeps %d runes", len([]rune(got)))
	}
}

// Dates in the email are Vietnam dates (UTC+7), day first.
func TestFormatDateUsesVietnamTime(t *testing.T) {
	at := time.Date(2026, 3, 31, 20, 0, 0, 0, time.UTC) // 1 April 03:00 in Vietnam
	if got := formatDate(at); got != "01/04/2026" {
		t.Errorf("formatDate = %q, want 01/04/2026", got)
	}
}

func TestEmbeddedTemplatesAreTheOnesTheCodeUses(t *testing.T) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	want := []string{"remind.txt.tmpl", "state_free.txt.tmpl", "state_past_due.txt.tmpl", "state_read_only.txt.tmpl"}
	if !slices.Equal(names, want) {
		t.Errorf("templates = %v, want %v", names, want)
	}
}

// The email the service sends carries the workspace's name and settings link.
func TestServiceEmailNamesTheWorkspaceAndLinksToSettings(t *testing.T) {
	e := newEnv(t)
	e.store.put(activeSub(wsID(1), t0))
	e.store.contact(wsID(1), admin)
	e.run(t, t0)
	m := e.mail.messages()
	if len(m) != 1 {
		t.Fatalf("%d emails, want 1", len(m))
	}
	for _, want := range []string{"Workspace 001", "https://app.example.com/ws-001/settings", "Pro"} {
		if !strings.Contains(m[0].Body, want) {
			t.Errorf("body lacks %q:\n%s", want, m[0].Body)
		}
	}
	if !strings.Contains(m[0].Subject, "Workspace 001") {
		t.Errorf("subject = %q", m[0].Subject)
	}
}
