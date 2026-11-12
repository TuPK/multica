# Sources

Every skill under `skills/` is copied unchanged from the upstream commit below, except the two marked "this pack". Upstream licenses are in `licenses/`; keep them with the files.

Before importing, each skill was checked for non-markdown files (none, apart from JSON eval fixtures in `marketingskills`) and for shell, network, or prompt-injection patterns (none found). Skills are not sandboxed by Multica, so repeat that check whenever you update a skill from upstream.

| Skill | Upstream | Commit | License |
|---|---|---|---|
| `product-marketing`, `customer-research`, `competitor-profiling`, `marketing-ideas` | [coreyhaines31/marketingskills](https://github.com/coreyhaines31/marketingskills) `skills/<name>` | `820ad2c1c0ac4cc350158bd7912c753681a7d173` | MIT, `licenses/marketingskills.LICENSE` |
| `market-sizing`, `user-segmentation` | [phuryn/pm-skills](https://github.com/phuryn/pm-skills) `pm-market-research/skills/<name>` | `2a12295a7717569baa7be181b4cfebe3ca38a6bb` | MIT, `licenses/pm-skills.LICENSE` |
| `ideal-customer-profile`, `beachhead-segment`, `growth-loops`, `gtm-motions` | phuryn/pm-skills `pm-go-to-market/skills/<name>` | same | same |
| `value-proposition` | phuryn/pm-skills `pm-product-strategy/skills/value-proposition` | same | same |
| `prioritize-assumptions` | phuryn/pm-skills `pm-product-discovery/skills/prioritize-assumptions` | same | same |
| `mom-test`, `jobs-to-be-done`, `obviously-awesome` | [wondelai/skills](https://github.com/wondelai/skills) `<name>` | `c172996495bed0fcd26896a9416b2093fd7073f0` | MIT, `licenses/wondelai-skills.LICENSE` |
| `distribution-analysis`, `cac-modeler` | [MaxKmet/idea-validation-agents](https://github.com/MaxKmet/idea-validation-agents) `skills/<name>` | `3a4c800a3022e73e9997533b603c79b60ee7597a` | MIT, `licenses/idea-validation-agents.LICENSE` |
| `outlet-finder`, `evidence-discipline` | this pack | n/a | same as this repository |

## Updating a skill

1. Clone the upstream repository at the commit you want and read the diff against `skills/<name>`.
2. Run the checks described above on the new files.
3. Replace `skills/<name>`, update the commit in the table, and install with `--update` (see `README.md`).

## Local changes

None to the upstream skills.
