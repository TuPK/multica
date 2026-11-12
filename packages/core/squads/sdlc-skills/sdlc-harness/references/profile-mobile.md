# Profile: mobile (Expo or React Native)

What to probe. A probe becomes a line in `AGENTS.md` only if you trip on it, with the evidence.

- **Commands.** The simulator or device command, the Metro or dev-client command, and the build command: put in Verify only what a Developer runs
  every time; the rest stays in the repository's own docs.
- **Evidence.** Typecheck, lint and unit tests are L3. A screen is verified only when someone saw it run on a simulator or device. Typecheck alone never
  counts as a UI check; say which command is the Done command and its level.
- **Native versus JS changes.** Which changes need a new native build and which reload? If an agent would get this wrong, it is a Pitfall.
- **Environment.** Public config per target (development, staging, production) in separate files; names only in the example file.
- **Look for.** A cached bundler serving old code; a simulator not booted; a native dependency added without a rebuild.
