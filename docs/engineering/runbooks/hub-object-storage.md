# Skill / Plugin Hub object storage

The Hub keeps its immutable bundles in an S3-compatible store. The self-host compose
file runs SeaweedFS for it. This runbook covers what is stored and where, the
configuration, backup and restore, the staging expiry rule, credential rotation, and
moving to another S3 endpoint.

## 1. What is stored where

| Data | Where | Notes |
|---|---|---|
| Bundle zip of an item, per digest | Hub store, `hub/{workspace}/{kind}/{ref_id}/{digest}/bundle.zip` | Immutable. Written at import commit, or on the first bundle request for a skill or plugin that has none. |
| Staged import preview (zip and metadata) | Hub store, `staging/{workspace}/{preview}.zip` and `.json` | Transient. A preview expires 30 minutes after it is staged. |
| Item rows, digests, version rows | Postgres: `hub_item`, `hub_item_version_blob` | `bundle_key` and `bundle_digest` point at the store. Version rows are append-only. |
| Plugin runtime files and manifests | Postgres: `plugin_package_version`, `plugin_package_file` | Unchanged. Plugin availability must not depend on the object store, so these never move to it. |
| Skill content and supporting files | Postgres: `skill`, `skill_file` | Unchanged. |

The store holds bundles, which are snapshots, and staged previews. It does not hold live
plugin or skill content. Losing the store loses the bundles, not the items: a skill's or
plugin's current files stay in Postgres. A rebuilt bundle has a new digest, and a bundle
that was committed from an import cannot be rebuilt from Postgres at all, so back the store up.

## 2. Configuration

Hub storage reads `HUB_S3_*`. It is separate from the attachment bucket (`S3_*`).

| Variable | Default | Meaning |
|---|---|---|
| `HUB_S3_BUCKET` | unset | Bucket name. Unset, the Hub uses the attachment store, or local disk when that is unset. Compose sets `multica-hub`. |
| `HUB_S3_REGION` | `us-east-1` | Signing region. |
| `HUB_S3_ENDPOINT` | unset (AWS) | Custom endpoint. Compose sets `http://seaweedfs:8333`. |
| `HUB_S3_ACCESS_KEY` / `HUB_S3_SECRET_KEY` | unset | Static credentials. Unset, the default AWS credential chain is used. |
| `HUB_S3_USE_PATH_STYLE` | `true` when `HUB_S3_ENDPOINT` is set | Path-style addressing. |
| `HUB_S3_PUBLIC_ENDPOINT` | unset | The address clients reach the store at. Presigned download links are signed against it. Unset, bundles download through the server. |

| `HUB_MAX_UPLOAD_BYTES` | `16777216` (16 MiB) | Archive cap for a zip uploaded to the import preview. Any positive byte count; zero, blank or non-numeric values keep the default. Commit reads the staged copy under the same cap. The cap for fetched sources (GitHub, zip URL) stays 16 MiB. |
| `MULTICA_HUB_SECRET_KEY` | unset | Base64 of 32 random bytes. Seals the access token of a private source (GitHub, zip URL, registry). Unset, saving a credential fails with `409 secrets_not_configured`; nothing is stored in plaintext. Changing the key makes stored tokens unreadable: previews answer `409 secrets_not_configured` and syncs record a credential error until the owner saves the token again. |

Generate the Hub key with `openssl rand -base64 32`. It is separate from every other secret key, so it rotates on its own. Tokens are pinned to the source's host: a redirect to another host drops them, and a source whose URL moves to another host loses its stored token.

With a custom endpoint the backend creates the bucket at startup when it is missing. A
failure is logged and does not stop the server. Startup logs `hub storage initialized`
with the bucket, the endpoint and whether presigning is on. Without `HUB_S3_ENDPOINT`
(AWS) the bucket must already exist.

### Local development with `make dev-full`

`make dev-full` starts the same `seaweedfs` service on loopback ports and writes what the host-run
backend (`make dev`) needs to the env file: `HUB_S3_ENDPOINT=http://localhost:<HUB_S3_API_PORT>`, the
bucket, region and path style, and the credentials. It then confirms the bucket with a signed request
and creates it if it is missing. The ports are free ports from 8333 and 9333, kept in the env file.
A SeaweedFS that is already running (for example, from another checkout that uses the same compose
project) is adopted with its ports and credentials, so the containers are not recreated. The
published ports are also the loopback addresses listed in `SELF_HOSTING.md`.

