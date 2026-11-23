package mailer

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
)

const welcomeSubject = "Your Delegora account is ready"

// SendWelcome tells a person that a platform administrator created an account
// for them (spec 7b.5, rule 9). It names the workspace and the plan, links to
// the web app at appURL and says how to ask for a sign-in code. It holds no
// code, password or token link: appURL is the only link and it carries no query.
// The address must be a plain address and appURL an absolute http or https URL.
func (m *Mailer) SendWelcome(ctx context.Context, to, workspaceName, planName, appURL string) error {
	u, err := url.Parse(appURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.User != nil {
		return errors.New("mailer: the web app address is not an http or https URL")
	}
	link := html.EscapeString(appURL)
	body := fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 480px; margin: 0 auto;">
<h2>Your Delegora account is ready</h2>
<p>A platform administrator created a Delegora account for you. Your workspace <strong>%s</strong> is on the <strong>%s</strong> plan.</p>
<p>To sign in, open <a href="%s">%s</a>, enter this email address and ask for a sign-in code. We send the code to this address each time you sign in.</p>
<p style="color: #666; font-size: 14px;">If you did not expect this email, you can ignore it.</p>
</div>`, html.EscapeString(workspaceName), html.EscapeString(planName), link, link)
	return m.deliver(ctx, to, welcomeSubject, body, "welcome message")
}
