# Profile: mobile (Expo or React Native)

- **Commands.** Name the simulator or device command, the Metro or dev-client command, and the build command.
- **Evidence.** Typecheck, lint and unit tests are L3. A screen is verified only when someone saw it run on a simulator or
  device. Write in Policy that typecheck alone never counts as a UI check, and say which command is the Done command.
- **Native versus JS changes.** Say which changes need a new native build and which reload.
- **Environment.** Public config per target (development, staging, production) in separate files; names only in the example file.
- **Boundaries.** Table rows for screens, shared logic that other apps import, native modules, and generated assets.
- **Pitfalls to look for.** A cached bundler serving old code; a simulator not booted; a native dependency added without
  a rebuild.
