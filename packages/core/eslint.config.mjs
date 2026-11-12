import reactConfig from "@multica/eslint-config/react";

// Template content (vendored skills with their own Node scripts, the Kit Viet upstream) lives in
// catalog/ at the repo root, outside this package, so it is never linted as product code.
export default [...reactConfig];
