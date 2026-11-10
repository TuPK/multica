import reactConfig from "@multica/eslint-config/react";

// Vendored skill bundles ship their own Node scripts and are not linted as product code.
export default [
  { ignores: ["squads/sdlc-skills/**/*.js", "squads/sdlc-skills/**/scripts/**", "squads/kitviet-upstream/**"] },
  ...reactConfig,
];
