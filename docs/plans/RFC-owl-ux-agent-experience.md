# RFC: Owl UX + Agent Experience Overhaul

- **Status:** Draft (Proposed)
- **Authors:** Joel + Geth
- **Date:** 2026-04-09
- **Target:** Owl CLI + TUI + Agent Runtime Integration
- **Related docs:**
  - `plans/orchestration-rebuild/BLUEPRINT.md`
  - `plans/orchestration-rebuild/phases/PHASE-4-owl-observability.md`
  - `plans/orchestration-rebuild/phases/PHASE-5-promptops.md`

---

## 1) Summary

This RFC proposes a comprehensive redesign of the Owl user and agent experience to support:

1. A first-class **meta-agent** in Owl’s main console.
2. Stronger, explicit, and safer **agent hatching semantics**.
3. Portable **AGENTS.md-style agent definitions** with layered prompt architecture.
4. Better **operability** (stop/remove/control running agents via TUI and CLI).
5. Continuous improvement via **logs access + metrics + prompt iteration loop**.
6. Support for importing existing agent identities with **global/project scope**.

The goal is to evolve Owl from “spawn helper” into a robust local control surface for orchestrated agent systems.

---

## 2) Motivation

Current and emerging constraints:

- Provider harness lock-in means runtime behavior must be predictable and adapter-friendly.
- Prompt quality and guardrail adherence are now core reliability concerns, not optional polish.
- Agent systems need operational controls (inspect, stop, resume, remove) and measurable quality.
- Existing `hatch [prompt]` flow is ambiguous and weak for reproducibility and governance.

Without this redesign, users face:

- inconsistent startup behavior,
- brittle prompt practices,
- poor observability,
- and high operational friction as systems scale.

---

## 3) Goals

### Primary goals

1. Make Owl’s first interaction useful and guided via a **meta-agent console**.
2. Replace ambiguous hatch UX with explicit command contracts.
3. Adopt a portable prompt model based on **AGENTS.md-style** definitions.
4. Support **global and project scope** for reusable identities.
5. Enable full run lifecycle control from CLI and TUI.
6. Capture actionable metrics to support autonomous or guided prompt evolution.

### Secondary goals

- Improve naming consistency (`--agent` over `--template`).
- Preserve backwards compatibility with clear deprecation pathways.
- Keep architecture composable with Viche and orchestration workflows.

---

## 4) Non-Goals (for this RFC)

- Building a fully autonomous self-modifying system without approvals.
- Replacing orchestrator logic in Geth.
- Defining provider-specific runtime internals (adapter-level details live in separate RFCs).
- Designing complete cloud sync/distribution infrastructure.

---

## 5) Proposed UX Model

## 5.1 First-run / Main Console Experience

When opening Owl, users land in a main console with an embedded **meta-agent** session:

- “What do you want to build/run?”
- “Do you have an existing agent identity to import?”
- “Do you want global or project scope?”
- “Would you like guardrail validation before hatch?”

The meta-agent is:

- interactive,
- context-aware (workspace/project settings),
- guardrail-aware,
- and prompt-quality-aware.

### Meta-agent responsibilities

- Construct candidate agent definitions.
- Validate definitions against policy/guardrails.
- Explain resolved prompt stack.
- Suggest prompt improvements from logs/metrics.
- Produce patch proposals (diffs), not silent changes by default.

---

## 5.2 Hatch Command UX Changes

### Current issue
`hatch [prompt]` is ambiguous and hard to reproduce.

### Proposed hatch contract

Preferred:

- `owl hatch --agent <agent_name>`

Optional:

- `--prompt "..."` (inline override)
- `--from-file <path>` (prompt override from file)
- `--scope project|global` (resolution hint)
- `--dry-run` (show resolved setup, no execution)

### Transitional compatibility

- Keep `--template` as deprecated alias for `--agent` for at least one minor release.
- Keep bare positional prompt temporarily with warning + migration guidance.

### Validation behavior

Hatch fails fast if:

- agent not found,
- missing required fields,
- guardrail mismatch,
- invalid scope resolution.

---

## 5.3 Naming and Mental Model

