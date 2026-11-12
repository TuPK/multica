# Market Strategist agent pack

An agent and 19 skills for market analysis, customer segmentation, positioning, and finding creative outlets for a product.

## Install

In the app, open **Agents** and choose **Install agent pack**. It does the same as the command below, on the runtime you pick. The app reads its copy of the pack from `packages/core/agent-packs/bundles/market-strategist.json`.

From the command line, on any machine running this checkout, after `multica login` and with the daemon running (`multica daemon start`):

```bash
make agent-pack PACK=market-strategist
```

The command is safe to repeat. It imports the skills, creates the agent on your first online Claude runtime, and attaches the skills. Anything that already exists is left alone.

| Option | Use |
|---|---|
| `ARGS=--dry-run` | Show what would change; change nothing |
| `ARGS=--update` | Overwrite the skills and the agent's instructions with this pack's version (this discards edits made in Multica) |
| `ARGS="--runtime-id <id>"` | Create the agent on a specific runtime |
| `MULTICA_WORKSPACE_ID=<id>` | Install into another workspace |

Needs `multica`, `jq`, and `zip` on `PATH`. The same script works without `make`: `bash scripts/install-agent-pack.sh examples/agent-packs/market-strategist`.

## What it installs

- **Agent** "Market Strategist": instructions in `instructions.md`; by default only the creator can run it.
- **Skills**: the 19 listed in `pack.json`, copied from the folders under `skills/`. Where each came from, and under which license, is in `SOURCES.md`.

## Use it

Assign an issue to "Market Strategist" and describe the product, who pays, the market, and your budget and hours. It works through context, market scan, segments, beachhead, positioning, outlets, and tests, and stops for a member's decision before choosing the beachhead, before recommending anything that costs money, and before contacting anyone outside the workspace.

## Change it

- Edit `instructions.md` or a skill under `skills/`, then run `make agent-pack-bundles` so the app's copy matches, and run the install with `--update`.
- Add or remove a skill: add or delete its folder and list it in `pack.json`.
- Upstream skills are copied, not linked. See `SOURCES.md` for how to update one.

## Limits

- The agent also sees skills installed on the runtime itself (for example other plugins). Disable the ones it does not need in the agent's Skills tab.
- The Vietnam channel list in `skills/outlet-finder/references/local-channels.md` is a set of starting hypotheses, not verified data.
