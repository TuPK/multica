package publish

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Errors the handlers map to HTTP statuses.
var (
	ErrChannelNotFound      = errors.New("publish channel not found")
	ErrPlatformTaken        = errors.New("this workspace already has a channel on that platform")
	ErrUnknownPlatform      = errors.New("unknown platform")
	ErrUnknownMarket        = errors.New("market must be vn or intl")
	ErrAccountName          = errors.New("account_name is required and must be at most 100 characters")
	ErrTextTooLong          = errors.New("text is longer than the platform allows")
	ErrTextEmpty            = errors.New("text is required")
	ErrKeyInvalid           = errors.New("keys must be at most 512 characters with no spaces")
	ErrSecretsNotConfigured = errors.New("publish secret key is not configured on this server")
)

// ayrshareNotConfiguredDetail is the status detail a channel reports while its
// workspace has no Ayrshare key. It is the same wording the API reports.
const ayrshareNotConfiguredDetail = "Ayrshare not configured"

// maxAccountNameLen bounds the display name of a channel.
const maxAccountNameLen = 100

// maxKeyLen bounds one key. Ayrshare keys and X developer keys are far shorter.
const maxKeyLen = 512

// Service owns the publishing channels of every workspace, their Ayrshare keys
// and profiles, and the publisher that sends approved posts.
type Service struct {
	Queries *db.Queries
	// Box seals each workspace's keys at rest. Nil means no key can be stored or
	// read, and every workspace reports not configured.
	Box *secretbox.Box
	// BaseURL is the Ayrshare API root. Tests point it at a fake.
	BaseURL string
	HTTP    *http.Client
	// Backoff holds the waits before each retry of a send. Its length is the
	// number of retries, so the default of two gives at most three tries.
	Backoff []time.Duration
	// StaleAfter is how long an attempt may stay sending before a run treats its
	// send as interrupted and queues it again.
	StaleAfter time.Duration
	// PollInterval is how often the publisher looks for queued attempts.
	PollInterval time.Duration
	Now          func() time.Time
}

// NewService returns the publishing service with the production settings.
func NewService(q *db.Queries, box *secretbox.Box) *Service {
	return &Service{
		Queries:      q,
		Box:          box,
		BaseURL:      DefaultAyrshareBaseURL,
		HTTP:         &http.Client{Timeout: 20 * time.Second},
		Backoff:      []time.Duration{2 * time.Second, 5 * time.Second},
		StaleAfter:   10 * time.Minute,
		PollInterval: 10 * time.Second,
		Now:          time.Now,
	}
}

// Settings is what the settings API reports about a workspace. It says whether
// each key is set and never returns key material. XConfigured means both X keys
// are set, since Ayrshare needs both.
type Settings struct {
	AyrshareConfigured bool
	XConfigured        bool
}

// SettingsPatch is a partial write. A nil field is left unchanged; an empty
// string clears the field; any other value replaces it.
type SettingsPatch struct {
	AyrshareKey *string
	XAPIKey     *string
	XAPISecret  *string
}

// Channel is one channel as the API returns it. Status is the stored status,
// or "error" with a fixed detail when its workspace has no Ayrshare key.
type Channel struct {
	ID            string
	Platform      string
	Market        string
	AccountName   string
	Status        string
	StatusDetail  string
	LastCheckedAt *time.Time
	Limits        Limits
}

// Present turns a stored row into the API shape. configured says whether the
// row's workspace has an Ayrshare key. It writes nothing.
func (s *Service) Present(row db.PublishChannel, configured bool) Channel {
	limits, _ := LimitsFor(row.Platform)
	c := Channel{
		ID:           util.UUIDToString(row.ID),
		Platform:     row.Platform,
		Market:       row.Market,
		AccountName:  row.AccountName,
		Status:       row.Status,
		StatusDetail: row.StatusDetail,
		Limits:       limits,
	}
	if row.LastCheckedAt.Valid {
		t := row.LastCheckedAt.Time
		c.LastCheckedAt = &t
	}
	if !configured {
		c.Status = StatusError
		c.StatusDetail = ayrshareNotConfiguredDetail
	}
	return c
}

