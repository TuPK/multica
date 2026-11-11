# Profile: backend as a service (Supabase or similar)

Applies when the data layer, auth and server functions are hosted or run by a local stack.

- **The ensure command must serve everything the app calls.** For Supabase that means the local stack is up and every
  Edge Function the app uses is served. A function that is not served shows up as a generic 5xx in the UI, which looks
  like an application bug. Put the serve step inside the ensure command and name it in `docs/sdlc/environment.md`.
- **Two environments.** Hosted and local variables live in separate files, and no script reads a hosted URL when it means
  the local one. Write which file each command reads.
- **Policies.** Row-level security is tested (database tests). Say the command. The service-role key is never used in
  frontend code; add that as a Policy line with the alternative.
- **Auth.** Say how a test identity is created locally and how a stale saved session is cleared.
- **Evidence.** The local stack with migrations and seed data is L2. Hosted is L1. Mocked-client tests are L3. The Done
  line says which it covers.
- **Boundaries.** Table rows for the client app, functions, migrations and seed data, and generated types.
