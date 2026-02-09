# AI Agent Instructions for KubeVirt Shepherd

## 🚨 MANDATORY PROTOCOL - VIOLATION IS FAILURE

### ⬇️ BEFORE Starting Any Task

**STEP 1**: Execute `/init` workflow (NOT just read - EXECUTE):

```
view_file: ai-code/.agent/workflows/init.md
```

**STEP 2**: Follow ALL steps in init.md, including:
- Read `skills.yaml` manifest
- Match task keywords to skills
- Load relevant SKILL.md files
- Output the confirmation block:

```
✅ Skills loaded: [list skills]
📋 Relevant ADRs: [list ADRs]
🚀 Starting task...
```

**⚠️ If you skip STEP 1-2, STOP and restart correctly.**

---

### ⬆️ AFTER Completing Any Task

**STEP 3**: Run `/code-review` workflow for any code changes:

```
view_file: ai-code/.agent/workflows/code-review.md
```

**STEP 4**: Create session notes following `continuous-learning` skill:

```
File: ai-code/.agent/sessions/{date}-{task}.md
```

**STEP 5**: Output completion block:

```
📝 Session saved: [session file path]
✅ Code review: [APPROVE/WARNING/BLOCK]
🏁 Task complete.
```

---

## Execution Checklist (MUST Complete)

```
At task START:
[ ] Read init.md workflow
[ ] Execute ALL init steps (not just understand them)
[ ] Output "Skills loaded" confirmation block

At task END:
[ ] Run code-review workflow (if code changed)
[ ] Save session notes
[ ] Output "Task complete" confirmation block
```

---

## Quick References

### Skills

| Skill | Path | Triggers |
|-------|------|----------|
| auto-context | `ai-code/.agent/skills/auto-context/SKILL.md` | Always read first |
| **api-contract-first** | `ai-code/.agent/skills/api-contract-first/SKILL.md` | **api, openapi, endpoint, handler, spec** |
| ent-patterns | `ai-code/.agent/skills/ent-patterns/SKILL.md` | ent, schema, database, transaction |
| river-patterns | `ai-code/.agent/skills/river-patterns/SKILL.md` | river, queue, job, async |
| schema-patterns | `ai-code/.agent/skills/schema-patterns/SKILL.md` | schema, cache, fallback, degradation, form |
| kubernetes-patterns | `ai-code/.agent/skills/kubernetes-patterns/SKILL.md` | k8s, kubevirt, vm, provider |
| golang-patterns | `ai-code/.agent/skills/golang-patterns/SKILL.md` | go code, test, interface |
| continuous-learning | `ai-code/.agent/skills/continuous-learning/SKILL.md` | After completing tasks |
| master-flow-context | `ai-code/.agent/skills/master-flow-context/SKILL.md` | implement, workflow, state machine |
| github-workflow | `ai-code/.agent/skills/github-workflow/SKILL.md` | commit, pr, issue, submit, github |

### Workflows

| Command | Purpose | When to Run |
|---------|---------|-------------|
| `/init` | Load task context | **ALWAYS FIRST** |
| `/code-review` | Security and quality review | **ALWAYS AFTER code changes** |
| `/adr-review` | Deep ADR review with evidence | **MANDATORY for ADR reviews** |
| `/adr-sync` | Sync ADR decisions to design docs | **MANDATORY after ADR acceptance** |
| `/tdd` | Test-driven development | When writing new features |
| `/ent-schema` | Ent schema design | When modifying database schemas |
| `/migration` | Atlas database migrations | When schema changes need migration |
| `/build-check` | Verify build and tests | Before committing |
| `/plan` | Task planning | For complex multi-step tasks |

---

## Critical Constraints (ADRs) - NEVER VIOLATE

