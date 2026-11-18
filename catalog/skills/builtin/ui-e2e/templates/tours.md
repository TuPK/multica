# Exploratory tours (run after the scripted tests, record each finding)

For each tour: the steps, what you expected, what you saw, and the evidence (screenshot, measured value, or the request and response). A finding becomes a `test.fixme` with its evidence, or a new flow.

1. First-time user: open the product with no data. Are the empty states useful? Is the next step clear?
2. Returning user: reload each main screen; press Back; open an inner page from a bookmark; sign out in one tab and act in another.
3. Two users or two tabs: change the same record in both; does the stale view refresh or mislead?
4. Errors: block the network for one request on each screen; slow one response by several seconds; return an error body with status 200.
5. Money and access: every action that charges, refunds, grants or revokes. Check each refusal and its message.
6. Boundaries: the longest allowed text, one character over, emoji, right-to-left text, pasted text with new lines, the largest number, a date at midnight.
7. Narrow and keyboard: 375 pixels wide, no horizontal scroll; Tab order reaches every control; Enter submits; Escape closes a dialog.
8. Copy and labels: is every button's text what it does? Does each error say what to do next?
9. Consistency: the same value shown on two screens; totals that match their rows; dates in one format.
