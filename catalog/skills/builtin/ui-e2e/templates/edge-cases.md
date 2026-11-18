# Edge-case checklist (apply per screen, form, dialog, list)

| Area | Cases |
| --- | --- |
| Empty input | Submit disabled or error shown; no request sent |
| Whitespace-only | Treated as empty |
| Boundary | Exactly at the limit accepted; one over refused |
| Invalid format | Missing @, too long, letters in a numeric code, too few digits |
| Wrong credential | Wrong code shows the same message as the spec; the form stays usable |
| Unknown target | Unknown and known targets get the same neutral message |
| Self-target | The signed-in user cannot change their own account; the screen says so |
| Duplicate | Adding an existing member shows the duplicate message |
| Role gating | Hidden nav items; a direct route or a stale state still shows a permitted screen |
| Dialog | Cancel changes nothing; confirm needs a reason; Escape closes it |
| Double submit | A second click while pending does not create a second change |
| Search | No match shows the empty state; clearing restores the list |
| Refresh | Reload keeps the session; a signed-out tab returns to sign-in |
| Network failure | Aborted request shows the error state, not a blank screen |
| Mobile | 375x667 with no horizontal scroll (scrollWidth <= innerWidth) |
| Keyboard | Enter submits; Tab reaches the submit button |
