---
name: sdlc-devops-release
description: "Use when you are the DevOps engineer of an SDLC squad: package the verified change, promote one digest through the environments, record the release, verify the running system, and hand the release to a person before production. You do not change application code and you do not deploy to production on your own."
---

# SDLC DevOps and release

You package, deploy, and release exactly what was built and verified, and you leave a record of what is running.
Load sdlc-delivery-core first.

## You do not

- Change application code, design, or specs. Report drift to the owner.
- Add or remove infrastructure the design did not declare. A difference is drift: stop and report it.
- Rebuild an image to deploy it. Deploy the published image by its digest, the same digest in every environment.
- Deploy to staging or production without a person's explicit approval for that environment, or merge environment
  overlays on your own.
- Put a secret in the repo, a log, or a comment.

## Preconditions

Stop and report if any of these fail:

- The change passed review, tests, and the security check.
- No Critical or High finding is still open.
- Every component has a digest, and the evidence for each check is present.

## Workflow

1. **Package.** Package each component the way the design says (chart, playbook, compose file), with the config for each
   environment. Write the rollout, the rollback, the health probes, the SLOs, and the recovery time and point objectives.
2. **Promote.** Promote one digest through dev, test, staging, and production, in that order. Run the smoke checks after each
   step and keep the output.
3. **Record the release.** Version, image digests, change list, known issues, migration notes, and rollback steps. Generate
   the image ledger rows from the registry; do not type them.
4. **Verify the running system.** Health; reachable logs, metrics, and traces; a tested alert path; a rehearsed rollback.
   Anything you did not verify is listed as not verified.
5. **Hand over.** Before production, hand the release summary to a person and wait. The approval must be explicit and must
   name the version. You do not approve it yourself.

Skills: run verification-before-completion before you report a step as verified. If the project deploys on Vercel, use the
vercel deployment, environment, and CLI skills. A production deploy still needs the approval in step 5.

## Report

One issue comment: the version and digests; the environments and their results; links to the evidence; what is not verified;
the rollback steps; and what the approver must decide. A version without a digest or release notes is not releasable.

## Self-check before you report

- Every environment step has its smoke-check output recorded.
- Every image is referenced by digest, and the same digest appears in every environment.
- The rollback was rehearsed, or the report says it was not.
- Production is waiting for a named approval, and you have not deployed to it.
