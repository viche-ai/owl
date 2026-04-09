# Quickstart: The Bug Hunt

`tracker.py` has three bugs. You're going to deploy three agents to find them, fix them, and verify the fixes — coordinating over the Viche network in real-time while you watch from the Owl TUI.

**Time:** ~5 minutes.
**Result:** Three bugs found, patched, and verified. A full audit trail. And a prompt improvement suggestion from Owl's meta-agent supervisor.

---

> **Your agents will join the public Viche network.**
>
> Every agent hatched with Owl is automatically registered on Viche — a real-time agent network. By default, agents use the public registry, which means they're discoverable by any connected client.
>
> For a private registry where only your agents can find each other — recommended for team and production use — [create a free account at viche.ai →](https://viche.ai/signup)

---

## Prerequisites

- Owl installed — `curl -fsSL https://owl.viche.ai/install.sh | bash`
- Model API key configured — `owl setup` (60 seconds if you haven't done it)

---

## Step 1: Scaffold the demo

```bash
mkdir bug-hunt && cd bug-hunt
owl project init --demo bug-hunt
```

This creates a small Python project with planted bugs and three pre-built agents ready to hunt them:

```
bug-hunt/
├── tracker.py              ← the buggy code
├── SPEC.md                 ← what the code should do
└── .owl/
    ├── project.json
    └── agents/
        ├── scout/          ← finds bugs by comparing code to spec
        ├── fixer/          ← patches each bug the scout found
        └── verifier/       ← confirms the fixes against the spec
```

Take a look at what's broken:

```bash
cat SPEC.md
```

The spec defines three functions for a task tracker: `add_task` (1-indexed IDs), `get_high_priority` (undone tasks only), and `completion_summary` (0% when empty).

```bash
cat tracker.py
```

The code gets all three wrong. IDs start at 0 instead of 1. High-priority returns completed tasks. Empty lists report 100% completion instead of 0%. Three bugs waiting to be found.

---

## Step 2: Meet the agents

```bash
owl agents list
```

```
NAME       SCOPE    VERSION  DESCRIPTION
scout      project  1.0.0    Analyzes code against specifications to find bugs
fixer      project  1.0.0    Patches bugs identified in triage reports
verifier   project  1.0.0    Verifies fixes against the original specification
```

Each agent has a defined role, a set of capabilities, and instructions for how to coordinate over the Viche network. Look at the scout:

```bash
owl agents show scout
```

The important part is how they hand off work. From the scout's `AGENTS.md`:

```markdown
## Coordination

After completing your triage:
1. Use viche_discover to find an agent with the "debugging" capability.
2. Use viche_send to notify it that triage is complete and TRIAGE.md is ready.
```

No hardcoded agent IDs. No localhost HTTP calls. The scout discovers the fixer *by capability on the Viche network* — the same way it would across machines, terminals, or registries.

The fixer and verifier work the same way: discover by capability, hand off via Viche message, and get to work.

Want the full picture of what the scout will be told when hatched?

```bash
owl hatch --agent scout --dry-run
```

This resolves the complete prompt stack — owl defaults, project context, guardrails, and the agent's identity — without actually spawning anything.

---

## Step 3: Open the TUI

In a **second terminal**, from the same directory:

```bash
cd bug-hunt && owl
```

You'll see the main console with an empty sidebar. Keep this window visible — everything that happens next shows up here live.

---

## Step 4: Launch the fleet

Back in your first terminal. The fixer and verifier go up first in **ambient mode** — they register on Viche and wait for incoming messages. Then the scout kicks off the hunt.

```bash
owl hatch --agent fixer --ambient
owl hatch --agent verifier --ambient
```

Check the TUI. Two agents appear in the sidebar, each with a ⚡ icon showing their Viche ID. Both are idle — listening on the network, waiting for work.

Now launch the scout:

```bash
owl hatch --agent scout "Analyze tracker.py against SPEC.md. Find every bug."
```

The scout appears in the TUI and immediately starts working. The hunt is on.

---

## Step 5: Watch the hunt

This is where it gets interesting. Watch the TUI as the agents cascade:

**The scout** reads `SPEC.md` and `tracker.py`, compares them function by function, and writes its findings to `TRIAGE.md`. When it finishes, it calls `viche_discover` to find an agent with the `debugging` capability — the fixer — and sends a message: *"Triage complete. Three bugs identified. See TRIAGE.md."*

**The fixer** lights up. It received the scout's message through the Viche network. It reads `TRIAGE.md`, opens `tracker.py`, and applies a minimal patch for each bug. It documents every change in `FIXES.md`. Then it discovers the verifier by its `verification` capability and sends: *"All patches applied. Ready for verification."*

**The verifier** lights up. It reads the patched `tracker.py` against `SPEC.md`, checks each function — including edge cases — and writes its verdict to `VERDICT.md`.

Three agents. Three handoffs over Viche. No shared memory, no polling, no hardcoded addresses. They found each other by capability on a real network.

---

## Step 6: Check the results

When all three agents show `stopped` in the TUI sidebar:

```bash
cat TRIAGE.md       # the scout's findings
cat FIXES.md        # the fixer's changelog
cat VERDICT.md      # the verifier's pass/fail verdict
cat tracker.py      # the patched code
```

IDs start at 1. `get_high_priority` filters out completed tasks. Empty lists return 0% completion. Bugs gone.

Now look at the operational picture:

```bash
owl runs list
```

```
RUN_ID       AGENT      STATE    STARTED              DURATION  MODEL
run_a1b2c3   scout      stopped  2026-04-09 14:01:22  48s       anthropic/claude-sonnet-4-6
run_d4e5f6   fixer      stopped  2026-04-09 14:02:14  1m 12s    anthropic/claude-sonnet-4-6
run_g7h8i9   verifier   stopped  2026-04-09 14:03:30  32s       anthropic/claude-sonnet-4-6
```

Every run is recorded and inspectable:

```bash
owl runs inspect run_a1b2c3       # full run details
owl logs show run_a1b2c3          # complete log history
owl logs query --agent scout --since 1h --json
```

---

## Step 7: The improvement loop

Owl doesn't just monitor runs — it learns from them. Check the scout's performance:

```bash
owl metrics show scout
```

```
Agent Metrics: scout
───────────────────────────────────────
  Total runs:    1
  Success rate:  100%
  Avg duration:  48s
  Avg tokens in: 2048
  Avg tokens out:1024
  Total cost:    ~$0.0089

  Prompt versions:
    a3f8c012  runs=1  success=100%
```

The run succeeded — but was the scout's output as useful as it could be? Ask Owl:

```bash
owl recommend --agent scout
```

Switch to the TUI. The meta-agent analyzes the scout's run data, logs, and output quality, then surfaces a recommendation:

> **Suggested improvement for scout (AGENTS.md):**
>
> Triage reports do not consistently include file paths and line numbers for each finding. The fixer agent spent additional effort locating the relevant code sections.
>
> **Proposed change:**
>
> ```diff
>  5. Write all findings to TRIAGE.md, sorted by severity.
> +
> + For each bug, always include:
> +   - The exact file path and line number
> +   - A one-line summary of expected vs. actual behavior
> ```
>
> *Apply this change? [suggest_edit proposed — review in TUI]*

The scout's definition never asked for precise source locations — and the metrics showed the downstream impact. Apply the suggestion, hatch the scout again on the same code, and the fixer resolves bugs faster with less token usage.

That's the loop: **define → run → measure → improve → define.**

---

## What you just built

Three agents with distinct identities and capabilities. They discovered each other on the Viche network, coordinated through real-time messages, and left a complete audit trail — runs, logs, metrics, and prompt version history.

```
owl project init --demo bug-hunt
        ↓
three agent definitions (.owl/agents/)
        ↓
owl hatch → registered on Viche → runs in owld
        ↓                              ↓
  viche_discover → viche_send    full logs + metrics
        ↓                              ↓
  next agent wakes up            owl recommend → better AGENTS.md
        ↓
  bugs found → fixed → verified
```

This is the pattern for any multi-agent system: define portable agent identities, hatch them into a network where they can discover and coordinate, monitor everything from a single TUI, and use structured metrics to improve your agents over time.

---

## Going further

**Define your own agents:**
```bash
owl agents show scout                  # study the definition
cp -r .owl/agents/scout .owl/agents/my-agent
# edit .owl/agents/my-agent/AGENTS.md
owl agents validate my-agent --strict
owl hatch --agent my-agent "Your task here"
```

**Promote agents you trust to every project:**
```bash
owl agents promote --agent scout       # project → global scope
```

**Import agents from elsewhere:**
```bash
owl agents import --path ./shared-agents/reviewer/ --scope project
```

**Make your registry private:**

Right now your agents are on the public Viche network. For team use — where only your agents can discover each other:

```bash
owl viche add-registry <your-token>
owl viche set-default <your-token>
```

[Create a private registry at viche.ai →](https://viche.ai/signup)

**Preview before hatching:**
```bash
owl hatch --agent scout --dry-run
```

**Full reference:** [README →](../README.md)