Standardize on:

- **Agent** = reusable identity + behavior contract + prompts.
- **Run** = one execution instance of an agent.
- **Template** = deprecated term retained only for migration compatibility.

---

## 6) Agent Definition Format (Portable)

## 6.1 Core direction

Use **AGENTS.md-style** human-editable prompts as primary definition surface.

### Recommended structure

```
<scope>/agents/<agent-name>/
  AGENTS.md
  role.md                  (optional)
  guardrails.md            (optional)
  metrics.md               (generated/maintained)
  agent.yaml               (optional strict metadata)
```

Where `<scope>` is either:

- global: `~/.owl/agents`
- project: `./.owl/agents`

---

## 6.2 Layered prompt architecture (tiered)

Resolved prompt stack (high to low precedence):

1. runtime override (`--prompt`, `--from-file`)
2. project agent overlays
3. project shared policy
4. global agent base
5. Owl system defaults

Owl must provide:

- `owl explain <agent>` to show fully resolved prompt and source layers.

---

## 6.3 Metadata schema (optional sidecar)

`agent.yaml` can include strict machine-readable fields:

- `name`
- `version`
- `description`
- `capabilities[]`
- `allowed_workspaces[]`
- `default_model`
- `prompt_layers[]`
- `owner`
- `tags[]`

AGENTS.md remains canonical for natural-language behavior; sidecar is for strict automation.

---

## 7) Scope and Import Model

## 7.1 Scope rules

Two scopes:

- **Global scope:** reusable across projects.
- **Project scope:** local to repository/project.

Resolution precedence:

- project > global

---

## 7.2 Import flows

New command group:

- `owl agents import --path <dir|file> [--scope project|global]`
- `owl agents export --agent <name> --out <path>`
- `owl agents promote --agent <name> --to global`
- `owl agents demote --agent <name> --to project`

Import should validate structure and auto-suggest fixes where possible.

---

## 8) Logs CLI + Meta-Agent Feedback Loop

## 8.1 Logs CLI requirements

Add command group:

- `owl logs list`
- `owl logs show <run-id>`
- `owl logs tail [--agent <name>]`
- `owl logs query --agent <name> --since <time> --json`

Required properties:

- structured output mode (`--json`),
- redaction options,
- stable schema for machine parsing.

---

## 8.2 Meta-agent usage of logs

Meta-agent can consume log query outputs to:

- detect recurring failures,
- identify weak prompt instructions,
- suggest prompt deltas,
- correlate issues with model/provider/adapter patterns.

Default mode: **proposal + approval** before modifications.

---

## 9) Runtime Controls (Stop/Remove/Manage)

## 9.1 CLI controls

Add command group:

- `owl runs list`
- `owl runs stop <run-id> [--force]`
- `owl runs remove <run-id> [--archive]`
- `owl runs inspect <run-id>`

## 9.2 TUI controls

From run list/detail views, allow:

- graceful stop,
- force stop,
- remove/archive,
- inspect logs,
- replay summary.

## 9.3 Safety semantics

- `stop` should be graceful by default.
- `--force` should require explicit confirmation in interactive mode.
- `remove` should default to archive, not hard-delete.

---

## 10) Metrics and Continuous Improvement

Each run should emit and persist metrics at minimum:

- run duration
- success/failure state
- retries
- blocker count
- handoff count
- token/cost usage (if available)
- model/provider/adapter
- prompt version/hash

Optional advanced metrics:

- quality score (human-reviewed)
- rework loops
- time-to-resolution

Meta-agent should expose:

- “what changed?” analysis across versions,
- top failure modes,
- proposed edits ranked by expected impact.

---

## 11) Prompt Mutation Policy

Default policy for v1:

1. Meta-agent proposes edits as patch/diff.
2. User reviews and approves.
3. Owl applies with version increment and changelog entry.

Optional future mode (opt-in):

- policy-bounded auto-apply with rollback guard.

---

## 12) Backwards Compatibility + Migration

### 12.1 CLI migration

- `--template` accepted with deprecation warning.
- `hatch [prompt]` accepted with warning and rewrite hint.

