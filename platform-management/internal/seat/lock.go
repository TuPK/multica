package seat

import (
	"crypto/sha256"
	"encoding/binary"
)

// LockNamespace is the first key of the platform's seat lock. The lock is
// taken as pg_advisory_xact_lock(LockNamespace, key), the two-integer form.
//
// Delegora's WorkspaceLocker (server/internal/seatcapacity/workspace_locker.go)
// takes pg_try_advisory_lock(bigint), the one-integer form, on a pinned
// connection and holds it while it calls this service. PostgreSQL keeps the two
// forms in separate key spaces, so a platform lock never waits on a handler
// lock for the same workspace (Appendix B rule 6). The namespace and the hash
// prefix also differ from Delegora's, so the two keys differ even if one form
// were ever changed to the other.
const LockNamespace int32 = 0x50534551 // "PSEQ"

const lockKeyPrefix = "platform-seat-capacity:"

// LockKey returns the two keys of the seat lock for a workspace ID given in its
// canonical text form.
func LockKey(workspaceID string) (namespace, key int32) {
	sum := sha256.Sum256([]byte(lockKeyPrefix + workspaceID))
	return LockNamespace, int32(binary.BigEndian.Uint32(sum[:4]))
}
