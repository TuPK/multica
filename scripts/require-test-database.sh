#!/usr/bin/env bash
# Guard in front of every database-backed test command.
#
# Reads DATABASE_URL from the process environment only: no default, no env file,
# no connection. Exits 0 and prints "test database: <name>" when the URL points
# at a dedicated test database; exits 1 with the reason otherwise.
#
#   DATABASE_URL='postgres://u:p@localhost:5432/app_test?sslmode=disable' \
#     bash scripts/require-test-database.sh && ( cd server && DATABASE_URL=... go test ... )
set -u

reject() {
  echo "require-test-database: $*" >&2
  exit 1
}

url="${DATABASE_URL:-}"
if [ -z "$url" ]; then
  reject "DATABASE_URL is empty or unset; set it explicitly in the command to a database whose name ends in _test"
fi

case "$url" in
  postgres://* | postgresql://*) ;;
  *) reject "DATABASE_URL must be a postgres:// or postgresql:// URL (key=value DSNs are not accepted)" ;;
esac

rest="${url#*://}"
query=""
case "$rest" in
  *\?*) query="${rest#*\?}" ;;
esac
rest="${rest%%\?*}"
rest="${rest%%#*}"

# The query may rename the database (the dbname and database parameters).
lowered_query=$(printf '%s' "$query" | tr '[:upper:]' '[:lower:]')
case "&$lowered_query" in
  *"&dbname="*) reject "DATABASE_URL must not carry a dbname= parameter; put the database name in the path" ;;
  *"&database="*) reject "DATABASE_URL must not carry a database= parameter; put the database name in the path" ;;
esac

case "$rest" in
  */*) name="${rest#*/}" ;;
  *) name="" ;;
esac

if [ -z "$name" ]; then
  reject "DATABASE_URL has no database name in the path"
fi

if ! printf '%s' "$name" | grep -Eq '^[A-Za-z0-9_]+$'; then
  reject "database name '$name' has a character outside [A-Za-z0-9_]"
fi

lowered_name=$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')
if [ "$lowered_name" = "multica" ]; then
  reject "database name '$name' is the shared development database (multica); use a dedicated test database"
fi

if ! printf '%s' "$name" | grep -Eq '^.+_test$'; then
  reject "database name '$name' does not end in _test"
fi

echo "test database: $name"
