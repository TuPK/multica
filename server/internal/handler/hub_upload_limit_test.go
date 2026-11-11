package handler

import (
	"bytes"
	"crypto/rand"
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

// HUB_MAX_UPLOAD_BYTES replaces the archive cap of the upload preview: a zip over
// the configured cap is refused and one under it is accepted, whatever the default is.
func TestHubImportPreviewUploadCapFollowsHubMaxUploadBytes(t *testing.T) {
	setupHubImportTest(t)
	prev := testHandler.HubMaxUploadBytes
	t.Cleanup(func() { testHandler.HubMaxUploadBytes = prev })

	const cap = 64 << 10
	testHandler.HubMaxUploadBytes = cap

	// Incompressible content, so the archive is about as large as the bytes given.
	noise := make([]byte, cap+(16<<10))
	if _, err := rand.Read(noise); err != nil {
		t.Fatalf("rand: %v", err)
	}
	testutil.Call(t, testHandler.PreviewHubImport, hubUploadRequest(t, noise)).
		Want(http.StatusRequestEntityTooLarge)

	small := hubImportZip(t, map[string]string{
		"SKILL.md": "---\nname: small-cap\ndescription: Fits under the raised cap.\n---\n\nBody.\n",
	})
	if len(small) >= cap {
		t.Fatalf("test archive is %d bytes, expected under %d", len(small), cap)
	}
	testutil.Call(t, testHandler.PreviewHubImport, hubUploadRequest(t, small)).Want(http.StatusOK)
}

// A cap raised above the default admits an archive the default would refuse.
func TestHubImportPreviewUploadCapCanBeRaisedAboveTheDefault(t *testing.T) {
	setupHubImportTest(t)
	prev := testHandler.HubMaxUploadBytes
	t.Cleanup(func() { testHandler.HubMaxUploadBytes = prev })

	raised := hub.DefaultLimits.MaxArchiveBytes + (1 << 20)
	testHandler.HubMaxUploadBytes = raised

	// Over the default cap, so refused by the default; under the raised cap, so the
	// body is read and the content is judged as an archive (not as too large).
	big := bytes.Repeat([]byte("z"), int(hub.DefaultLimits.MaxArchiveBytes)+1)
	rec := testutil.Call(t, testHandler.PreviewHubImport, hubUploadRequest(t, big))
	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("an archive under the raised cap was refused as too large")
	}
}
