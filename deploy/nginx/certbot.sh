#!/bin/sh
# Entrypoint of the certbot service (docker-compose.selfhost.yml, profile "proxy").
# Obtains one Let's Encrypt certificate for TLS_DOMAIN and its app and platform
# subdomains with the HTTP-01 challenge, served by nginx from the shared webroot.
# Then it runs `certbot renew` twice a day; certbot renews only within 30 days of expiry.
set -eu

: "${TLS_DOMAIN:?TLS_DOMAIN must be set, for example delegora.io.vn}"
: "${TLS_EMAIL:?TLS_EMAIL must be set: Let's Encrypt sends expiry notices to it}"

WEBROOT=/var/www/certbot

trap 'exit 0' TERM INT

issue() {
  certbot certonly --non-interactive --agree-tos --email "$TLS_EMAIL" \
    --webroot -w "$WEBROOT" --cert-name "$TLS_DOMAIN" \
    -d "$TLS_DOMAIN" -d "app.$TLS_DOMAIN" -d "platform.$TLS_DOMAIN"
}

# Until the first issuance succeeds, retry every 30 minutes. Failed validations count
# against Let's Encrypt's rate limits, so retries are not made more often than that.
until [ -f "/etc/letsencrypt/renewal/$TLS_DOMAIN.conf" ]; do
  if ! issue; then
    echo "certbot: issuance failed (DNS or port 80 not ready?); retrying in 30 minutes" >&2
    sleep 1800 & wait $!
  fi
done

while :; do
  certbot renew --non-interactive --webroot -w "$WEBROOT" ||
    echo "certbot: renewal failed; retrying in 12 hours" >&2
  sleep 43200 & wait $!
done
