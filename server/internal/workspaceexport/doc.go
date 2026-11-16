// Package workspaceexport builds the data archive of a workspace (platform
// spec 5.11): NDJSON files in one zip, read through an allowlist of columns so
// that secrets are left out by construction. It also holds the rules of the
// export queue (one at a time, once a day, a 7-day link) and the job that runs
// it. The package reads workspace data and never writes platform_workspace.
package workspaceexport
