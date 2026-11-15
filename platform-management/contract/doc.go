// Package contract holds the fixtures that show Multica's existing entitlement
// client and the platform agree on the wire.
//
// The fixtures in testdata are the platform's own responses, written by this
// package's test with the -update flag:
//
//	go test ./contract -update
//
// Without the flag the test compares the handlers' output with the files, so a
// change to the wire format fails here until the fixtures are regenerated and
// reviewed. Multica's tests (server/internal/entitlement/
// client_platform_contract_test.go) read the same files and run its client on
// them, so a change the client cannot decode fails on that side.
package contract
