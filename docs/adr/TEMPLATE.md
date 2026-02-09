---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: YYYY-MM-DD
deciders: []  # GitHub usernames of decision makers
consulted: []  # Subject-matter experts consulted (two-way communication)
informed: []  # Stakeholders kept up-to-date (one-way communication)
---

# ADR-XXXX: [Short Title of Solved Problem and Solution]

> **Review Period**: Until YYYY-MM-DD (48-hour minimum)<br>
> **Discussion**: [Issue #XX](https://github.com/kv-shepherd/shepherd/issues/XX)<br>
> **Supersedes**: `ADR-XXXX-xxx.md` *(if applicable)*<br>
> **Amends**: `ADR-XXXX-xxx.md#section-anchor` *(if applicable)*

---

<!-- ⚠️ DELETE THIS GUIDELINES SECTION BEFORE SUBMITTING -->

## Writing Guidelines

| Principle | Description |
|-----------|-------------|
| **Single Decision** | One ADR = One architectural decision |
| **Concise** | Aim for 200-500 lines; split if exceeding 800 |
| **Immutable** | Once Accepted, never modify; use Amendments |
| **Value-Neutral Context** | State facts, not opinions, in Context section |

**Lifecycle**: `Draft → Proposed (48h review) → Accepted → [Deprecated | Superseded]`

---
<!-- END OF GUIDELINES SECTION -->

## Context and Problem Statement

<!-- Describe the context and problem in 2-3 sentences. Articulate as a question if helpful. -->

{What is the issue we are trying to solve? Why is this decision needed now?}

## Decision Drivers

<!-- Forces, concerns, or constraints that influence this decision -->

* {Driver 1: e.g., "Must support multi-cluster deployments"}
* {Driver 2: e.g., "Minimize operational complexity"}
* {Driver 3: e.g., "Align with KubeVirt upstream patterns"}

## Considered Options

* **Option 1**: {Title}
* **Option 2**: {Title}
* **Option 3**: {Title}

## Decision Outcome

**Chosen option**: "{Option X}", because {brief justification linking to decision drivers}.

### Consequences

* ✅ Good, because {positive outcome 1}
* ✅ Good, because {positive outcome 2}
* 🟡 Neutral, because {trade-off that neither helps nor hurts}
* ❌ Bad, because {negative outcome, with mitigation if any}

### Confirmation

<!-- How will we verify this decision is correctly implemented? -->

{Describe validation approach: code review checklist, automated tests, architecture fitness functions, etc.}

---

## Pros and Cons of the Options

### Option 1: {Title}

{Brief description or link to more details}

* ✅ Good, because {argument}
* ✅ Good, because {argument}
* 🟡 Neutral, because {argument}
* ❌ Bad, because {argument}

### Option 2: {Title}

{Brief description}

* ✅ Good, because {argument}
* ❌ Bad, because {argument}

### Option 3: {Title}

{Brief description}

* ✅ Good, because {argument}
* ❌ Bad, because {argument}

---

## More Information

<!-- Additional context, links to related decisions, implementation timeline, revisit criteria -->

### Related Decisions

* `ADR-XXXX-xxx.md` - {relationship description}

### References

* [Link description](https://example.com)
* Related Issue: [#XX](https://github.com/kv-shepherd/shepherd/issues/XX)

### Implementation Notes

<!-- Optional: Brief notes on implementation approach, timeline, or when to revisit -->

{When should this decision be revisited? What conditions would trigger reconsideration?}

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| YYYY-MM-DD | @username | Initial draft |

---

<!-- 
================================================================================
AMENDMENT TEMPLATE (For Accepted ADRs Only)
================================================================================
When amending an Accepted ADR, append this block to the END of the original ADR.
DO NOT modify the original content above.
================================================================================
-->

<!--
---

## Amendments by Subsequent ADRs

> ⚠️ **Notice**: The following sections have been amended by subsequent ADRs.
> The original decisions above remain **unchanged for historical reference**.

### ADR-XXXX: [Title] (YYYY-MM-DD)

| Original Section | Status | Amendment Details | See Also |
|------------------|--------|-------------------|----------|
| §X. [Section Name] | **MOVED** / **SUPERSEDED** / **CLARIFIED** | [Description] | `ADR-XXXX-xxx.md` |

---
-->
