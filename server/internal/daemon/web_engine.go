package daemon

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/multica-ai/multica/server/internal/salescollect"
)

// The daemon checks the Web engine (spec 4.13) on its own machine and reports the result for each of its
// runtimes. A check runs every webEngineCheck; a runtime is sent again when the result changes and at
// least every webEngineRefresh. The refresh matters because a registration overwrites a runtime's
// metadata, which is where the server keeps the status.
const (
	webEngineCheck       = 60 * time.Second
	webEngineRefresh     = 5 * time.Minute
	webEngineSendTimeout = 10 * time.Second
)

// webEngineSent is the last status the server accepted for one runtime.
type webEngineSent struct {
	status  string
	version string
	at      time.Time
}

// probeWebEngine reads the engine address and token from the daemon's environment. Without a token the
// engine is not configured; an address that is not loopback is a configuration error, reported the same way.
func probeWebEngine(ctx context.Context) (status, version string) {
	token := os.Getenv("CRAWL4AI_API_TOKEN")
	if token == "" {
		return "not_configured", ""
	}
	base := os.Getenv("CRAWL4AI_URL")
	if base == "" {
		base = salescollect.DefaultEngineURL
	}
	engine, err := salescollect.NewCrawl4AI(base, token)
	if err != nil {
		slog.Warn("web engine address rejected", "error", err)
		return "not_configured", ""
	}
	st := engine.Status(ctx)
	return st.Status, st.Version
}

// runWebEngineReporter reports the engine's status for the daemon's runtimes until ctx ends.
func (d *Daemon) runWebEngineReporter(ctx context.Context) {
	sent := map[string]webEngineSent{}
	ticker := time.NewTicker(webEngineCheck)
	defer ticker.Stop()
	for {
		d.reportWebEngine(ctx, sent)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// reportWebEngine checks the engine once and sends the result to each runtime that needs it. A failed
// send is retried on the next check; the runtime is not marked as sent.
func (d *Daemon) reportWebEngine(ctx context.Context, sent map[string]webEngineSent) {
	ids := d.allRuntimeIDs()
	if len(ids) == 0 {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, webEngineSendTimeout)
	status, version := probeWebEngine(probeCtx)
	cancel()

	now := time.Now()
	present := make(map[string]bool, len(ids))
	for _, id := range ids {
		present[id] = true
		last, seen := sent[id]
		if seen && last.status == status && last.version == version && now.Sub(last.at) < webEngineRefresh {
			continue
		}
		sendCtx, cancelSend := context.WithTimeout(ctx, webEngineSendTimeout)
		err := d.client.ReportWebEngineStatus(sendCtx, id, status, version)
		cancelSend()
		if err != nil {
			slog.Debug("web engine status not sent", "runtime_id", id, "error", err)
			continue
		}
		sent[id] = webEngineSent{status: status, version: version, at: now}
	}
	for id := range sent {
		if !present[id] {
			delete(sent, id)
		}
	}
}
