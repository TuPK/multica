// Package planlimit holds the quota rules that Multica applies at the moment a
// resource is created (platform-management spec 5.7): the resource gates
// (agents, runtimes, storage_bytes), the cap on free workspaces one user owns,
// and the list of blocked sign-up domains.
//
// The gates come from the entitlement policy, so a plan that does not set one
// leaves it off, and an install with no MULTICA_CLOUD_URL runs none of this
// code's queries. The owner cap and the domain list are account-level rules
// read from the environment; they are not plan gates.
package planlimit
