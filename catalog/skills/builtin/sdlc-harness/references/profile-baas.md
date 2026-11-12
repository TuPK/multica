# Profile: backend as a service (Supabase or similar)

What to probe when the data layer, auth and server functions are hosted or run by a local stack. A probe becomes a line in `AGENTS.md` only if you trip on it.

- **The ensure command must serve everything the app calls.** For Supabase that means the local stack is up and every Edge Function the app uses is
  served. A function that is not served shows up as a generic 5xx in the UI, which looks like an application bug. Put the serve step inside the ensure
  command and name it in `docs/sdlc/environment.md`.
- **Two environments.** Hosted and local variables live in separate files, and no script reads a hosted URL when it means the local one.
- **Policies.** Is row-level security tested by database tests? If so name the command in Verify. The service-role key in frontend code is a Rule; enforce
  it with a check in the Done command if you can.
- **Auth.** How is a test identity created locally, and how is a stale saved session cleared? If you tripped, that is a Pitfall.
- **Evidence.** The local stack with migrations and seed data is L2. Hosted is L1. Mocked-client tests are L3. The Done line says which it covers.
