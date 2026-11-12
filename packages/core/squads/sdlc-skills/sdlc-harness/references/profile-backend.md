# Profile: backend (Go, Node or other API service)

What to probe. A probe becomes a line in `AGENTS.md` only if you trip on it, with the evidence.

- **Migrations.** Is there a rule that applied files are immutable and forward-only? State it as a Rule with what enforces it (a test or CI job), and name
  the command that adds one.
- **Test database.** How do tests get an isolated database? Which command needs a running one? A test that needs the database is L2 at best; the Done line
  says which level it covers.
- **API compatibility.** If clients are released independently, what may a response not change? Add a Rule only if a test or a schema check enforces it.
- **Generated code.** Name the generator command; hand edits to generated files are a Rule.
- **Secrets.** Names only, in `.env.example`.
- **Look for.** A service started in the background that dies with the shell; a port already in use; tests that pass only against a seeded database;
  a full test run that is too slow for the Quick check.
