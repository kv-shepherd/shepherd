# ADR-0051 Design Note: Scheduled Directory Enrichment

## Purpose

This note captures implementation-facing details for enriching existing Shepherd
users from an external directory source on a schedule.

The first expected deployment example is WeCom department sync used to enrich
users whose primary identity is established through LDAP or a legacy upstream
auth provider.

## Design Goals

* Keep JIT user-center construction as the default rule.
* Avoid full directory mirroring by default.
* Support richer profile and cohort data from external directory sources.
* Keep provider workflow at the edge and canonical writes in core.

## Default Mode

The default scheduled mode is:

* `enrich_existing_only`

Behavior:

1. fetch records from external directory
2. match to existing canonical users by explicit join key
3. update display-only projections and external cohorts
4. do not create missing canonical users by default

## Join Strategy

Required provider/admin config:

* `join_key_type`
  - `username`
  - `employee_id`
  - future explicit stable keys
* optional `source_attribute_path`

Default first implementation:

* `join_key_type = username`

The implementation must classify:

* matched existing user
* no match
* multiple matches / ambiguous match

Ambiguous results must not be auto-attached silently.

## Scheduled Job Inputs

Provider-owned request/schedule inputs may include:

* department selectors
* nested department inclusion
* selected fields
* sync cadence / cron
* manual trigger vs scheduled trigger
* cursor or delta token

These stay provider-local and must remain opaque to core.

## Core-Owned Write Model

Scheduled enrichment may write:

* `UserDirectoryProfile`
* normalized `ExternalCohort`
* sync job records and summaries

Scheduled enrichment must not directly write:

* runtime RBAC decisions
* approval policy
* core resource state

If cohort-based access is desired later, it must go through explicit
`ExternalCohort -> Shepherd RBAC` mapping and persisted Shepherd-managed RBAC
records.

## Suggested Provider Config Additions

For a directory-capable provider, schedule/enrichment config may include:

* `enrichment_enabled`
* `enrichment_mode`
* `schedule_cron`
* `schedule_timezone`
* `join_key_type`
* `department_selectors`
* `selected_profile_fields`
* `selected_cohort_kinds`
* `write_profile_fields`
* `write_cohorts`

These are provider-local admin settings, not core identity fields.

## WeCom First-Provider Guidance

For WeCom, the likely first implementation shape is:

* select one or more departments
* fetch users under those departments on a schedule
* match by `username`
* write:
  - phone
  - localized name
  - organization unit / department labels
  - avatar URL
  - department-based external cohorts

WeCom login capability may remain enabled or disabled independently of this
scheduled enrichment capability.

## UI Guidance

Admin UI should expose:

* current join rule
* current sync scope
* last run / next run
* matched / unmatched / ambiguous counts
* sample enriched fields

The UI should not imply that enrichment changes permissions directly.

## Testing Requirements

Minimum tests:

* default `enrich_existing_only` mode does not create missing canonical users
* explicit join-key matching updates projections for matched users
* unmatched records are reported but not auto-created in default mode
* ambiguous matches are classified and skipped
* synced cohorts do not directly affect runtime authorization

## Private Repository Support

Scheduled enrichment is expected to support enterprise-specific secondary
development in a separate private repository.

Recommended split:

### Public host repository

Owns:

* canonical enrichment semantics
* match classification rules
* projection/cohort persistence rules
* admin-visible result contract
* public enrichment SDK/common package if third-party or private adapters are supported

### Private integration repository

Owns:

* concrete directory adapters
* enterprise field mapping
* enterprise scheduling presets
* enterprise deployment wiring

It must not import Shepherd `internal/...` packages.

## Recommended Public SDK Surface

If enrichment adapters are implemented outside the host repository, the public
SDK surface should expose:

* directory sync capability interfaces
* canonical directory record/result DTOs
* public descriptor/request-schema DTOs

The host should not require private repositories to depend on internal core
services in order to implement enrichment adapters.

### Current repository implication

The current repository already exposes an initial public provider surface under:

* `pkg/authproviderplugin`

That package is the natural seed for a stable enrichment SDK. As private
repository support becomes formalized, the host should continue moving external
provider-facing contracts into that public surface rather than leaving them
defined only under `internal/...`.

### Current SDK gaps to close

To make separate-repository enrichment adapters practical, the public SDK
should be strengthened in these areas:

1. re-export provider-facing request-validation error types
2. re-export canonical conflict code constants, not only the alias type
3. keep descriptor, preview, and canonical record DTOs stable under `pkg/...`
4. document the public package path and capability-composition model for
   adapter authors

## Future Extension Points

Possible later extensions, still opt-in:

* `create_missing_users`
* manual review queue for ambiguous matches
* multiple join-key strategies with precedence
* more than one directory enrichment source per user
