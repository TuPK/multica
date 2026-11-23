# Market Strategist agent pack

An agent for market analysis, customer segmentation, positioning, and finding creative outlets for a product. It is part of the marketing domain: its 19 skills live in `catalog/skills/marketing/` with the other marketing skills, and [catalog/domains/marketing/README.md](../../domains/marketing/README.md) lists the whole domain.

## Install

In the app, open **Agents** and choose **Install agent pack**. It does the same as the command below, on the runtime you pick. The app reads the pack from the bundle that `pnpm catalog:build` generates.

From the command line, on any machine running this checkout, after `multica login` and with the daemon running (`multica daemon start`):

```bash
make agent-pack PACK=market-strategist
```

The command is safe to repeat. It imports the skills, creates the agent on your first online Claude runtime, and attaches the skills. Anything that already exists is left alone.

| Option | Use |
|---|---|
| `ARGS=--dry-run` | Show what would change; change nothing |
| `ARGS=--update` | Overwrite the skills and the agent's instructions with this pack's version (this discards edits made in Delegora) |
| `ARGS="--runtime-id <id>"` | Create the agent on a specific runtime |
| `MULTICA_WORKSPACE_ID=<id>` | Install into another workspace |

Needs `multica`, `jq`, and `zip` on `PATH`. The same script works without `make`: `bash scripts/install-agent-pack.sh catalog/packs/market-strategist`.

## What it installs

- **Agent** "Market Strategist": instructions in `instructions.md`; by default only the creator can run it.
- **Skills**: the 19 listed in `pack.json`. Each name resolves to the catalog skill with that id; the pack holds no copy. Where each came from, and under which license, is in [catalog/skills/marketing/SOURCES.md](../../skills/marketing/SOURCES.md).

## Use it

Assign an issue to "Market Strategist" and describe the product, who pays, the market, and your budget and hours. It works through context, market scan, segments, beachhead, positioning, outlets, and tests, and stops for a member's decision before choosing the beachhead, before recommending anything that costs money, and before contacting anyone outside the workspace.

## Change it

- Edit `instructions.md` here, then run `pnpm catalog:build` and install with `--update`.
- Edit a skill in `catalog/skills/marketing/<group>/<skill>/`. The marketing squads use the same copy, so an edit there changes both.
- Add or remove a skill: list its id in `pack.json`. A skill that is not in the catalog must have a folder under this pack's `skills/`; this pack has none now.
- Upstream skills are copied, not linked. See [catalog/skills/marketing/SOURCES.md](../../skills/marketing/SOURCES.md) for how to update one.

## Limits

- The agent also sees skills installed on the runtime itself (for example other plugins). Disable the ones it does not need in the agent's Skills tab.
- The Vietnam channel list in `catalog/skills/marketing/research/outlet-finder/references/local-channels.md` is a set of starting hypotheses, not verified data.
