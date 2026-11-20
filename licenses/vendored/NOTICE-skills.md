# Notice: skills adapted from third-party sources

Some of the skills that ship in the binary (`web/resources/skills/`) were adapted, with attribution, from two
sources the author approved (decision D-071). The adapted text is rewritten for Stuarty; the licence text of each
source is kept in this folder, and each adapted skill names its source, the path of the original and the commit
it was read at in its front matter (`license`, `metadata.stuarty-source`, `metadata.stuarty-adapted-from`).

| Source | Licence | Commit read | Licence text |
| --- | --- | --- | --- |
| `coreyhaines31/marketingskills` | MIT, Copyright (c) 2025 Corey Haines | `73573e94a06a39965978b3fa1f9c1eace8cebc7c` | `marketingskills-MIT.txt` |
| `heygen-com/hyperframes` | Apache-2.0, Copyright 2026 HeyGen, Inc. | `77ea9d80cfc8b347ccca966487a8ff902ce28320` | `hyperframes-Apache-2.0.txt` |

## What was adapted, and what changed

| Skill of Stuarty | Source and path | What was changed |
| --- | --- | --- |
| `write-a-hook` | `marketingskills`: `skills/social/SKILL.md` (hook formulas, the list of machine-sounding phrases) and `skills/social/references/short-form-video.md` (the hook families) | Condensed to five families, rewritten in other words, tied to the brief and to the operations of Stuarty; steps and rules are new |
| `short-script` | `marketingskills`: `skills/social/references/short-form-video.md` (the scripting template and the story and scene structures) | Rewritten, with the timing tied to the duration of the brief and the script kept as a text item of the project |
| `caption-per-network` | `marketingskills`: `skills/social/SKILL.md` and `skills/social/references/platform-limits.md` (the limits per network) | Limits copied as facts and reduced to the networks of the first flow; the steps, the fitting rules and the checks are new |
| `captions-for-shorts` | `hyperframes`: `.agents/skills/captions-overlay/SKILL.md` (the caption model: leave out the filler, keep the rest verbatim on one line, mark a punch word, the caption is an overlay) | The model is kept in three sentences; every step and every operation named is Stuarty's own |
| `motion-with-an-external-engine` | `hyperframes`: `skills/hyperframes-cli/SKILL.md` (the order of the steps around a render: lint and check the composition, look at the snapshots, render with a quality level, confirm the duration of the file; the form of the render command line) | The order and the three command forms are kept as facts, in other words, as one step of six; every other step, every operation named and every rule are Stuarty's own, and the skill says the engine is the agent's and the person's, never installed or run by Stuarty |
| `commercial-copy` | `marketingskills`: `skills/ad-creative/references/hook-system.md` (a hook has parts that do different work; the seconds after it keep its promise) and `skills/ad-creative/references/short-form-video-specs.md` (the beats and the timing of a short ad) | Rewritten in other words for a commercial of 15 or 30 seconds: the three parts of the hook, the lines per beat and the rule that every claim traces to the brief are Stuarty's own; no text is copied |

No file of either source is copied as it is, and no code, binary or asset is taken from them. The Apache-2.0 source
carries no NOTICE file at the commit read. These skills are text that an agent reads: Stuarty never runs anything
that comes with a skill (SDD-018).