| ADR | Rule |
|-----|------|
| ADR-0003 | Use Ent ORM only (no GORM) |
| ADR-0006 | All writes via River Queue |
| ADR-0012 | K8s calls outside DB transactions |
| ADR-0013 | Manual DI, no Wire/Dig |
| ADR-0015 | Entity decoupling (VM → Service only, no direct SystemID) |
| ADR-0016 | Use vanity import path: `kv-shepherd.io/shepherd/...` |
| ADR-0017 | User does NOT provide ClusterID; Namespace immutable after submission |
| ADR-0019 | RFC 1035 naming, least privilege RBAC, audit log redaction |
| **ADR-0021** | **API Contract-First: OpenAPI spec is single source of truth** |
| ADR-0028 | oapi-codegen with `omitzero`; Go 1.25+ required |

> 📚 **Complete List**: For all ADR constraints with CI enforcement details, see [CHECKLIST.md §Core ADR Constraints](docs/design/CHECKLIST.md#core-adr-constraints-single-reference-point).

---

## Core Principles

### Protocol Principles

1. **Execute, don't just read** - Workflows must be FOLLOWED, not just understood
2. **Confirmation blocks are mandatory** - They prove you followed the protocol
3. **ADRs are law** - Project-specific patterns override general knowledge
4. **Skills before assumptions** - Always check skill files before using general knowledge
5. **Session notes preserve learning** - Every significant task should have a record

### Project Priority Hierarchy (INVIOLABLE)

```
Stability > Consistency > Performance
```

| Priority | Description | Example |
|----------|-------------|---------|
| **1st: Stability** | System reliability is paramount | Never skip error handling for speed |
| **2nd: Consistency** | Data integrity must be maintained | Use transactions, never bypass |
| **3rd: Performance** | Concurrency is a bonus, not a goal | Throttling/queuing is acceptable |

### Review Standards

When reviewing documents, code, or ADRs:

- ✅ **Be neutral and strict** - No flattery or unnecessary encouragement
- ✅ **Propose alternatives** - If a better solution exists, state it clearly
- ✅ **Search for best practices** - Verify via web search when uncertain (year: 2026)
- ✅ **Check version compatibility** - Confirm library versions are current
- ✅ **Compliance alignment** - Follow cloud-native patterns (implicit, never explicit in docs)

### ADR Review Guidelines

| ADR Status | Expected Behavior |
|------------|-------------------|
| **Proposed** | Differences with existing docs are NORMAL; verify "Documents Requiring Updates" section |
| **Accepted** | Documents should be synchronized; check for missing sync items |

**⚠️ MANDATORY: For any ADR review task, you MUST use `/adr-review` workflow.**

The `/adr-review` workflow enforces:
- Evidence-based review (line numbers required for every claim)
- Cross-validation (Schema ↔ Code, Principles ↔ Implementation)
- Devil's Advocate mode (must find at least 3 potential issues)
- Process compliance (Review Period, Template, Single Decision)

**Quick "PASS" conclusions without evidence are FORBIDDEN.**

For Proposed ADR reviews, always check:
- [ ] "Documents Requiring Updates" section exists and is complete
- [ ] No conflicts with other Proposed ADRs
- [ ] Best practices verified via current documentation
- [ ] Review Period has NOT ended
- [ ] Internal consistency (tables match text, schema matches code)

**⚠️ MANDATORY: When syncing Accepted ADR to design docs, you MUST use `/adr-sync` workflow.**

The `/adr-sync` workflow enforces:
- Complete extraction of ALL items from "Documents Requiring Updates"
- **REMOVE verification**: grep to confirm deprecated content is gone
- ADD/REPLACE content verification with line numbers
- No partial syncs allowed

---

## Self-Check Questions

Before proceeding with any task, ask yourself:

1. "Did I output the '✅ Skills loaded' block?" → If no, go back to STEP 1
2. "Did I actually read the skill files, not just list them?" → If no, read them now
3. "Am I about to use general knowledge instead of project patterns?" → Check the skill first

After completing any task:

4. "Did I run /code-review?" → If code changed, do it now
5. "Did I save session notes?" → Create them now
6. "Did I output the '🏁 Task complete' block?" → Do it now
