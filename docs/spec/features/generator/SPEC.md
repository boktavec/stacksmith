# Feature spec: generator (Milestone 3)

Status: approved for implementation (STACK-5 only).
Parent plan: `docs/stacksmith-core-plan.md`, sections 4-8 and section 9,
"Milestone 3 - generate the root we already understand."
Predecessor: `docs/spec/features/spec-parse-validate/SPEC.md` (Milestone 2, complete).
Tracking: YouTrack STACK-1 (milestone); stories STACK-5 to STACK-8, one PR each,
implemented in order.

This folder holds the contract for all of Milestone 3. Only STACK-5 is designed
in detail. STACK-6 to STACK-8 are outlines copied from their YouTrack
descriptions and must be planned before they are implemented.

## Story chain

| Story | Title | Status in this spec |
| --- | --- | --- |
| STACK-5 | M3.1: Resolve validated inputs into generator configuration | Designed (section 1) |
| STACK-6 | M3.2: Render deterministic Terraform root files | Placeholder (section 2) |
| STACK-7 | M3.3: Bundle the canonical S3 module in generated output | Placeholder (section 3) |
| STACK-8 | M3.4: Verify generator parity, determinism, and Terraform validity | Placeholder (section 4) |

---

## 1. STACK-5: resolve validated inputs into generator configuration

### 1.1 Goal

Add an internal resolved configuration that combines a validated developer spec
with platform values, and a public generation entry point that always runs the
shared validation first. Invalid input returns diagnostics and no files. No
HCL is produced yet.

### 1.2 Scope

- `core/resolve.go`: unexported `resolved` struct and `resolve` function.
- `core/generate.go`: public `File` type and `Generate` function (a stub for
  valid input until STACK-6).
- Unit tests in `core/core_test.go` (or a new `core/resolve_test.go` if that
  keeps the diff readable).
- No new dependencies.

### 1.3 Non-goals

- HCL rendering (STACK-6).
- Embedding or copying the Terraform module (STACK-7).
- Parity, determinism and Terraform format/validate verification (STACK-8).
- CLI, file writing, or any I/O.
- Tags, `force_destroy`, or `ManagedBy` in Go. The Terraform module owns them
  (plan section 5).
- A public or persisted resolved-config format (plan section 7).
- Changes to existing parse or validation behavior.

### 1.4 Repository facts

- `core` already has `ParseSpec`, `ParseStandards`, `Validate(spec, standards)
  []Diagnostic`, `Diagnostic`, `Format`, and the unexported
  `composeBucketName(prefix, name, environment, accountID, region)`.
- No `File` type and no `generate.go` exist yet.
- The only approved module ID is `s3-private/v1`, enforced by `Validate`.
- Plan section 7: "Generation always runs shared validation; library callers
  cannot bypass it by forgetting to call `validate` first. Invalid inputs
  return no generated files."

### 1.5 Decisions (user)

1. Public entry point is `Generate(spec Spec, standards PlatformStandards)
   ([]File, []Diagnostic)`. It is a stub until STACK-6: valid input returns
   `(nil, nil)`.
2. `File` is minimal: `File{Path string; Content []byte}`.
3. `resolve` is unexported, does validation and resolution, and lives in
   `core/resolve.go` (this supersedes the plan's layout comment placing the
   resolved config in `validate.go`).
4. The resolved config holds raw values plus a derived `BucketName` computed
   once via `composeBucketName`. It does not hold tags, `ManagedBy`, or
   `force_destroy`.

### 1.6 Design

`resolved` (unexported) fields:

| Field | Source |
| --- | --- |
| `AccountID` | `standards.Spec.AWS.AccountID` |
| `Region` | `standards.Spec.AWS.Region` |
| `Module` | `standards.Spec.Bucket.Module` |
| `LogicalName` | `spec.Metadata.Name` |
| `Environment` | `spec.Spec.Environment` |
| `Owner` | `spec.Spec.Owner` |
| `NoncurrentVersionExpirationDays` | `standards.Spec.Bucket.NoncurrentVersionExpirationDays` |
| `BucketName` | `composeBucketName(prefix, name, environment, accountID, region)` |

