# Profile: backend (Go, Node or other API service)

- **Migrations.** State the rules in Policy: forward only, applied files are immutable, each is idempotent where the
  engine allows it, and how to add one (the command).
- **Test database.** Say how tests get an isolated database and which command needs a running one. A test that needs the
  database is L2 at best; say which of the commands in "Running and verifying" is L2 and which is L3.
- **API compatibility.** If clients are released independently, say what a response may and may not change.
- **Boundaries.** Table rows for handlers or routes, services, data access, generated code (never hand-edit; name the
  generator command), and migrations.
- **Secrets.** Names only, in `.env.example`. Say where a real value comes from.
- **Pitfalls to look for.** A service started in the background that dies with the shell; a port already in use; tests
  that pass only against a seeded database.