The store is shared by every checkout that uses the compose project `multica`, as the WeKnora
services are. Its data is the `multica_seaweedfs-data` volume. `make dev-full` does not remove it.
The cleanup commands are in [fresh-machine-install.md](fresh-machine-install.md#removing-the-local-stores).

Why `HUB_S3_PUBLIC_ENDPOINT` is separate: the compose endpoint `http://seaweedfs:8333`
resolves only inside the compose network. A presigned link to it cannot be opened by a
browser or a desktop client. Without a public endpoint the bundle route answers with
`/api/hub/items/{kind}/{id}/bundle/download`, which the server streams.

## 3. Backup and restore of the SeaweedFS volume

The bundles live in the named volume `seaweedfs-data`. Compose prefixes the volume name
with the project name, `multica` (`docker volume ls | grep seaweedfs-data`).

Take the backup together with the Postgres dump, and after it. Bundles are never deleted
by the application, so a volume copy taken after the dump holds every bundle the dump
refers to. A copy taken before the dump can miss bundles the dump refers to.

Backup:

```sh
docker compose -f docker-compose.selfhost.yml stop seaweedfs
docker run --rm \
  -v multica_seaweedfs-data:/data:ro \
  -v "$PWD":/backup \
  alpine tar czf /backup/seaweedfs-data-$(date +%Y%m%d-%H%M).tgz -C /data .
docker compose -f docker-compose.selfhost.yml start seaweedfs
```

Restore onto a stopped stack:

```sh
docker compose -f docker-compose.selfhost.yml stop backend seaweedfs
docker run --rm \
  -v multica_seaweedfs-data:/data \
  -v "$PWD":/backup \
  alpine sh -c 'rm -rf /data/* /data/.[!.]* 2>/dev/null; tar xzf /backup/seaweedfs-data-YYYYMMDD-HHMM.tgz -C /data'
docker compose -f docker-compose.selfhost.yml start seaweedfs
docker compose -f docker-compose.selfhost.yml start backend
```

Restore in this order: Postgres first (`make migrate-up` so the schema matches the
binary), then the volume.

Check after a restore. For each item that has a bundle, the object must exist:

```sql
SELECT workspace_id, kind, ref_id, bundle_key FROM hub_item WHERE bundle_key <> '';
```

Then, for a sample of rows, request the bundle through the API. A missing object answers
`404 bundle not found`. A list of the bucket taken with any S3 client pointed at the
endpoint can also be compared with the keys above.

## 4. Staging expiry rule

Staged previews are removed by the application when they are committed, and they are
never read after 30 minutes. A preview that is abandoned leaves its objects behind, and
the expiry rule removes them. The rule targets the top-level `staging/` prefix, which is
why staging is kept apart from the bundles under `hub/`.

The backend also runs an hourly sweep (`jobs_hub_staging.go`) that deletes `staging/` objects older than 60 minutes. It complements the bucket rule, which stays the backstop for stores or periods where the sweep does not run.

Apply the rule once the bucket exists. The application does not set it. The rule is:

```json
{
  "Rules": [
    {
      "ID": "expire-hub-staging",
      "Status": "Enabled",
      "Prefix": "staging/",
      "Expiration": { "Days": 1 }
    }
  ]
}
```

With the AWS CLI against SeaweedFS:

```sh
aws s3api put-bucket-lifecycle-configuration \
  --endpoint-url http://127.0.0.1:8333 \
  --bucket multica-hub \
  --lifecycle-configuration file://hub-lifecycle.json
aws s3api get-bucket-lifecycle-configuration --endpoint-url http://127.0.0.1:8333 --bucket multica-hub
```

Status: SeaweedFS 4.53 accepts the rule and returns it from the get call. Expiry itself
has not been observed yet. Confirm it once a staged preview is older than a day: its
objects under `staging/` should be gone. Never give a rule the `hub/` prefix. It would
expire every bundle.

## 5. Credential rotation

The SeaweedFS identity is written from `HUB_S3_ACCESS_KEY` and `HUB_S3_SECRET_KEY` each
time the container starts. The backend reads the same values. Rotate both together:

1. Generate a new secret: `openssl rand -hex 24`.
2. Change `HUB_S3_ACCESS_KEY` and/or `HUB_S3_SECRET_KEY` in `.env`.
3. Recreate SeaweedFS and the backend together, so they start with the same values:

   ```sh
   docker compose -f docker-compose.selfhost.yml up -d --force-recreate seaweedfs backend
   ```

Expect uploads to fail until the backend has restarted. Presigned links issued with the
old key stop working when SeaweedFS restarts. They last up to 15 minutes, so a client
that holds one retries the bundle request.

For another endpoint, rotate the key at the provider, change the variables, and restart
the backend.

## 6. Moving to another S3 endpoint

No code change is needed. The Hub talks only to the S3 API.

1. Make the new endpoint hold the bundles. Copy every key under `hub/` from the old
   bucket to the new one, with any S3 sync tool, for example `rclone copy`. Staged
   previews do not need copying; they expire in 30 minutes.
2. Set `HUB_S3_ENDPOINT` (empty for AWS), `HUB_S3_REGION`, `HUB_S3_BUCKET`,
   `HUB_S3_ACCESS_KEY`, `HUB_S3_SECRET_KEY`, and `HUB_S3_USE_PATH_STYLE` (`true` for
   SeaweedFS and MinIO-style endpoints, `false` for AWS virtual-hosted addressing).
   Set `HUB_S3_PUBLIC_ENDPOINT` if clients should get presigned links.
3. Restart the backend and check the `hub storage initialized` line in its log.
4. Re-apply the staging expiry rule to the new bucket (section 4).

Rollback: restore the old variables and restart the backend. Objects written to the new
bucket after the switch are not copied back.

## 7. Metrics

Exposed on the server's `/metrics` endpoint, in the `multica_hub_` family. Every series is present from startup at zero.

| Metric | Labels | Meaning |
|---|---|---|
| `multica_hub_sync_duration_seconds` | none | Histogram of one source sync, from its lock to its recorded result. |
| `multica_hub_sync_runs_total` | `result` = `ok` \| `error` | Source syncs. `error` means at least one item could not be checked. |
| `multica_hub_sync_items_checked_total` | none | Items checked against their remote by syncs, successful or not. |
| `multica_hub_sync_updates_found_total` | none | Items a sync found with a newer remote version. |
| `multica_hub_import_commits_total` | `kind` = `skill` \| `plugin`, `result` = `ok` \| `skipped` \| `error` | Import commits. `skipped` is a conflict strategy that kept the existing item. |
| `multica_hub_bundle_builds_total` | `result` = `ok` \| `error` | Bundle builds and stores. |
| `multica_hub_storage_errors_total` | `operation` = `put` \| `get` \| `delete` | Failed Hub object-store operations. A missing object is not counted. |

A rising `multica_hub_storage_errors_total` with a flat `multica_hub_bundle_builds_total` points at the store (section 2 and section 8 below). A rising `multica_hub_sync_runs_total{result="error"}` on one source points at that source: its last sync error names the cause, and a credential error means the Hub key changed or is unset.

## 8. Troubleshooting

| Symptom | Cause and action |
|---|---|
| Log: `hub storage: could not ensure bucket exists` | The endpoint is unreachable or the credentials lack bucket-create rights. Uploads fail until the bucket exists. |
| `503 file storage is not configured` on bundle or import routes | Neither the Hub store nor the attachment store could be built. Check the log for `failed to load AWS config`, or the local upload directory. |
| `404 bundle not found` on the download route | The item's `bundle_key` names an object that is missing from the store. Restore the volume or copy the bundle back (section 3). |
| `404 preview not found` for an import committed right after a deploy | The preview was staged under the old `hub/…/staging/` key. It expires within 30 minutes; preview again. |
| `500 InternalError`, and the SeaweedFS log says `Not enough data nodes found` or `No writable volumes` for a collection | SeaweedFS has a default cap of 8 volumes per volume server (`Max` in `http://<HUB_S3_UI_PORT>/vol/status`). Each bucket is a collection that takes volumes, and the first one takes most of them, so a second bucket in the same store cannot take writes. The Hub uses one bucket, so this does not affect it. A test or other bucket in the same store does. Use a separate store for another bucket, or raise the cap in the compose command (`-volume.max`). The compose file does not set it. |

## 9. Verification record

| Check | Result |
|---|---|
| Object operations (list with continuation, copy, presigned GET and PUT, bucket create) against SeaweedFS 4.53 | Passed (`HUB_S3_TEST_ENDPOINT=... go test ./internal/storage -run TestS3StorageAgainstEndpoint`). |
| Staging expiry rule accepted by SeaweedFS 4.53 | Accepted and returned by get. Expiry not yet observed. |
| Volume backup and restore on a copy | Not run. |
| Credential rotation on a copy | Not run. |
| `make dev-full` Hub path (`scripts/dev-full.sh`, functions loaded with `DEV_FULL_LIB=1`) against an isolated compose project on the same host, 2026-11-11 | Passed: first start publishes 8333/9333, the status page answers, the bucket is created. A second checkout with its own generated secret adopts the running store's ports and credentials, and the container is not recreated. Stopped with `down -v`, volume removed. Not run: the full `make dev-full`. |
| Repo S3 integration test (`TestS3StorageAgainstEndpoint`) against that store, bucket `multica-hub` | Passed. Against bucket `hub-it` it failed with the volume cap above, because the store had no free volumes left. |
| Backend env contract (`TestNewHubS3StorageFromEnv*`) | Passed. |