`resolve(spec, standards) (resolved, []Diagnostic)`:

1. Call `Validate(spec, standards)`.
2. If any diagnostics: return the zero `resolved` and those diagnostics.
3. Otherwise copy the fields above and compute `BucketName`.
4. No re-validation of individual values; `Validate` is the sole gatekeeper.

`Generate(spec, standards) ([]File, []Diagnostic)`:

1. Call `resolve`.
2. On diagnostics: return `nil` files and the diagnostics.
3. On success: return `(nil, nil)`. A comment states that STACK-6 renders HCL
   from the resolved config and that this stub does not yet generate files.

The bypass guarantee: `Generate` is the only public generation path and it
always goes through `resolve`, which always calls `Validate`.

The core stays pure: no `os`, `net`, `os/exec`, or terminal imports.

Alternatives rejected:

- Unexported `resolve` only: does not meet the public entry point criterion.
- Raw-bytes `Generate(specData, standardsData, format)`: couples parsing to
  generation; the plan expects a future adapter to build typed inputs.
- Fully expanded config with tags and `force_destroy`: duplicates module
  invariants in Go and risks drift from the Terraform module.
- Raw values without a derived bucket name: the name could be computed in two
  places, and STACK-5 tests could not check it.

### 1.7 Acceptance criteria

1. An internal resolved configuration combines the spec with the platform
   account, region, name, module and expiration values (and owner,
   environment and derived bucket name).
2. A public generation entry point invokes shared validation, so callers
   cannot bypass it.
3. Invalid input returns diagnostics and no files (`nil` file slice).
4. `core` has no filesystem, network, terminal, or subprocess dependencies.
5. Unit tests cover the cases in section 1.9.
6. `go test ./...`, `go vet ./...`, and `go build ./...` pass; `gofmt` is clean.

### 1.8 Risks and security

- The `Generate` stub could be mistaken for finished generation. Mitigation: a
  doc comment says STACK-6 completes it.
- A zero-file valid result could mask a missing render step later.
  Mitigation: STACK-6 tests must assert non-empty, correct output.
- `resolve` trusts `Validate`. If a future validation rule is dropped, resolve
  will not catch it. Mitigation: tests exercise invalid inputs through
  `Generate` itself.
- `composeBucketName` stays in `validate.go` and is used by both files (same
  package, low risk).
- Security: the resolved values are still untrusted-origin strings. STACK-6
  must emit them only through HCL value APIs, never by string concatenation
  (plan section 7). Nothing in STACK-5 touches AWS, the filesystem, or
  credentials.

### 1.9 Testing

Use `.agents/skills/test-writing/SKILL.md`: real code paths, table-driven,
production-shaped inputs.

- Valid spec and standards resolve with every field carried through, including
  `Module` and `NoncurrentVersionExpirationDays`.
- `BucketName` is correct with and without a platform prefix, matching the
  plan example `acme-orders-assets-dev-123456789012-us-east-2`.
- Invalid inputs return diagnostics and `nil` files from `Generate`, covering
  at least: invalid spec field, invalid standards field, disallowed
  environment, and an invalid composed bucket name.
- `Generate` returns the same diagnostics as `Validate` for invalid input
  (proves validation is not bypassed).
- Valid input through `Generate` returns no diagnostics.
- YAML-parsed and JSON-parsed equivalent inputs resolve to identical values.
- Import check (grep or review) that `core` has no `os`, `net`, `os/exec` or
  terminal imports.

### 1.10 Ordered implementation steps

1. Add `core/resolve.go` with the `resolved` struct and `resolve`.
2. Add `core/generate.go` with `File` and the stub `Generate`.
3. Add the unit tests from section 1.9 and confirm each fails before the code
   exists, then passes.
