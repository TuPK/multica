#!/bin/sh
# Entrypoint of the nginx proxy service (docker-compose.selfhost.yml, profile "proxy").
# Renders the config for TLS_DOMAIN and keeps nginx in step with certbot: the
# bootstrap config (ACME challenge only) until the certificate exists, the TLS site
# after that, and a reload every 12 hours so renewed certificates are loaded.
set -eu

: "${TLS_DOMAIN:?TLS_DOMAIN must be set, for example delegora.io.vn}"

DEPLOY=/etc/nginx/deploy
CONF=/etc/nginx/conf.d/default.conf
CERT_DIR="/etc/letsencrypt/live/$TLS_DOMAIN"

# Writes the config for the current certificate state. Returns 0 only when the
# content changed and nginx accepted it.
apply() {
  if [ -s "$CERT_DIR/fullchain.pem" ] && [ -s "$CERT_DIR/privkey.pem" ]; then
    template="$DEPLOY/site.conf.template"
  else
    template="$DEPLOY/bootstrap.conf.template"
  fi
  # Substitute only TLS_DOMAIN, so nginx variables such as $host survive.
  envsubst '$TLS_DOMAIN' < "$template" > "$CONF.next"
  if [ -f "$CONF" ] && cmp -s "$CONF.next" "$CONF"; then
    rm -f "$CONF.next"
    return 1
  fi
  mv "$CONF.next" "$CONF"
  nginx -t
}

apply || true
nginx

trap 'nginx -s quit; exit 0' TERM INT

n=0
while :; do
  sleep 60 & wait $!
  n=$((n + 1))
  if apply; then
    echo "nginx: configuration changed, reloading"
    nginx -s reload
  fi
  if [ $((n % 720)) -eq 0 ]; then
    nginx -s reload
  fi
done
