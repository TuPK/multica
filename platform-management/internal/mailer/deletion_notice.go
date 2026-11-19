package mailer

import (
	"context"
	"fmt"
	"html"
	"time"
)

const deletionNoticeSubject = "A Multica workspace is scheduled for deletion"

// deletionNoticeBody is the HTML of the notice to an owner (spec R6 section 3c.9, rule 4).
// It names the workspace and the day deletion starts, and says where an export
// goes when one was requested. It holds no link.
func deletionNoticeBody(workspaceName string, at time.Time, exportFirst bool) string {
	export := ""
	if exportFirst {
		export = `<p>An export of the workspace data was requested before deletion. Staff will send a download link to the owners once it is ready.</p>`
	}
	return fmt.Sprintf(`<div style="font-family: sans-serif; max-width: 480px; margin: 0 auto;">
<h2>Workspace scheduled for deletion</h2>
<p>A platform administrator asked for the workspace <strong>%s</strong> to be deleted now. Deletion starts on <strong>%s</strong> (UTC).</p>
%s
<p style="color: #666; font-size: 14px;">If you did not expect this, contact your platform administrator.</p>
</div>`, html.EscapeString(workspaceName), at.UTC().Format("2006-01-02 15:04"), export)
}

// SendDeletionNotice tells one owner that a workspace is scheduled for deletion.
// The address must be a plain address.
func (m *Mailer) SendDeletionNotice(ctx context.Context, to, workspaceName string, at time.Time, exportFirst bool) error {
	return m.deliver(ctx, to, deletionNoticeSubject, deletionNoticeBody(workspaceName, at, exportFirst), "deletion notice")
}