4. Run `gofmt`, `go vet ./...`, `go build ./...`, `go test ./...`; confirm
   the import check.
5. Independent review (`staff-engineering-reviewer`) against this spec.

---

## 2. STACK-6: render deterministic Terraform root files

Status: approved for implementation (STACK-6 only). Predecessor: STACK-5 (merged).

### 2.1 Goal

Replace the STACK-5 stub so `Generate` returns the four Terraform root files
for valid input, built with HCL APIs and typed values. Invalid input still
returns diagnostics and no files.

### 2.2 Scope

- `core/render.go`: unexported `renderRoot(resolved) []File` using
  `hclwrite` and `cty` values; one small function per file.
- `core/generate.go`: `Generate` returns `renderRoot(r)` on success; add the
  unexported `generatorVersion` const.
- `core/render_test.go`: structure, escaping, determinism, golden output.
- `testdata/` golden files for the example inputs (readable review artifact).
- New dependency: `github.com/hashicorp/hcl/v2` (approved by plan section 3).

### 2.3 Non-goals

- Module embedding or `modules/s3/*` files (STACK-7). The root references
  `./modules/s3` but this slice does not emit it.
- Parity across YAML/JSON, `terraform fmt`/`validate` on a fixture (STACK-8).
- CLI, file writing, or any I/O. Tags, `force_destroy`, `ManagedBy` stay in the
  module.

### 2.4 Decisions (user)

1. **Account guard:** emit `data "aws_caller_identity" "current" {}` and a
   `precondition` on the `bucket_name` output asserting
   `data.aws_caller_identity.current.account_id` equals the expected account ID.
   No extra resource, no state. `allowed_account_ids` on the provider remains
   the primary guard.
2. **Provider pin:** root `versions.tf` pins `aws` to exactly `5.100.0` (the
   tested version); `required_version = ">= 1.7.0"`. The child module keeps
   `~> 5.0`.
3. **Generator identity:** unexported `generatorVersion = "0.1.0-dev"` const,
   bumped by hand at release; recorded with the module ID in a header comment.

### 2.5 Design

Output order is fixed: `versions.tf`, `providers.tf`, `main.tf`, `outputs.tf`.
Every file starts with the same header, no timestamps or paths:

```hcl
# Generated by StackSmith 0.1.0-dev (module s3-private/v1). Review before use.
# Do not apply a regenerated copy as a second state for the same bucket.
```

| File | Content |
| --- | --- |
| `versions.tf` | `terraform { required_version = ">= 1.7.0"; required_providers { aws = { source = "hashicorp/aws", version = "5.100.0" } } }` |
| `providers.tf` | `provider "aws" { region = <Region>, allowed_account_ids = [<AccountID>] }` and `data "aws_caller_identity" "current" {}` |
| `main.tf` | `module "bucket" { source = "./modules/s3", bucket_name, name, environment, owner, noncurrent_version_expiration_days }` from `resolved` only |
| `outputs.tf` | `bucket_arn` (value `module.bucket.bucket_arn`); `bucket_name` (value `module.bucket.bucket_name`) with a `precondition { condition = data.aws_caller_identity.current.account_id == <AccountID>, error_message = ... }` block directly inside the output (Terraform accepts `precondition` in `output`; `lifecycle` is for resources and data sources). Same names and descriptions as `examples/manual-root/outputs.tf` |

Rules:

- Strings go in via `cty.StringVal`; numbers via `cty.NumberIntVal`. Only
  references (`module.bucket.bucket_arn`, `data....account_id`) use traversals
  built with `hcl.Traversal`, never from input.
- Attribute order is set by the code, not by map iteration, so output is
  byte-stable. `hclwrite.Format` normalizes spacing.
- `resolved` already holds every needed value; no new `resolved` fields.
- Core stays pure (no `os`, `net`, `os/exec`).

### 2.6 Acceptance criteria

1. `Generate` returns exactly four files, in the order above, for valid input.
2. HCL is built with `hclwrite` and typed values; no input is concatenated into
   HCL source.
