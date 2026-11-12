package litscout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/ledongthuc/pdf"
)

const (
	maxPDFBytes     = 25 << 20
	maxFullTextRune = 200_000
)

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func blockedAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip)
}

// PDFFetcher downloads an open-access PDF and extracts its text. The address comes from a third-party
// API, so connections to private, loopback and metadata addresses are refused at dial time.
type PDFFetcher struct {
	// HTTP overrides the guarded client (tests only).
	HTTP *http.Client
	// Timeout bounds one download. Zero means 60 seconds.
	Timeout time.Duration
}

func (f *PDFFetcher) client() *http.Client {
	if f.HTTP != nil {
		return f.HTTP
	}
	timeout := f.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf("refusing to connect to %q", address)
		}
		if blockedAddr(ap.Addr()) {
			return fmt.Errorf("refusing to connect to %s: a private, loopback or metadata address", ap.Addr())
		}
		return nil
	}}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Proxy: nil, DialContext: dialer.DialContext},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}
}

// Fetch downloads rawURL and returns the extracted text. The body must start with %PDF.
func (f *PDFFetcher) Fetch(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", errors.New("invalid PDF address")
	}
	req.Header.Set("User-Agent", "MulticaLitScout/1.0 (+https://multica.ai)")
	resp, err := f.client().Do(req)
	if err != nil {
		return "", errors.New("download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPDFBytes+1))
	if err != nil {
		return "", errors.New("download failed")
	}
	if len(body) > maxPDFBytes {
		return "", errors.New("PDF is larger than the size limit")
	}
	return ExtractPDFText(ctx, body)
}

// ErrWordSpacingLost means the pure-Go reader returned text with no spaces between words, which
// happens with PDFs set in TeX fonts. Installing poppler-utils (pdftotext) fixes it.
var ErrWordSpacingLost = errors.New("the PDF text came out without word spacing; install poppler-utils (pdftotext) for readable text")

// ExtractPDFText returns the text of a PDF. It uses pdftotext when it is installed, because that
// reads columns and spacing far better, and otherwise a pure-Go reader whose result is checked for
// lost word spacing.
func ExtractPDFText(ctx context.Context, body []byte) (string, error) {
	if !bytes.HasPrefix(body, []byte("%PDF")) {
		return "", errors.New("not a PDF")
	}
	if path, err := exec.LookPath("pdftotext"); err == nil {
		if text, err := pdftotext(ctx, path, body); err == nil {
			return capText(text)
		}
	}
	text, err := goPDFText(body)
	if err != nil {
		return "", err
	}
	if lostSpacing(text) {
		return "", ErrWordSpacingLost
	}
	return capText(text)
}

func capText(text string) (string, error) {
	r := []rune(strings.TrimSpace(text))
	if len(r) > maxFullTextRune {
		r = r[:maxFullTextRune]
	}
	if len(r) == 0 {
		return "", errors.New("PDF has no extractable text")
	}
	return string(r), nil
}

func pdftotext(ctx context.Context, bin string, body []byte) (string, error) {
	f, err := os.CreateTemp("", "litscout-*.pdf")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(body); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "-q", "-enc", "UTF-8", f.Name(), "-")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", errors.New("pdftotext failed")
	}
	return out.String(), nil
}

// lostSpacing reports text in which almost no letter is followed by a space, as a long text
// without word breaks looks. Real prose has a space about every five to seven characters.
func lostSpacing(text string) bool {
	letters, spaces := 0, 0
	for _, r := range text {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsSpace(r):
			spaces++
		}
	}
	return letters > 200 && float64(spaces)/float64(letters) < 0.08
}

// goPDFText reads a PDF with the pure-Go parser, one line per row. The parser can panic on a
// malformed file; that is reported as an error.
func goPDFText(body []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", errors.New("PDF could not be read")
		}
	}()
	reader, err := pdf.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", errors.New("PDF could not be read")
	}
	var b strings.Builder
	for page := 1; page <= reader.NumPage(); page++ {
		rows, err := reader.Page(page).GetTextByRow()
		if err != nil {
			return "", errors.New("PDF could not be read")
		}
		for _, row := range rows {
			var line strings.Builder
			prevEnd := 0.0
			for i, run := range row.Content {
				if i > 0 && run.X-prevEnd > run.FontSize*0.15 {
					line.WriteByte(' ')
				}
				line.WriteString(run.S)
				prevEnd = run.X + run.W
			}
			if l := strings.Join(strings.Fields(line.String()), " "); l != "" {
				b.WriteString(l + "\n")
			}
		}
		if b.Len() > maxFullTextRune*4 {
			break
		}
	}
	return b.String(), nil
}
