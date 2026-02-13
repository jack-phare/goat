# 16 — Skill Evaluation Framework

How skills are loaded, injected, and invoked during eval benchmarks,
and how the A/B comparison mode works.

## Skill System Overview

Goat's skill system mirrors Claude Code's three-tier progressive disclosure:

```
Level 1: Metadata in system prompt (~100 tokens/skill)
         "- go-expert: Go coding expert... When writing Go code"

Level 2: Full body returned on invocation (~2-6k tokens)
         LLM calls Skill tool → gets full SKILL.md body

Level 3: Bundled files/context (not yet implemented)
```

## Eval Binary Skill Pipeline

The eval binary (`cmd/eval/main.go`) wires skills when `-skills-dir` is provided:

```
cmd/eval/main.go
    │
    │  -skills-dir ./eval/skills
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│  1. LOAD                                                │
│                                                         │
│  prompt.NewSkillLoader(skillsDir, "")                   │
│      └─ scans {skillsDir}/.claude/skills/{name}/SKILL.md│
│      └─ parses YAML frontmatter + markdown body         │
│      └─ returns map[string]types.SkillEntry             │
│                                                         │
│  Files:                                                 │
│    pkg/prompt/skill_loader.go    — discovery             │
│    pkg/prompt/skill_frontmatter.go — YAML parsing        │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│  2. REGISTER                                            │
│                                                         │
│  prompt.NewSkillRegistry()                              │
│      └─ thread-safe map[string]SkillEntry               │
│      └─ FormatSkillsList() → bullet list for prompt     │
│      └─ satisfies agent.SkillProvider interface          │
│                                                         │
│  File: pkg/prompt/skill_registry.go                      │
└──────────────────────┬──────────────────────────────────┘
                       │
              ┌────────┴────────┐
              ▼                 ▼
┌──────────────────┐  ┌──────────────────────────────────┐
│  3a. SYSTEM      │  │  3b. TOOL                        │
│     PROMPT       │  │                                  │
│                  │  │  tools.SkillProviderAdapter       │
│  config.Skills = │  │      └─ bridges SkillRegistry    │
│    registry      │  │         to tools.SkillProvider   │
│                  │  │                                  │
│  Assembler sees  │  │  tools.SkillTool{                │
│  Skills != nil,  │  │    Skills:  adapter,             │
│  injects:        │  │    ArgSub:  prompt.SubstituteArgs│
│                  │  │  }                               │
│  "Available      │  │      └─ registered in Registry   │
│   skills:        │  │                                  │
│   - go-expert    │  │  File: pkg/tools/skilltool.go    │
│   - testing-..." │  │  File: pkg/tools/skill_adapter.go│
│                  │  │                                  │
│  File:           │  │                                  │
│  pkg/prompt/     │  │                                  │
│  assembler.go    │  │                                  │
│  :112-121        │  │                                  │
└──────────────────┘  └──────────────────────────────────┘
```

## Skill Invocation Flow

When the LLM sees skills listed in the system prompt and a relevant
user request, it emits a Skill tool call:

```
User: "Write a Go function with proper error handling"
                │
                ▼
┌───────────────────────────────────────┐
│  LLM sees system prompt:             │
│    "Available skills:                │
│     - go-expert: Go coding expert..."│
│                                      │
│  LLM emits:                          │
│    tool_use: Skill                   │
│    input: {skill: "go-expert"}       │
└───────────────┬───────────────────────┘
                │
                ▼
┌───────────────────────────────────────┐
│  SkillTool.Execute()                 │
│    1. adapter.GetSkillInfo("go-expert")
│    2. registry.GetSkill("go-expert") │
│    3. return ToolOutput{Content: body}│
│                                      │
│  Returns 3,797 chars of Go idioms,   │
│  error handling patterns, stdlib...  │
└───────────────┬───────────────────────┘
                │
                ▼
┌───────────────────────────────────────┐
│  LLM sees tool_result with full      │
│  skill body. Generates response      │
│  informed by Go expert knowledge.    │
└───────────────────────────────────────┘
```

## Eval Binary Usage

