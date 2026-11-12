// Package research stores the Research Lab's source settings of a workspace: the OpenAlex and
// Semantic Scholar API keys, sealed at rest, and the contact address for Crossref and Unpaywall.
package research

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util/secretbox"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// SecretKeyEnv names the base64 32-byte key that seals the stored API keys.
const SecretKeyEnv = "MULTICA_RESEARCH_SECRET_KEY"

// ErrNotConfigured means no sealing key is available, so an API key can be neither stored nor read.
var ErrNotConfigured = errors.New("the server has no key to seal research API keys: set " + SecretKeyEnv)

// ValidationError is a rejected input; its text is safe to show to the caller.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Service reads and writes research_settings.
type Service struct {
	Queries *db.Queries
	// Box seals the API keys. Nil means keys cannot be stored or returned.
	Box *secretbox.Box
}

// NewBox builds the sealing box. MULTICA_RESEARCH_SECRET_KEY wins. Without it a key is derived from
// JWT_SECRET, which every deployment has, so the page works without extra setup; rotating
// JWT_SECRET then makes the stored keys unreadable and they must be entered again. fromJWT
// reports which was used. The box is nil only when neither secret exists.
func NewBox() (box *secretbox.Box, fromJWT bool, err error) {
	if raw := os.Getenv(SecretKeyEnv); raw != "" {
		key, err := secretbox.LoadKey(SecretKeyEnv)
		if err != nil {
			return nil, false, err
		}
		b, err := secretbox.New(key)
		return b, false, err
	}
	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		return nil, false, nil
	}
	sum := sha256.Sum256([]byte("multica-research-secretbox-v1\x00" + jwt))
	b, err := secretbox.New(sum[:])
	return b, true, err
}

// Settings is what a client may see. It never holds a key.
type Settings struct {
	OpenAlexConfigured        bool       `json:"openalex_configured"`
	SemanticScholarConfigured bool       `json:"semantic_scholar_configured"`
	ContactEmail              string     `json:"contact_email"`
	KeysCanBeStored           bool       `json:"keys_can_be_stored"`
	UpdatedAt                 *time.Time `json:"updated_at"`
}

// Credentials are the plaintext keys, for `multica research` only.
type Credentials struct {
	OpenAlexKey  string `json:"openalex_api_key"`
	S2Key        string `json:"semantic_scholar_api_key"`
	ContactEmail string `json:"contact_email"`
}

// Update changes settings. A nil field is left alone; an empty string clears the value.
type Update struct {
	OpenAlexKey  *string
	S2Key        *string
	ContactEmail *string
}

func (s *Service) settingsOf(row db.ResearchSetting, found bool) Settings {
	out := Settings{KeysCanBeStored: s.Box != nil}
	if !found {
		return out
	}
	out.OpenAlexConfigured = len(row.OpenalexKeySealed) > 0
	out.SemanticScholarConfigured = len(row.S2KeySealed) > 0
	out.ContactEmail = row.ContactEmail
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time.UTC()
		out.UpdatedAt = &t
	}
	return out
}

func (s *Service) load(ctx context.Context, ws pgtype.UUID) (db.ResearchSetting, bool, error) {
	row, err := s.Queries.GetResearchSettings(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ResearchSetting{}, false, nil
	}
	return row, err == nil, err
}

// Get returns the workspace's settings without any key.
func (s *Service) Get(ctx context.Context, ws pgtype.UUID) (Settings, error) {
	row, found, err := s.load(ctx, ws)
	if err != nil {
		return Settings{}, err
	}
	return s.settingsOf(row, found), nil
}

func validKey(name, v string) error {
	if len(v) > 256 {
		return &ValidationError{name + " is too long"}
	}
	for _, r := range v {
		if r <= ' ' || r > '~' {
			return &ValidationError{name + " must be one token of printable characters without spaces"}
		}
	}
	return nil
}

func validEmail(v string) error {
	if v == "" {
		return nil
	}
	if len(v) > 254 {
		return &ValidationError{"contact email is too long"}
	}
	a, err := mail.ParseAddress(v)
	if err != nil || a.Address != v || !strings.Contains(v, "@") {
		return &ValidationError{"contact email is not a valid address"}
	}
	return nil
}

// Update applies u and returns the new settings.
func (s *Service) Update(ctx context.Context, ws pgtype.UUID, u Update) (Settings, error) {
	oa, s2, mailTo := "", "", ""
	if u.OpenAlexKey != nil {
		oa = strings.TrimSpace(*u.OpenAlexKey)
		if err := validKey("OpenAlex API key", oa); err != nil {
			return Settings{}, err
		}
	}
	if u.S2Key != nil {
		s2 = strings.TrimSpace(*u.S2Key)
		if err := validKey("Semantic Scholar API key", s2); err != nil {
			return Settings{}, err
		}
	}
	if u.ContactEmail != nil {
		mailTo = strings.TrimSpace(*u.ContactEmail)
		if err := validEmail(mailTo); err != nil {
			return Settings{}, err
		}
	}
	if (oa != "" || s2 != "") && s.Box == nil {
		return Settings{}, ErrNotConfigured
	}
	row, _, err := s.load(ctx, ws)
	if err != nil {
		return Settings{}, err
	}
	seal := func(v string) ([]byte, error) { return s.Box.Seal([]byte(v)) }
	arg := db.UpsertResearchSettingsParams{
		WorkspaceID: ws, OpenalexKeySealed: row.OpenalexKeySealed, S2KeySealed: row.S2KeySealed, ContactEmail: row.ContactEmail,
	}
	if u.OpenAlexKey != nil {
		arg.OpenalexKeySealed = nil
		if oa != "" {
			if arg.OpenalexKeySealed, err = seal(oa); err != nil {
				return Settings{}, fmt.Errorf("seal key: %w", err)
			}
		}
	}
	if u.S2Key != nil {
		arg.S2KeySealed = nil
		if s2 != "" {
			if arg.S2KeySealed, err = seal(s2); err != nil {
				return Settings{}, fmt.Errorf("seal key: %w", err)
			}
		}
	}
	if u.ContactEmail != nil {
		arg.ContactEmail = mailTo
	}
	saved, err := s.Queries.UpsertResearchSettings(ctx, arg)
	if err != nil {
		return Settings{}, err
	}
	return s.settingsOf(saved, true), nil
}

// Credentials returns the plaintext keys. A key that cannot be opened (the sealing secret changed)
// is returned as empty, so a run falls back to the unauthenticated tier instead of failing.
func (s *Service) Credentials(ctx context.Context, ws pgtype.UUID) (Credentials, error) {
	row, found, err := s.load(ctx, ws)
	if err != nil || !found {
		return Credentials{}, err
	}
	out := Credentials{ContactEmail: row.ContactEmail}
	open := func(sealed []byte) string {
		if len(sealed) == 0 || s.Box == nil {
			return ""
		}
		b, err := s.Box.Open(sealed)
		if err != nil {
			return ""
		}
		return string(b)
	}
	out.OpenAlexKey, out.S2Key = open(row.OpenalexKeySealed), open(row.S2KeySealed)
	return out, nil
}