3. Provider has the standards region and `allowed_account_ids`; the
   caller-identity data source and output precondition guard the account.
4. The module call passes only the five resolved values, with `source =
   "./modules/s3"`.
5. Header comment records generator version and module ID; no timestamps or
   machine-specific data.
6. Invalid input still returns `nil` files plus the same diagnostics as
   `Validate`.
7. `go test ./...`, `go vet ./...`, `go build ./...` pass; `gofmt` is clean.

### 2.7 Risks and security

- **HCL injection:** an owner or name containing quotes, `${`, `%{`, newlines or
  backslashes must come out as a literal string. `hclwrite` escapes `${`/`%{`;
  tests must prove it (see 2.8).
- **Precondition is not a pre-apply guard** for the provider's own account
  check; it is defense in depth. The doc/header must not claim more.
- **Exact provider pin** means a provider bump is a deliberate code change;
  `terraform init` creates the lock file, which StackSmith never fabricates.
- **Drift from the hand-written root:** goldens are compared by eye against
  `examples/manual-root/`; STACK-8 adds the Terraform validate check.

### 2.8 Testing

Use `.agents/skills/test-writing/SKILL.md`: real code paths, table-driven.

- Golden files for `examples/` inputs (YAML parsed) for all four files.
- Determinism: render twice, bytes equal.
- Escaping: owner/name values with `"`, `\`, `${x}`, `%{x}` and a newline
  render as literal strings that parse back (via `hclparse`) to the original.
- Structure: parse the output with `hclsyntax` and assert the provider region,
  allowed accounts, module source, and the five arguments, not just text.
- `Generate` file count/order/paths; invalid inputs still yield `nil` and
  identical diagnostics; update `TestGenerateAcceptsValidInput`.
- Import check: no `os`, `net`, `os/exec` in `core`.

### 2.9 Ordered implementation steps

1. Add `hcl/v2` to `go.mod` (`go get`), confirm version via docs.
2. Write failing tests (goldens, escaping, determinism, structure).
3. Implement `render.go` and wire `Generate`; add `generatorVersion`.
4. `gofmt`, `go vet ./...`, `go build ./...`, `go test ./...`, import check.
5. Independent review (`staff-engineering-reviewer`) against this section.

## 3. STACK-7: bundle the canonical S3 module (placeholder)

Outline only, from YouTrack. Needs its own planning before implementation.
Predecessor: STACK-6.

- Embed only the canonical production module files from `core/modules/s3`.
- Place them under the documented generated path; omit tests, provider lock
  files and unrelated files.
- Copy bytes unchanged; no second implementation, no source transformation.
- Tests compare each generated module file byte-for-byte with its canonical
  source and verify deterministic paths.
- Non-goals: CLI, later slices.

## 4. STACK-8: verify parity, determinism, and Terraform validity (placeholder)

Outline only, from YouTrack. Needs its own planning before implementation.
Predecessor: STACK-7.

- End-to-end tests: equivalent YAML and JSON inputs give byte-identical named
  files; repeated generation is deterministic.
- Invalid inputs give diagnostics and zero files.
- Generated root is readable and passes Terraform format/validate using an
  offline fixture where possible; no AWS operations.
- Module output is byte-identical to the canonical module.
- Minimum generator usage and learning notes only.
- Run `go test ./...`, `go vet ./...`, `go build ./...` and relevant Terraform
  checks; record unavailable checks honestly.
- Non-goals: CLI, AWS access, deployment.

## 5. Approval log

- STACK-5 design approved by the user (including the `(nil, nil)` valid-input
  assumption).
- 2026-10-12: user approved this spec for STACK-5 implementation on branch
  `feature/generator` (off `origin/main`). STACK-6 to STACK-8 remain unplanned.
- 2026-10-12: user approved the STACK-6 spec (section 2) for implementation on
  branch `feature/STACK-6-render-root-files`. STACK-7 and STACK-8 remain unplanned.
