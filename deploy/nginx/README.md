# Public proxy: nginx and Let's Encrypt

`docker-compose.selfhost.yml` runs an nginx reverse proxy (service `nginx`) and a
certbot renewer (service `certbot`) under the `proxy` profile. Both start with the
rest of the stack when `.env` has `COMPOSE_PROFILES=proxy`.

## Design

**Routing by subdomain, not by path.** Each app expects to own its origin:

| Host | Upstream | Why |
| --- | --- | --- |
| `delegora.io.vn` | `landing:80` | Marketing site, served at the root. |
| `app.delegora.io.vn` | `frontend:3000`, plus `backend:8080` for `/api`, `/v1`, `/ws`, `/collab`, `/health`, `/auth` (not the OAuth callbacks) | The web app uses root-relative paths and `/ws` at the root. A path prefix would need a rewrite for every route and asset. |
| `platform.delegora.io.vn` | `platform:30001` | The console is a root-mounted SPA with `/api/platform/*` on the same origin. |

Cookies stay host-only on `app.`, so `COOKIE_DOMAIN` stays empty. Splitting the
hosts also keeps the marketing and admin surfaces out of the app's origin.

**WebSockets go straight to the backend.** The web app's Next.js runtime proxy
(`apps/web/config/runtime-urls.ts`) rewrites plain HTTP only. Realtime (`/ws`),
collab (`/collab`) and the daemon socket (`/api/daemon/ws`) would fail through it,
so nginx routes those paths to the backend with the `Upgrade` headers set. The
backend path list in `site.conf.template` mirrors `runtimeRewriteDestination`.

**Upstreams resolve at request time.** nginx uses Docker's embedded DNS
(`127.0.0.11`) with variables in `proxy_pass`, so it starts even while a service is
restarting. The platform service exits until its schema is migrated, which is why
this matters.

**TLS: Let's Encrypt with HTTP-01 through a webroot.** The alternatives:

- *Caddy* handles issuance and renewal itself, with less config, but it would replace
  nginx, which the owner asked for.
- *DNS-01* works without port 80 but needs DNS-provider API credentials in the stack.
- *A Cloudflare Tunnel* needs no open ports but puts Cloudflare in the path and
  requires an account and token.

HTTP-01 is the simplest fit. It needs port 80 reachable from the internet, and a
certificate covers all three names (`delegora.io.vn`, `app.` and `platform.`), so one
renewal covers them.

- `certbot` writes certificates to the `certbot-conf` volume and the challenge files
  to the `certbot-www` volume. `nginx` mounts both (certificates read-only).
- Until a certificate exists, nginx serves only the challenge (`bootstrap.conf.template`).
  Once `live/<TLS_DOMAIN>/` appears, `entrypoint.sh` renders `site.conf.template`,
  checks it with `nginx -t`, and reloads. A reload also runs every 12 hours, so
  renewed certificates load.
- Renewal: `certbot renew` runs twice a day and renews within 30 days of expiry.
- Failed issuance (for example DNS not pointing here yet) retries every 30 minutes, to
  stay clear of Let's Encrypt's failed-validation rate limit.
- Certificate state is in the `certbot-conf` volume. Back it up with the database. If it is
  lost, certbot issues a new certificate on the next start, subject to Let's Encrypt's
  issuance rate limits.

**Ports.** The proxy publishes `80` and `443` on all interfaces. These are the only
public ports. The loopback mappings on `frontend`, `backend`, `landing` and `platform`
stay as they were for local use. Nothing else is published on all interfaces. Docker
bypasses UFW for published ports, so the firewall must allow only what you intend.

## What the owner must do

1. **DNS.** `157.10.52.181` is the production host, and `delegora.io.vn` and `www`
   already point to it. The proxy must run on that host, so the subdomains go there too.
   The owner adds these records:

   | Type | Name | Value | TTL |
   | --- | --- | --- | --- |
   | A | `app` | `157.10.52.181` | 300 |
   | A | `platform` | `157.10.52.181` | 300 |

   Leave `@` and `www` as they are. Check also for existing `AAAA` records: if they
   point elsewhere, Let's Encrypt may validate over IPv6 and fail.

2. **Host firewall and router.** Ports `80` and `443` on `157.10.52.181` must be
   reachable from the internet, through the firewall and any NAT in front of it.
   The development machine that generated this change is not public, so Let's Encrypt
   cannot validate against it. Test the stack on the production host.

3. **`.env`**, next to the existing values:

   ```bash
   COMPOSE_PROFILES=collab,proxy   # or just proxy
   TLS_DOMAIN=delegora.io.vn
   TLS_EMAIL=contact@delegora.io.vn
   FRONTEND_ORIGIN=https://app.delegora.io.vn
   MULTICA_PUBLIC_URL=https://app.delegora.io.vn
   MULTICA_TRUSTED_PROXIES=172.16.0.0/12
   RATE_LIMIT_TRUSTED_PROXIES=172.16.0.0/12
   ```

   `MULTICA_TRUSTED_PROXIES` and `RATE_LIMIT_TRUSTED_PROXIES` must cover the address
   nginx has on the compose network. Check it with
   `docker network inspect multica_default` and widen the range if needed.

## Operating it

```bash
docker compose -f docker-compose.selfhost.yml up -d nginx certbot
docker compose -f docker-compose.selfhost.yml logs -f certbot
docker compose -f docker-compose.selfhost.yml exec nginx nginx -t
```

## Not in this change

- The platform's webhook listener (`PLATFORM_WEBHOOK_PORT`) is not routed. If an
  external service must call it, add a `location` for that path on `platform.` (or
  a dedicated host) once the caller and its path are known.
- No HTTP→HTTPS redirect exemption beyond the ACME path.
- No `www.` host.
