package accountdeletion

import "time"

// Wait is how long a deletion request waits before the job runs. It is not
// configurable.
const Wait = 14 * 24 * time.Hour

// RecentLoginWindow is how fresh a login must be to request deletion.
const RecentLoginWindow = 10 * time.Minute

// clockSkew is how far ahead of the server a token's login time may be.
const clockSkew = time.Minute

// BatchSize is the most accounts one job run processes.
const BatchSize = 50

// StatusPending is the platform_account status of an account waiting to be deleted.
const StatusPending = "pending_deletion"

// DeletedName is the display name an anonymised user keeps.
const DeletedName = "Deleted user"

// ScheduledAt is when a request made at now is carried out.
func ScheduledAt(now time.Time) time.Time { return now.Add(Wait) }

// DeletedEmail is the address an anonymised user row gets. It cannot receive
// mail and is unique per user.
func DeletedEmail(userID string) string { return "deleted+" + userID + "@deleted.invalid" }

// RecentLogin reports whether a session token's login time (Unix seconds) is
// within RecentLoginWindow of now. present is false for a token that carries
// no login time, such as a personal access token; it is never recent.
func RecentLogin(authTime int64, present bool, now time.Time) bool {
	if !present {
		return false
	}
	age := now.Sub(time.Unix(authTime, 0))
	return age >= -clockSkew && age <= RecentLoginWindow
}

// Due reports whether an account is pending deletion and its date has come.
// The job's SQL selection and its in-transaction re-check both follow it.
func Due(status string, scheduledAt *time.Time, now time.Time) bool {
	return status == StatusPending && scheduledAt != nil && !scheduledAt.After(now)
}

// Membership is one workspace the user belongs to, with how many members it has.
type Membership struct {
	WorkspaceID string
	Name        string
	Role        string
	MemberCount int
}

// Classification splits a user's workspaces by what deletion does with them.
type Classification struct {
	// Sole: the user is the only member; the job deletes the workspace.
	Sole []Membership
	// Blocking: the user owns a workspace that has other members; the request
	// is refused until ownership moves or the workspace is deleted.
	Blocking []Membership
}

// Classify applies the sole-member and sole-owner rules.
func Classify(ms []Membership) Classification {
	out := Classification{Sole: []Membership{}, Blocking: []Membership{}}
	for _, m := range ms {
		switch {
		case m.MemberCount <= 1:
			out.Sole = append(out.Sole, m)
		case m.Role == "owner":
			out.Blocking = append(out.Blocking, m)
		}
	}
	return out
}
