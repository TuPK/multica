# Profile: web (SPA or server-rendered app)

What to probe when the repository serves a browser UI. A probe becomes a line in `AGENTS.md` only if you actually trip on it, and then it
carries the evidence (command and exit code). Do not paste this list into the file.

- **Where it runs.** Open the app at the URL the Ensure command serves. An app opened over plain HTTP on a LAN address is not a secure context:
  `crypto.subtle` and `crypto.randomUUID` are missing there. If sign-in or any crypto call fails, that is a Pitfall.
- **Evidence in a browser.** Done should include a browser check at a desktop and a phone width (for example 1440 and 390 px).
  Typecheck and unit tests with a mocked backend are L3. Say so in the Done line.
- **Environment variables.** Are public (shipped to the browser) and private variables in separate files or prefixes? `.env.example` lists names only.
  A service key behind a public prefix is a Rule, enforced by a check you add to the Done command if the repository has none.
- **Accessibility.** If the repository has an accessibility check, it belongs in the Done command or the Quick check.
- **Look for.** A saved session the server no longer accepts; a dev server that is not running; a stale build served instead of the dev server;
  a test suite slow enough that the Quick check must skip it.
