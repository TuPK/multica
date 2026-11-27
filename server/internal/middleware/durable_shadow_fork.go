package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// shadowTaskCode is the code a shadow run's request that would change workspace data is
// refused with (durable spec 9.4 (d)). A shadow run reads; every change answers 403.
const shadowTaskCode = "shadow_task"

// shadowRunKind is the durable kind a shadow task's context carries.
const shadowRunKind = "shadow"

// shadowMutationRefused reports whether a request by a shadow task would change workspace data:
// any method other than GET, HEAD or OPTIONS, from a task whose context marks it a shadow run.
func shadowMutationRefused(method string, taskContext []byte) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	var marker struct {
		DurableKind string `json:"durable_kind"`
	}
	return len(taskContext) > 0 && json.Unmarshal(taskContext, &marker) == nil && marker.DurableKind == shadowRunKind
}

// shadowTaskMutationRefused answers 403 shadow_task and reports true when the task token's task
// is a shadow run making a change. A read or an unknown task is passed on.
func shadowTaskMutationRefused(w http.ResponseWriter, r *http.Request, q *db.Queries, taskID pgtype.UUID) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	task, err := q.GetAgentTask(r.Context(), taskID)
	if err != nil || !shadowMutationRefused(r.Method, task.Context) {
		return false
	}
	writeCodedError(w, http.StatusForbidden, shadowTaskCode, "a shadow run may only read; this request would change workspace data")
	return true
}
