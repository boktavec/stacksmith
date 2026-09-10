# Feature spec: S3 Terraform foundation (Milestone 1)

Status: draft, awaiting approval.
Parent plan: `docs/stacksmith-core-plan.md`, section 9, "Milestone 1 — understand
the actual Terraform." This is the only milestone with no Go code — it exists
to build and prove the canonical Terraform by hand before anything generates it.

## 1. Goal

Produce a hand-written, reusable Terraform child module implementing the
approved private-S3 contract, plus a hand-written example root that calls it,
plus a native Terraform test proving the module's invariants — all before
writing a single line of Go. The Go generator (milestone 3) will later embed
these exact `.tf` files unchanged.

## 2. In scope

- `core/modules/s3/`: `main.tf`, `variables.tf`, `outputs.tf`, `versions.tf`.
- `examples/manual-root/`: a hand-written root configuration that calls the
  module with literal example values (no StackSmith spec/standards parsing
  involved — that's milestone 2+).
- `core/modules/s3/tests/`: native Terraform test file(s) (`.tftest.hcl`)
  using `mock_provider`/`mock_resource` to check module behavior without
  touching AWS.
- Minimal docs: a short learning note on the HCL/Terraform concepts this
  milestone introduces (blocks, resources vs. data sources, provider
  inheritance, root vs. child modules, variables/outputs, dependency
  ordering) — added incrementally as each concept appears, not a single
  upfront essay.
- `.gitignore` rules for Terraform state, caches, saved plans (`*.tfplan`),
  and real `.tfvars`; keep `.terraform.lock.hcl` trackable.

## 3. Out of scope (deferred to later milestones or explicitly excluded)

- Any Go code, CLI, parsing, or validation (milestones 2-4).
- Real AWS credentials, `terraform init` against a live account, `plan`,
  `apply`, or `destroy` (milestone 5, requires separate explicit
  authorization).
- Anything the core plan already excludes: KMS, Object Lock, replication,
  IAM access grants, other infrastructure kinds, per-environment
  accounts/regions, arbitrary tags, escape hatches.

## 4. Module contract (fixed, from the plan — not re-litigated here)

Inputs: `bucket_name`, `name` (logical name, used for the `Name` tag),
`environment`, `owner`, `noncurrent_version_expiration_days`.

Outputs: `bucket_name`, `bucket_arn`.

Behavior, all with no override/toggle:

- All four S3 Block Public Access settings enabled.
- Bucket-owner-enforced ownership (ACLs disabled).
- Default encryption: SSE-S3 (not customer-managed KMS).
- Bucket policy denies non-TLS (`aws:SecureTransport = false`) requests.
- Versioning enabled.
- Lifecycle rule expires noncurrent versions after the input day count.
- `force_destroy = false`, hardcoded.
- Tags: `Name` (from `name` input), `Environment` (from `environment` input),
  `Owner` (from `owner` input), `ManagedBy = "stacksmith"` (fixed literal).

The module validates its own inputs (it can be called without StackSmith, via
the example root). Use separate `aws_s3_bucket*` resources for bucket,
public access block, ownership controls, encryption, versioning, lifecycle,
and policy — not one monolithic resource — and order the lifecycle
configuration after versioning.

## 5. Example root (`examples/manual-root/`)

Hand-written, literal example values (no bucket-name-collision-safe scheme
needed here — that's the naming convention in milestone 2/3). Purpose is to
prove the module composes correctly and to give the learning walkthrough
something concrete to `terraform validate`/`plan` against with a mocked or
absent provider. Calls the module via `source = "./modules/s3"`-equivalent
relative path from its own location, configures the `aws` provider block,
and exposes the module's outputs as root outputs.

## 6. Version pins

- **Terraform: `>= 1.7.0`.** Required for `mock_provider`/`mock_resource` in
  the native `terraform test` framework, so module invariants (tags,
  lifecycle, `force_destroy`, policy) can be checked without AWS access.
- **AWS provider: `~> 5.0`** in `versions.tf`. Exact tested patch version is
  confirmed when `terraform init` first runs and recorded via the resulting
  `.terraform.lock.hcl`, per the plan's "confirm dependency versions during
  the relevant milestone" note — not hand-picked in this spec.

## 7. Checks (acceptance criteria)

- `terraform fmt -check -recursive` passes across `core/modules/s3/` and
  `examples/manual-root/`.
- `terraform init` succeeds for both directories without a remote backend
  (provider download only; no state, no credentials required for this step).
- `terraform validate` passes for both directories.
- `terraform test` (native, mock provider) passes in
  `core/modules/s3/tests/`, covering:
  - all four required tags are set correctly from inputs/fixed literal,
  - `force_destroy` is `false` and not settable by the caller,
  - versioning is enabled,
  - the lifecycle rule expires noncurrent versions after the given day count,
  - public access block has all four settings enabled,
  - the bucket policy denies non-TLS requests.
- No AWS resources are created by any automated check in this milestone.
  Real-account behavior stays unverified until milestone 5.

## 8. Deliverable checklist (maps to Phase 3 "Todos" once this spec is approved)

1. `core/modules/s3/versions.tf` — Terraform/provider version constraints.
2. `core/modules/s3/variables.tf` — the five inputs, each with its own
   validation block where Terraform supports it.
3. `core/modules/s3/main.tf` — bucket + the six supporting resources.
4. `core/modules/s3/outputs.tf` — `bucket_name`, `bucket_arn`.
5. `core/modules/s3/tests/*.tftest.hcl` — mock-provider contract tests from
   section 7.
6. `examples/manual-root/` — provider config, module call, root outputs.
7. `.gitignore` entries for Terraform artifacts (if not already present).
8. Short learning note(s) under `docs/` introduced alongside the relevant
   files, per CLAUDE.md's "explain Terraform concepts as they arise" rule.

## 9. Open questions resolved during Phase 0

- Terraform floor version: **>= 1.7** (needed for mock-provider tests).
- AWS provider constraint: **~> 5.0**, exact patch confirmed at `init` time.

No other product decisions remain open for this milestone — the module
contract, tags, and safety invariants are already fixed by
`docs/stacksmith-core-plan.md` sections 4-6 and are not re-decided here.