// Settings reports which keys the workspace has set. It does not decrypt them.
func (s *Service) Settings(ctx context.Context, ws pgtype.UUID) (Settings, error) {
	if s.Box == nil {
		return Settings{}, nil
	}
	row, err := s.Queries.GetPublishSettings(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("load publish settings: %w", err)
	}
	return Settings{
		AyrshareConfigured: len(row.AyrshareKeySealed) > 0,
		XConfigured:        len(row.XKeySealed) > 0 && len(row.XSecretSealed) > 0,
	}, nil
}

// SaveSettings applies a partial write to the workspace's keys. It seals each
// key it is given. Keys are checked before anything is written, so a bad key
// leaves the stored ones alone.
func (s *Service) SaveSettings(ctx context.Context, ws pgtype.UUID, patch SettingsPatch) (Settings, error) {
	if s.Box == nil {
		return Settings{}, ErrSecretsNotConfigured
	}
	params := db.UpsertPublishSettingsParams{WorkspaceID: ws}
	var err error
	if patch.AyrshareKey != nil {
		params.SetAyrshare = true
		if params.AyrshareKeySealed, err = s.sealKey(*patch.AyrshareKey); err != nil {
			return Settings{}, err
		}
	}
	if patch.XAPIKey != nil {
		params.SetXKey = true
		if params.XKeySealed, err = s.sealKey(*patch.XAPIKey); err != nil {
			return Settings{}, err
		}
	}
	if patch.XAPISecret != nil {
		params.SetXSecret = true
		if params.XSecretSealed, err = s.sealKey(*patch.XAPISecret); err != nil {
			return Settings{}, err
		}
	}
	if err := s.Queries.UpsertPublishSettings(ctx, params); err != nil {
		return Settings{}, fmt.Errorf("save publish settings: %w", err)
	}
	return s.Settings(ctx, ws)
}

// sealKey trims a key and seals it. An empty key seals to nil, which clears the
// stored key.
func (s *Service) sealKey(raw string) ([]byte, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return nil, nil
	}
	if len(key) > maxKeyLen || strings.ContainsAny(key, " \t\r\n") {
		return nil, ErrKeyInvalid
	}
	sealed, err := s.Box.Seal([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("seal key: %w", err)
	}
	return sealed, nil
}

