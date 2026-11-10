# Web engine (Crawl4AI): optional container for the Sales Loop squad

The Sales Loop squad reads public web pages that need a browser to render (access ladder rung 5,
[sales-loop-squad.md](../sales-loop-squad.md), section 4.13). The engine is optional: without it,
such a source stays `needs_connector` with the sentence "Needs the Web engine", and nothing runs
half-way. This runbook starts the engine on the machine that runs the daemon.

Status: the REST shape below was checked against image `unclecode/crawl4ai:0.9.5`. `GET /health` answers
`{"status":"ok","version":"0.9.5"}` and needs no token. `POST /crawl` with `{"urls":[url]}` answers
`{"success":true,"results":[{"html":...,"status_code":200,"success":true,"error_message":""}]}`; the raw `html`
keeps attributes such as a link's `href`. `POST /html` strips attributes, so the squad does not use it. A
refused request answers `{"detail":"..."}`. The engine also refuses loopback and private URLs itself
(`URL blocked (SSRF protection)`), which is a second check on top of the collector's.

## Pinned version

- Image `unclecode/crawl4ai:0.9.5`. Do not use `latest`; the squad's recipes name the version they were
  checked with.
- Token required from 0.9.0. Generate a random token and keep it out of the repository and out of recipes.

## Start

```bash
export CRAWL4AI_API_TOKEN="$(openssl rand -hex 32)"
docker run -d --name multica-crawl4ai \
  --shm-size=1g \
  -p 127.0.0.1:11235:11235 \
  -e CRAWL4AI_API_TOKEN="$CRAWL4AI_API_TOKEN" \
  unclecode/crawl4ai:0.9.5
```

Bind the port to `127.0.0.1` only. The engine must not listen on a network interface: a squad reads
URLs that people add, and an open engine could be used to read internal services.

## Check

```bash
curl -s -H "Authorization: Bearer $CRAWL4AI_API_TOKEN" -H "Content-Type: application/json" \
  -d '{"urls":["https://example.com"]}' http://127.0.0.1:11235/crawl | head -c 300
```

If the port mapping fails with `timed out starting the userland proxy` or `address already in use`, the
host's `docker-proxy` is stuck; restart the Docker daemon before retrying. Do not publish the port on another
interface to work around it.

## Stop and remove

```bash
docker stop multica-crawl4ai && docker rm multica-crawl4ai
```

## Limits the squad enforces

- Stealth, undetected browsers, saved logins and proxies stay off.
- Each page has a timeout; the squad caps pages in flight and pages per source per run.
- The collector refuses URLs that resolve to private, loopback or link-local addresses (`multica sales`
  guard), so the engine is never used to read an internal service.
- Facebook, Instagram, LinkedIn and TikTok are read through connections or providers, not through the
  engine.
