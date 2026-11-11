# Profile: web (SPA or server-rendered app)

Add these to the common template when the repository serves a browser UI.

- **Where it runs.** State the URL the app is reached at and the context that changes behaviour. An app opened over plain
  HTTP on a LAN address is not a secure context: `crypto.subtle` and `crypto.randomUUID` are missing there. Write the
  URL and that limit in "Running and verifying".
- **Evidence in a browser.** Done includes a browser check at a desktop and a phone width (for example 1440 and 390 px).
  Typecheck and unit tests with a mocked backend are L3. Say so in the Done line.
- **Environment variables.** Public (shipped to the browser) and private variables are separate files or prefixes.
  `.env.example` lists names only. Never place a service key behind a public prefix.
- **Boundaries.** Table rows for pages or routes, shared components, the API client, and generated code. Name the lint
  rule or test that enforces each, or write `prose`.
- **Accessibility.** If the repository has an accessibility check, name its command in "Running and verifying".
- **Pitfalls to look for.** A saved session that the server no longer accepts; a dev server that is not running; a
  stale build served instead of the dev server.