// credentials decrypts the workspace's keys for one call. A workspace with no
// stored keys gets empty credentials, and the caller treats that as not configured.
func (s *Service) credentials(ctx context.Context, ws pgtype.UUID) (Credentials, error) {
	if s.Box == nil {
		return Credentials{}, nil
	}
	row, err := s.Queries.GetPublishSettings(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credentials{}, nil
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("load publish settings: %w", err)
	}
	var c Credentials
	if c.AyrshareKey, err = s.openKey(row.AyrshareKeySealed); err != nil {
		return Credentials{}, err
	}
	if c.XAPIKey, err = s.openKey(row.XKeySealed); err != nil {
		return Credentials{}, err
	}
	if c.XAPISecret, err = s.openKey(row.XSecretSealed); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

func (s *Service) openKey(sealed []byte) (string, error) {
	if len(sealed) == 0 {
		return "", nil
	}
	raw, err := s.Box.Open(sealed)
	if err != nil {
		return "", fmt.Errorf("open publish key: %w", err)
	}
	return string(raw), nil
}

// clientFor returns a client that calls Ayrshare with one workspace's keys.
func (s *Service) clientFor(c Credentials) *Client {
	base := s.BaseURL
	if base == "" {
		base = DefaultAyrshareBaseURL
	}
	return &Client{
		BaseURL:    base,
		APIKey:     c.AyrshareKey,
		HTTP:       s.HTTP,
		XAPIKey:    c.XAPIKey,
		XAPISecret: c.XAPISecret,
	}
}

// List returns the workspace's channels in creation order.
func (s *Service) List(ctx context.Context, ws pgtype.UUID) ([]Channel, error) {
	rows, err := s.Queries.ListPublishChannels(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("list publish channels: %w", err)
	}
	configured, err := s.Settings(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := make([]Channel, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.Present(row, configured.AyrshareConfigured))
	}
	return out, nil
}

// Create adds a channel that is not connected yet.
func (s *Service) Create(ctx context.Context, ws pgtype.UUID, platform, market, accountName string) (Channel, error) {
	if !ValidPlatform(platform) {
		return Channel{}, ErrUnknownPlatform
	}
	if !ValidMarket(market) {
		return Channel{}, ErrUnknownMarket
	}
	accountName = strings.TrimSpace(accountName)
	if accountName == "" || len([]rune(accountName)) > maxAccountNameLen {
		return Channel{}, ErrAccountName
	}
	row, err := s.Queries.InsertPublishChannel(ctx, db.InsertPublishChannelParams{
		WorkspaceID: ws, Platform: platform, Market: market, AccountName: accountName,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Channel{}, ErrPlatformTaken
		}
		return Channel{}, fmt.Errorf("insert publish channel: %w", err)
	}
	configured, err := s.Settings(ctx, ws)
	if err != nil {
		return Channel{}, err
	}
	return s.Present(row, configured.AyrshareConfigured), nil
}

// Delete removes a channel from the workspace. The social account stays linked on
// Ayrshare: Ayrshare's only documented removal deletes the whole profile, which
// would drop every other channel of the workspace too.
func (s *Service) Delete(ctx context.Context, ws, id pgtype.UUID) error {
	n, err := s.Queries.DeletePublishChannel(ctx, db.DeletePublishChannelParams{ID: id, WorkspaceID: ws})
	if err != nil {
		return fmt.Errorf("delete publish channel: %w", err)
	}
	if n == 0 {
		return ErrChannelNotFound
	}
	return nil
}

// Connect creates the workspace's Ayrshare profile if it has none, and returns
// a link the user opens to connect the channel's account.
func (s *Service) Connect(ctx context.Context, ws, id pgtype.UUID) (string, error) {
	if _, err := s.channel(ctx, ws, id); err != nil {
		return "", err
	}
	creds, err := s.credentials(ctx, ws)
	if err != nil {
		return "", err
	}
	if creds.AyrshareKey == "" {
		return "", ErrNotConfigured
	}
	client := s.clientFor(creds)
	key, err := s.ensureProfileKey(ctx, ws, client)
	if err != nil {
		return "", err
	}
	return client.LinkSession(ctx, key)
}

// Check asks Ayrshare whether the channel's account is linked and stores the
// result. A channel with no profile, or whose network is not linked, is not
// connected. A rejected key or an unreachable Ayrshare is an error.
func (s *Service) Check(ctx context.Context, ws, id pgtype.UUID) (Channel, error) {
	row, err := s.channel(ctx, ws, id)
	if err != nil {
		return Channel{}, err
	}
	creds, err := s.credentials(ctx, ws)
	if err != nil {
		return Channel{}, err
	}
	status, detail := StatusNotConnected, ""
	if creds.AyrshareKey == "" {
		status, detail = StatusError, ayrshareNotConfiguredDetail
	} else {
		linked, err := s.linked(ctx, ws, creds, row.Platform)
		if err != nil {
			status, detail = StatusError, checkErrorDetail(err)
		} else if linked {
			status = StatusReady
		}
	}
	updated, err := s.Queries.UpdatePublishChannelStatus(ctx, db.UpdatePublishChannelStatusParams{
		ID: id, WorkspaceID: ws, Status: status, StatusDetail: detail,
		LastCheckedAt: pgtype.Timestamptz{Time: s.Now(), Valid: true},
	})
	if err != nil {
		return Channel{}, fmt.Errorf("update publish channel status: %w", err)
	}
	return s.Present(updated, creds.AyrshareKey != ""), nil
}

// DryRun returns the exact Ayrshare request body a post on this channel would
// send. It makes no network call and needs no key.
func (s *Service) DryRun(ctx context.Context, ws, id pgtype.UUID, text string, mediaURL string, hashtags []string) (map[string]any, error) {
	row, err := s.channel(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, ErrTextEmpty
	}
	limits, _ := LimitsFor(row.Platform)
	if n := len([]rune(ComposeText(text, hashtags))); n > limits.MaxChars {
		return nil, ErrTextTooLong
	}
	return PostRequest{Text: text, Hashtags: hashtags, Platform: row.Platform, MediaURL: mediaURL}.Body()
}

// channel loads a channel of the workspace or returns ErrChannelNotFound.
func (s *Service) channel(ctx context.Context, ws, id pgtype.UUID) (db.PublishChannel, error) {
	row, err := s.Queries.GetPublishChannelInWorkspace(ctx, db.GetPublishChannelInWorkspaceParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PublishChannel{}, ErrChannelNotFound
	}
	if err != nil {
		return db.PublishChannel{}, fmt.Errorf("load publish channel: %w", err)
	}
	return row, nil
}

// linked reports whether the workspace's profile has the platform linked. A
// workspace with no profile has nothing linked.
func (s *Service) linked(ctx context.Context, ws pgtype.UUID, creds Credentials, platform string) (bool, error) {
	key, err := s.profileKey(ctx, ws)
	if errors.Is(err, errNoProfile) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	accounts, err := s.clientFor(creds).ActiveAccounts(ctx, key)
	if err != nil {
		return false, err
	}
	name, err := ayrshareName(platform)
	if err != nil {
		return false, err
	}
	for _, a := range accounts {
		if a == name {
			return true, nil
		}
	}
	return false, nil
}

// checkErrorDetail is the status detail for a failed check. It never includes
// the key or Ayrshare's raw answer.
func checkErrorDetail(err error) string {
	var re *RequestError
	if errors.As(err, &re) && (re.Status == 401 || re.Status == 403) {
		return "Ayrshare rejected the API key"
	}
	if errors.Is(err, ErrNotConfigured) {
		return ayrshareNotConfiguredDetail
	}
	return "Ayrshare could not be reached; check again"
}

var errNoProfile = errors.New("no Ayrshare profile for this workspace")

// profileKey returns the workspace's Profile-Key, or errNoProfile.
func (s *Service) profileKey(ctx context.Context, ws pgtype.UUID) (string, error) {
	if s.Box == nil {
		return "", ErrSecretsNotConfigured
	}
	row, err := s.Queries.GetPublishProfile(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errNoProfile
	}
	if err != nil {
		return "", fmt.Errorf("load publish profile: %w", err)
	}
	key, err := s.Box.Open(row.ProfileKeySealed)
	if err != nil {
		return "", fmt.Errorf("open publish profile key: %w", err)
	}
	return string(key), nil
}

// ensureProfileKey returns the workspace's Profile-Key, creating and storing its
// Ayrshare profile first when there is none.
func (s *Service) ensureProfileKey(ctx context.Context, ws pgtype.UUID, client *Client) (string, error) {
	key, err := s.profileKey(ctx, ws)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, errNoProfile) {
		return "", err
	}
	// The profile title must be unique on the Ayrshare account; the workspace id
	// is unique and stable.
	title := "multica-" + util.UUIDToString(ws)
	created, refID, err := client.CreateProfile(ctx, title)
	if err != nil {
		return "", fmt.Errorf("create Ayrshare profile: %w", err)
	}
	sealed, err := s.Box.Seal([]byte(created))
	if err != nil {
		return "", fmt.Errorf("seal publish profile key: %w", err)
	}
	if _, err := s.Queries.InsertPublishProfileIfAbsent(ctx, db.InsertPublishProfileIfAbsentParams{
		WorkspaceID: ws, ProfileKeySealed: sealed, RefID: refID,
	}); err != nil {
		return "", fmt.Errorf("store publish profile: %w", err)
	}
	return created, nil
}

// Run polls for queued attempts until ctx ends. Each pass drains the queue up to
// a fixed batch, so one busy workspace cannot starve the others between ticks.
func (s *Service) Run(ctx context.Context) {
	interval := s.PollInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.drain(ctx)
		}
	}
}

const drainBatch = 20

func (s *Service) drain(ctx context.Context) {
	for i := 0; i < drainBatch && ctx.Err() == nil; i++ {
		processed, err := s.ProcessNext(ctx)
		if err != nil {
			slog.Warn("publish: process attempt failed", "error", err)
			return
		}
		if !processed {
			return
		}
	}
}