```bash
# Baseline (no skills)
goat-eval -prompt "Write a Go function..." -max-turns 10

# With skills
goat-eval -prompt "Write a Go function..." -max-turns 10 \
          -skills-dir ./eval/skills

# Skills dir layout (standard .claude convention):
eval/skills/
  .claude/skills/
    go-expert/SKILL.md        # Go idioms, error handling, stdlib
    project-context/SKILL.md  # Simulated project knowledge
    testing-patterns/SKILL.md # Table-driven tests, mocks, fixtures
```

## A/B Benchmark Mode

The Modal sandbox runner (`scripts/modal_sandbox.py`) supports A/B testing:

```
python scripts/modal_sandbox.py \
    --batch eval/benchmark_skills.json \
    --skills-dir eval/skills \
    --ab

                        ┌─────────────────────┐
                        │  benchmark_skills.json│
                        │  15 tasks:           │
                        │   5× go-expert       │
                        │   5× project-context │
                        │   5× testing-patterns│
                        └──────────┬──────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    │                             │
                    ▼                             ▼
          ┌─────────────────┐           ┌─────────────────┐
          │  Baseline Run   │           │  Skills Run     │
          │  (no skills)    │           │  (+skills)      │
          │                 │           │                 │
          │  15 sandboxes   │           │  15 sandboxes   │
          │  goat-eval      │           │  goat-eval      │
          │  -prompt "..."  │           │  -prompt "..."  │
          │                 │           │  -skills-dir    │
          │                 │           │    /opt/skills  │
          └────────┬────────┘           └────────┬────────┘
                   │                             │
                   └──────────────┬──────────────┘
                                  ▼
                        ┌─────────────────┐
                        │  summary.json   │
                        │                 │
                        │  baseline: 12/15│
                        │  +skills:  14/15│
                        │                 │
                        │  Per-task pairs: │
                        │  ┌─────────────┐│
                        │  │go-error:    ││
                        │  │ base: PASS  ││
                        │  │ skill: PASS ││
                        │  │ Δtime: -3s  ││
                        │  └─────────────┘│
                        └─────────────────┘
```

## Benchmark Task Categories

Each skill has 5 benchmark tasks across 3 categories:

```
┌──────────────────────────────────────────────────────┐
│  skill_relevant: true     │  Tasks where the skill   │
│  (3 per skill)            │  knowledge is directly   │
│                           │  applicable. Skills      │
│                           │  should measurably help. │
├───────────────────────────┼──────────────────────────┤
│  skill_relevant: false    │  Baseline tasks where    │
│  target_skill: same       │  skills shouldn't help   │
│  (1 per skill)            │  or hurt. Controls for   │
│                           │  skill overhead.         │
├───────────────────────────┼──────────────────────────┤
│  skill_relevant: false    │  Adjacent-domain tasks.  │
│  target_skill: same       │  Tests if skill knowledge│
│  (1 per skill)            │  generalizes.            │
└───────────────────────────┴──────────────────────────┘
```

## Test Skills

| Skill | Tokens | Tests |
|-------|--------|-------|
| `go-expert` | ~1k | Error wrapping, errgroup concurrency, functional options |
| `project-context` | ~1.1k | Add endpoint, add entity, write middleware |
| `testing-patterns` | ~1.7k | Table-driven tests, mock interfaces, httptest |

## Key Files

| File | Purpose |
|------|---------|
| `cmd/eval/main.go` | `-skills-dir` flag, skill loading, SkillTool registration |
| `cmd/eval/skills_integration_test.go` | Pipeline test: load → register → adapt → invoke |
| `cmd/eval/skills_e2e_test.go` | Live LLM test: secret-word skill invoked by gpt-4o-mini |
| `eval/skills/.claude/skills/*/SKILL.md` | 3 benchmark skills |
| `eval/benchmark_skills.json` | 15 skill-specific benchmark tasks |
| `scripts/modal_sandbox.py` | `--skills-dir`, `--ab` flags for A/B benchmarking |
| `pkg/prompt/skill_loader.go` | Filesystem skill discovery |
| `pkg/prompt/skill_registry.go` | Thread-safe skill storage |
| `pkg/tools/skilltool.go` | LLM-callable Skill tool |
| `pkg/tools/skill_adapter.go` | Bridges registry → tool interfaces |
