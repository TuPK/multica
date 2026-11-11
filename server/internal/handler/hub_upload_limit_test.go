package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// hubUploadRequest wraps raw bytes as the "file" field of a multipart preview upload.
func hubUploadRequest(t *testing.T, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "bundle.zip")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("close form: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hub/import/preview", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	req.Header.Set("X-User-ID", testUserID)
	return req
}

// An upload over the 16 MiB archive cap is refused with 413, whether the cap is
// hit by the request body reader or by the content read after the multipart parse.
func TestHubImportPreviewUploadOverTheArchiveCapIs413(t *testing.T) {
	setupHubImportTest(t)
	archiveCap := hub.DefaultLimits.MaxArchiveBytes
	if archiveCap != 16<<20 {
		t.Fatalf("archive cap = %d, the upload test assumes 16 MiB", archiveCap)
	}

	// Just over the cap: the body is inside the body limit, so the content check refuses it.
	justOver := bytes.Repeat([]byte("a"), int(archiveCap)+1)
	testutil.Call(t, testHandler.PreviewHubImport, hubUploadRequest(t, justOver)).
		Want(http.StatusRequestEntityTooLarge)

	// Well over the cap: the request body reader refuses it before the parser spools it.
	wayOver := bytes.Repeat([]byte("a"), int(archiveCap)+(256<<10))
	testutil.Call(t, testHandler.PreviewHubImport, hubUploadRequest(t, wayOver)).
		Want(http.StatusRequestEntityTooLarge)
}
