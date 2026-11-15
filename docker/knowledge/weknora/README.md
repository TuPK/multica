# WeKnora patches

We run WeKnora as published images and do not fork it or push to it. A bug that
blocks us is carried here as a patch, and `scripts/knowledge/build-weknora-app.sh`
applies it to the upstream source and builds the image.

- `BASE_COMMIT`: the upstream commit the patches apply to.
- `patches/*.patch`: applied in name order with `git apply`. Create each one with
  `git format-patch` from a commit on top of `BASE_COMMIT`.

Rebuild after a change to the patches or a new upstream release:

```bash
scripts/knowledge/build-weknora-app.sh --check
```

Then set `KNOWLEDGE_VERSION=<tag printed by the script>` in `.env` and recreate the
`knowledge-app` service. Patches listed here:

| Patch | Fixes |
| --- | --- |
| `0001-fix-retriever-keyword-search-with-generated-question-exclusion.patch` | Keyword search failed with HTTP 500 `Unsupported query shape` in any knowledge base that has generated questions. |
| `0002-build-strip-llvm-bitcode-from-the-anydoc-archive.patch` | The anydoc build failed at the Go link: Go 1.26.x rejects the `.llvmbc` section in the Rust archive (`disallowed LTO section`). The script now removes that section and keeps anydoc. |