### 12.2 Conversion helpers

Provide:

- `owl migrate templates` to convert old config templates to AGENTS.md-style.
- `owl migrate check` to report unresolved fields and required edits.

### 12.3 Deprecation window

- At least one minor version maintaining compatibility.
- Removal gated by telemetry/usage data.

---

## 13) Security and Guardrails

Must enforce:

- workspace boundaries,
- explicit capability checks,
- sensitive log redaction,
- auditable prompt changes,
- policy validation prior to hatch.

Meta-agent must not bypass guardrails or silently escalate privileges.

---

## 14) Observability Requirements

Owl should expose:

- run timeline,
- state transitions,
- stop/remove actions audit trail,
- prompt version used per run,
- linked logs and metrics.

This supports both local debugging and orchestrator-level analysis.

---

## 15) Command Surface (Proposed)

## 15.1 Agents

- `owl agents list [--scope project|global|all]`
- `owl agents show <agent>`
- `owl agents validate <agent> [--strict]`
- `owl agents import ...`
- `owl agents export ...`
- `owl agents promote ...`
- `owl agents demote ...`
- `owl agents diff <agent> --from <v> --to <v>`
- `owl explain <agent>`

## 15.2 Hatch / Runs

- `owl hatch --agent <name> [--prompt ...] [--from-file ...] [--dry-run]`
- `owl runs list`
- `owl runs inspect <run-id>`
- `owl runs stop <run-id> [--force]`
- `owl runs remove <run-id> [--archive]`

## 15.3 Logs / Metrics

- `owl logs list`
- `owl logs show <run-id>`
- `owl logs tail ...`
- `owl logs query ... --json`
- `owl metrics show <agent|run-id>`
- `owl recommend --agent <name>` (meta-agent-driven suggestions)

---

## 16) Open Questions

1. Should `agent.yaml` be required for marketplace/imported agents, optional for local-only?
2. What is the minimum approval model for meta-agent patch application?
3. Should prompt versions be semantic (`v1.2.0`) or monotonic (`v17`)?
4. What redaction defaults are safe but still useful for debugging?
5. Should project scope support multiple environment profiles (dev/staging/prod)?

---

## 17) Phased Implementation Outline (high-level)

1. **CLI contract + naming migration** (`--agent`, deprecations)
2. **Agent definition format + resolver** (AGENTS.md layering)
3. **Meta-agent console scaffold + validation workflows**
4. **Logs CLI + structured query interface**
5. **Run controls in CLI/TUI (stop/remove/inspect)**
6. **Metrics capture + prompt improvement loop**
7. **Import/export/promote/demote identity flows**

Detailed implementation plans should be split per phase after RFC approval.

---

## 18) Acceptance Criteria for RFC Adoption

This RFC is considered accepted when:

- command surface and naming are approved,
- format/resolution model for agents is approved,
- mutation policy and safety model are approved,
- migration strategy is approved,
- and implementation phases are authorized.

---

## 19) Appendix A — Example Agent Folder

```
.owl/agents/reviewer/
  AGENTS.md
  role.md
  guardrails.md
  agent.yaml
  CHANGELOG.md
```

### Example `agent.yaml`

```yaml
name: reviewer
version: 1.0.0
description: PR review agent focused on correctness and risk
capabilities:
  - code-review
  - ci-analysis
allowed_workspaces:
  - neno
default_model: codex
owner: joel
tags:
  - review
  - quality
```

---

## 20) Appendix B — Example Hatch Flows

### Standard

```bash
owl hatch --agent reviewer
```

### With prompt override

```bash
owl hatch --agent reviewer --prompt "Focus heavily on migration safety and rollback plans."
```

### Validation-only

```bash
owl hatch --agent reviewer --dry-run
```

---

## 21) Appendix C — Suggested Telemetry Fields

- `run_id`
- `agent_name`
- `agent_version`
- `prompt_hash`
- `model`
- `adapter`
- `workspace`
- `start_ts`
- `end_ts`
- `duration_ms`
- `status`
- `retry_count`
- `blocked_count`
- `token_input`
- `token_output`
- `estimated_cost`
