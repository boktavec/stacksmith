# Feature spec: generator (Milestone 3)

Status: **Superseded** by `docs/spec/features/blueprint-catalog/SPEC.md` (approved 2026-10-13; YouTrack STACK-19). The Bucket contract, S3 generator and bundled S3 module described here are removed in that feature's Story 2a. This document is kept as a historical record only; it is not an active requirement.

Historical status: STACK-5 and STACK-6 approved and implemented; STACK-7 (section 3)
approved and implemented; STACK-8 (section 4) implemented.
Parent plan: `docs/stacksmith-core-plan.md`, sections 4-8 and section 9,
"Milestone 3 - generate the root we already understand."
Predecessor: `docs/spec/features/spec-parse-validate/SPEC.md` (Milestone 2, complete).
Tracking: YouTrack STACK-1 (milestone); stories STACK-5 to STACK-8, one PR each,
implemented in order.

This folder holds the contract for all of Milestone 3. STACK-5 to STACK-7 are
designed in detail. STACK-8 is approved for implementation.

## Story chain

| Story | Title | Status in this spec |
| --- | --- | --- |
| STACK-5 | M3.1: Resolve validated inputs into generator configuration | Designed (section 1) |
| STACK-6 | M3.2: Render deterministic Terraform root files | Designed (section 2) |
| STACK-7 | M3.3: Bundle the canonical S3 module in generated output | Approved (section 3) |
| STACK-8 | M3.4: Verify generator parity, determinism, and Terraform validity | Approved (section 4) |

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

## 3. STACK-7: bundle the canonical S3 module in generated output

Status: approved for implementation (STACK-7 only). Predecessor: STACK-6 (merged).

### 3.1 Goal

Make `Generate` return a self-contained deployment: the four root files from
STACK-6 followed by the four canonical S3 module files under `modules/s3/`,
so the root's `source = "./modules/s3"` resolves. The module bytes come from
the same `.tf` files the manual root and the Terraform tests use; there is no
second copy and no transformation.

### 3.2 Scope

- `core/render.go`: a `go:embed` directive naming the four production module
  files, an `embed.FS` variable, and an unexported `renderModule() []File`.
- `core/generate.go`: `Generate` returns `append(renderRoot(r), renderModule()...)`;
  update the doc comments on `Generate` and `File.Path` (slash-separated,
  relative, may contain directories).
- Tests in `core/render_test.go` (and one-line updates in
  `core/resolve_test.go`, see 3.8).
- No new dependencies (`embed` is standard library). No new package.

### 3.3 Non-goals

- CLI, writing files, creating directories, or any I/O (the future adapter
  creates `modules/s3/` when it writes `File.Path`).
- YAML/JSON parity, Terraform fmt/validate on the output, "module output
  identical to canonical" end-to-end checks beyond this slice (STACK-8).
- Changing any file under `core/modules/s3`, adding a header comment to module
  files, or a second module (only `s3-private/v1` exists).
- Module goldens under `core/testdata/`: the canonical source is the golden.

### 3.4 Repository facts

- `core/modules/s3/` is tracked and holds `main.tf`, `variables.tf`,
  `outputs.tf`, `versions.tf`, `tests/s3.tftest.hcl` and a tracked
  `.terraform.lock.hcl`. Only the four `.tf` files are production files.
- The module declares exactly the five variables the root passes
  (`bucket_name`, `name`, `environment`, `owner`,
  `noncurrent_version_expiration_days`) and outputs `bucket_name` and
  `bucket_arn`, which the root references.
- Plan section 7: "Go embeds those same `.tf` files from `core/modules/s3/`.
  Tests are not copied into generated deployments." Plan section 8 fixes the
  generated layout (`modules/s3/{main,variables,outputs,versions}.tf`) and
  says module files are copied unchanged. The root `main.tf` already emits
  `source = "./modules/s3"` (STACK-6).
- `render_test.go` already has `TestCoreNonTestFilesStayPure`, which forbids
  `os`, `net`, `os/exec` imports in non-test files. `embed` is not on that
  list and does no I/O at run time.
- Verified in a scratch copy (Go 1.27.0, outside the repo): an explicit
  `//go:embed modules/s3/main.tf modules/s3/variables.tf modules/s3/outputs.tf
  modules/s3/versions.tf` placed in a file in `core/` compiles; the resulting
  `embed.FS` holds exactly those four files at those paths; `ReadFile` bytes
  equal `os.ReadFile` of the source; a listed file that does not exist fails
  at compile time (`no matching files found`); a pattern containing `..` is
  rejected (`invalid pattern syntax`). Embed paths are relative to the
  directory of the Go file containing the directive, so the directive must
  live in `core/`, and the in-FS path equals the generated path.

### 3.5 Decisions (user)

1. The `go:embed` directive lives in `core` (in `core/render.go`) and lists the
   four files explicitly, not a glob, so adding a module file is a deliberate
   code change. No new package.
2. Byte-equality tests read the canonical files from disk with `os.ReadFile`
   in the test only; the core stays free of `os`.
3. `Generate` returns the four root files followed by the four module files in
   a fixed order.

Decisions below were recommended by the plan and approved by the user on 2026-10-13:

4. Embed into an `embed.FS` (not `[]byte` or `string` variables) and call
   `ReadFile` inside `renderModule`. `ReadFile` returns a fresh copy each
   call, so a caller that mutates `File.Content` cannot corrupt later calls.
   Listed files are guaranteed to exist at compile time, so a `ReadFile`
   error is a programming error and `renderModule` panics with a clear
   message rather than adding an error return to `Generate`.
5. Module files carry no StackSmith header (bytes unchanged). The root header
   already records generator version and module ID.
6. Include a drift-guard test (3.9) so a new file in `core/modules/s3` fails
   the test until someone decides to bundle or exclude it.

### 3.6 Design

Output order is fixed, 8 files total:

| # | Path | Source |
| --- | --- | --- |
| 1-4 | `versions.tf`, `providers.tf`, `main.tf`, `outputs.tf` | `renderRoot` (STACK-6, unchanged) |
| 5 | `modules/s3/main.tf` | embedded `modules/s3/main.tf` |
| 6 | `modules/s3/variables.tf` | embedded `modules/s3/variables.tf` |
| 7 | `modules/s3/outputs.tf` | embedded `modules/s3/outputs.tf` |
| 8 | `modules/s3/versions.tf` | embedded `modules/s3/versions.tf` |

Sketch (illustrative, final code is the implementer's):

```go
//go:embed modules/s3/main.tf modules/s3/variables.tf modules/s3/outputs.tf modules/s3/versions.tf
var moduleFS embed.FS

var moduleFiles = []string{"modules/s3/main.tf", "modules/s3/variables.tf",
    "modules/s3/outputs.tf", "modules/s3/versions.tf"}

func renderModule() []File { /* ReadFile each, in moduleFiles order */ }
```

Rules:

- The embed path, the `moduleFiles` entry and the generated `File.Path` are the
  same string. Paths use `/` (as `embed` does), never `filepath`.
- `moduleFiles` and the `go:embed` list must match. A test enforces that
  (3.9) so they cannot drift; if the implementer prefers one source of truth,
  deriving the list with `fs.ReadDir` on `moduleFS` is acceptable because the
  FS can only contain the four listed files, but the fixed order must still be
  explicit (`ReadDir` is sorted alphabetically, which would reorder
  `outputs.tf` before `variables.tf`), so the explicit slice is recommended.
- `renderModule` takes no arguments: module bytes do not depend on input.
  `.terraform.lock.hcl` and `tests/` are never named, so they cannot be
  embedded (and `go:embed` ignores dot-files in directory patterns anyway,
  which is not relied upon).
- No string handling of module content: bytes in, same bytes out.
- Core stays pure: `embed` only; no `os`, `net`, `os/exec`.

Alternatives rejected:

- `//go:embed modules/s3/*.tf` or the directory: would silently bundle any new
  file (contradicts decision 1); a directory pattern would also pull in
  `tests/`.
- A separate `modules` Go package exposing the FS: adds a package for no
  second consumer (user decision 1).
- `[]byte`/`string` embed variables: shared mutable backing array for `[]byte`;
  four variables instead of one FS.
- Rewriting or templating module text (for example to add a header): breaks
  "copy bytes unchanged" and the byte-equality guarantee.

### 3.7 Acceptance criteria

1. Only `main.tf`, `variables.tf`, `outputs.tf` and `versions.tf` from
   `core/modules/s3` are embedded, named explicitly in the directive.
2. For valid input, `Generate` returns 8 files: the four root files, then
   `modules/s3/main.tf`, `modules/s3/variables.tf`, `modules/s3/outputs.tf`,
   `modules/s3/versions.tf`, in that order, with no duplicate paths.
3. No generated file is `tests/`, `.terraform.lock.hcl`, or anything else from
   `core/modules/s3`.
4. Each module file's `Content` is byte-for-byte equal to its canonical source
   file, with no transformation.
5. Invalid input still returns `nil` files and the same diagnostics as
   `Validate` (module files are not returned for invalid input).
6. Output is deterministic: two `Generate` calls give equal paths and bytes,
   and mutating a returned `Content` does not change a later call.
7. `core` still has no `os`, `net`, `os/exec` imports in non-test files.
8. `go test ./...`, `go vet ./...`, `go build ./...` pass; `gofmt` is clean.

### 3.8 Existing tests that must change

Adding four files breaks these assumptions. Each is a deliberate edit, not a
weakened check:

| Test | Why it breaks | Change |
| --- | --- | --- |
| `TestGenerateGolden` (`render_test.go`) | iterates every generated file and reads `testdata/bucket/<Path>`; `modules/s3/*` goldens do not exist (and `-update` would write module copies into `testdata`) | compare only the four root files (skip paths under `modules/`); module files are covered by the new byte-equality test |
| `TestGenerateExamplesMatchGolden` | `len(files) != len(rootFileOrder)` fails at 8 vs 4; loop reads missing goldens | assert 8 files; compare root files to goldens; module files are covered by the new test |
| `TestGenerateFilesAndOrder` | requires the standard header on every file and `paths == rootFileOrder` | header check on root files only; assert full 8-path order |
| `TestGenerateAcceptsValidInput` in `resolve_test.go` (lines ~127-137) | `len(files) != len(rootFileOrder)` and per-index path check | compare against a new full-order list |
| `TestRenderRootDeterministic`, `TestRenderRootStructure`, `TestRenderRootEscapesHostileValues`, `TestGenerateInvalidInputYieldsNoFiles` | call `renderRoot` directly or assert `nil` | no change needed |

Suggested helper: keep `rootFileOrder`, add `moduleFileOrder` (the four
`modules/s3/...` paths) and `generatedFileOrder = root + module`.

### 3.9 Risks and security

- **Silent drift or accidental bundling.** A new file in `core/modules/s3` is
  not bundled unless named (by design). Mitigation: the drift-guard test fails
  when a regular file exists in `core/modules/s3` that is neither bundled nor
  in a small explicit exclusion list in the test (`tests/`,
  `.terraform.lock.hcl`), forcing a conscious decision.
- **Lock file or test leakage.** Excluded by naming; the test asserts no
  generated path contains `tests/` or ends in `.terraform.lock.hcl`, and that
  the set of generated module paths is exactly the four expected.
- **Module and root contract drift.** The root passes five inputs and
  references `bucket_name` and `bucket_arn`; the module declares and outputs
  them today. Nothing in this slice checks it automatically; STACK-8's
  `terraform validate` does. Do not add a Go-side check here.
- **Provider constraint compatibility.** Root pins `5.100.0`; module keeps
  `~> 5.0`, which 5.100.0 satisfies. Plan section 8: module bytes change only
  with a StackSmith release; bump `generatorVersion` by hand when they do.
- **Checkout transforms.** If a checkout converts line endings (for example
  Windows `autocrlf`), embedded bytes are the converted bytes and the test
  compares against the same checkout, so it still passes; the repo has no
  `.gitattributes`. Noted, not mitigated; out of scope.
- **Shared state.** Mutating a returned `Content` slice must not affect the
  embedded data or later calls; `embed.FS.ReadFile` copies, and a test proves
  it.
- **Panic path.** `renderModule` panics only if an embedded file is missing,
  which the compiler prevents. Not reachable from user input.
- **Security.** Module content is trusted repository source, not user input,
  so no escaping applies; user values still reach HCL only via STACK-6's typed
  APIs. Nothing here touches AWS, credentials, or the filesystem. Generated
  Terraform is still never applied by this work.

### 3.10 Testing

Use `.agents/skills/test-writing/SKILL.md`: real code paths, table-driven,
through `Generate` rather than only `renderModule`.

- **Byte-for-byte parity:** for each of the four expected module paths, find
  the file in `Generate` output and compare its `Content` with
  `os.ReadFile(filepath.Join("modules", "s3", name))` using `bytes.Equal`
  (path relative to `core/`, which is the test working directory). `os` is
  used in the test only.
- **Deterministic paths and order:** the full `Generate` path list equals
  `generatedFileOrder` for both YAML and JSON inputs; no duplicate paths.
- **Excluded files:** no generated path begins with `modules/s3/tests/` or
  ends with `.terraform.lock.hcl`; exactly four paths begin with `modules/s3/`.
- **Drift guard:** `os.ReadDir("modules/s3")`; every regular file must be in
  the bundled list or the explicit exclusion list; directories other than
  `tests` fail.
- **Determinism and isolation:** two `Generate` calls give equal results;
  zeroing the bytes of one call's module `Content` leaves a second call's
  content equal to the canonical file.
- **Invalid input:** existing `TestGenerateInvalidInputYieldsNoFiles` still
  passes (`nil` files, identical diagnostics).
- Update the tests listed in 3.8; confirm the new/changed tests fail before
  the implementation and pass after.
- Purity import check (`TestCoreNonTestFilesStayPure`) unchanged and passing.

### 3.11 Ordered implementation steps

1. Update the tests first (3.8, 3.10): new order constants, byte-equality,
   excluded-files and drift-guard tests; confirm they fail (8 files expected,
   4 returned).
2. Add the `go:embed` directive, `moduleFS`, `moduleFiles` and `renderModule`
   in `core/render.go`.
3. Wire `Generate` to append `renderModule()`; update the doc comments on
   `Generate` and `File.Path`.
4. `gofmt`, `go vet ./...`, `go build ./...`, `go test ./...`, import check.
5. Independent review (`staff-engineering-reviewer`) against this section.

### 3.12 Open questions

1. Confirm decisions 4-6 (embed into `embed.FS` with `ReadFile`; no header on
   module files; drift-guard test). Each is small and reversible.
2. Should the drift-guard test live in this slice or be left to STACK-8? It is
   cheap here, and it protects the "deliberate" intent of decision 1.

## 4. STACK-8: verify parity, determinism, and Terraform validity

Status: approved for implementation (STACK-8 only). Predecessor: STACK-7 (merged).

### 4.1 Goal

Close Milestone 3 by proving, end to end through `Generate`, that the 8-file
output is identical for equivalent YAML and JSON input, is stable across runs,
and is accepted by `terraform fmt -check` and `terraform validate` without AWS
access. Also run the module's native `terraform test` offline. Add a short
generator usage and learning note. No product behavior changes.

### 4.2 Scope

- `core/verify_test.go` (new): parity and full-output determinism tests.
- `core/terraform_test.go` (new): fmt, validate and module `terraform test`
  checks on the generated YAML-example output written to `t.TempDir()`, with
  explicit skip paths.
- `docs/learning/generator.md` (new): minimum `Generate` usage, Milestone 3
  teaching points, how to run the Terraform checks offline, what is not verified.
- Whitespace fix in `core/modules/s3/tests/s3.tftest.hcl` (alignment inside the
  `variables {}` block only). This is the only non-test, non-doc file touched,
  and it is not a generated or embedded file. Confirm the drift-guard and
  embedded-module tests still pass.
- Update this spec's status and approval log.
- No new Go dependencies. No change to `core` non-test Go files.

### 4.3 Non-goals

- CLI, writing files outside `t.TempDir()`, any adapter (Milestone 4).
- AWS access, credentials, `plan` against real AWS, `apply`, `destroy`, a backend,
  or network provider download. (`terraform test` here uses the module's
  `mock_provider "aws"` and `command = plan` only; see risks.)
- Changing the generator, the module's `.tf` files, or existing goldens.
- Re-testing what STACK-6/7 cover (canonical-module byte equality, drift guard,
  escaping, invalid-input-no-files, root structure).
- Generated root vs `examples/manual-root` diff test (decision 6).
- Makefile (decision 5).

### 4.4 Repository facts

- Already covered: invalid input yields nil files; module bytes equal canonical
  for both formats; module-dir drift guard; root and module determinism; escaping;
  root structure (region, allowed account IDs, module source, five arguments,
  output precondition).
- `TestGenerateGolden` compares both formats to one golden set, so parity is only
  transitive today; no test compares the two generated outputs directly.
- Terraform v1.15.7 at `/opt/homebrew/bin/terraform`; root requires `>= 1.7.0`.
  `fmt` is offline. `validate` and `test` need `init` and the AWS provider
  (exactly 5.100.0), even with `mock_provider`.
- No shared plugin cache or CLI config exists. A gitignored copy of provider
  5.100.0 exists under `examples/manual-root/.terraform/providers` (unpacked
  mirror layout); not present on a fresh clone or CI.
- `terraform fmt -check` is clean on generated root and module files; the only
  failing repo file is `s3.tftest.hcl` (fixed in this story).
- `core/modules/s3/tests/s3.tftest.hcl` uses `mock_provider "aws" {}` and
  `command = plan` runs.
- `TestCoreNonTestFilesStayPure` forbids `os`, `net`, `os/exec` in non-test core
  files; test files may use them.

### 4.5 Decisions (user)

1. Terraform checks live in Go tests (`core/terraform_test.go`), not a script.
2. Provider supplied via env var `STACKSMITH_TF_PLUGIN_DIR` pointing at an
   unpacked provider mirror. No auto-discovery of repo-local `.terraform`.
3. Only the YAML example is run through Terraform; JSON is covered by parity.
4. Missing Terraform/provider: skip with a message naming what was not verified;
   `STACKSMITH_REQUIRE_TERRAFORM=1` turns skips into failures.
5. The Terraform command is documented in the learning note only; no Makefile.
6. No generated-vs-manual-root diff test.
7. Learning note at `docs/learning/generator.md`.
8. In scope: fix `s3.tftest.hcl` whitespace and run `terraform test` on the
   module. (Planner recommended leaving both out; user chose to include them.)

### 4.6 Design

**Parity and determinism (pure Go, always run):**
1. `Generate(yaml)` and `Generate(json)` of the example inputs return the same
   paths, order and `bytes.Equal` contents for all 8 files.
2. `Generate` repeated several times on the same input returns identical output.

**Terraform checks (explicit skips):**
1. Generate from the YAML example; write all 8 files to `t.TempDir()` by `Path`.
2. `terraform fmt -check -recursive -diff` on that directory. Needs only the
   binary.
3. With a provider mirror: `terraform init -backend=false -input=false
   -plugin-dir=$STACKSMITH_TF_PLUGIN_DIR`, then `terraform validate`.
4. Module tests: from the temp dir's `modules/s3` copy, run `terraform init` with
   the same plugin dir and `terraform test`. The generated output does not
   include `tests/`, so the test copies `core/modules/s3/tests/` into the temp
   module copy (test files only, from the canonical source); the temp dir is
   discarded. Run it against a separate temp copy of the canonical module so the
   generated output tree stays exactly the 8 files that validate sees.
5. Subprocess environment: allowlist `PATH`, `HOME`, `TMPDIR`, plus
   `CHECKPOINT_DISABLE=1`, `TF_IN_AUTOMATION=1`, and `TF_CLI_CONFIG_FILE` set to a
   nonexistent path. No `AWS_*`. `exec.CommandContext` with timeout, fixed args,
   no shell. Only `version`, `fmt`, `init -backend=false`, `validate`, `test`
   are ever run.
6. Skip rules, each with `t.Skipf` naming what was not verified:
   `terraform` absent or older than `1.7.0` -> skip all; provider env var unset or
   mirror lacks `registry.terraform.io/hashicorp/aws/5.100.0/<os_arch>` -> skip
   validate and test (fmt still runs). `STACKSMITH_REQUIRE_TERRAFORM=1` converts
   every skip to a failure.

Rejected: plain `terraform init` (network, flaky); committing a provider or lock
file (binary, platform-specific, StackSmith must not fabricate locks); validating
in `examples/manual-root/.terraform` (wrong files, untracked state).

### 4.7 Acceptance criteria

1. A test proves YAML and JSON `Generate` output is identical in path, order and
   bytes for all 8 files.
2. A test proves repeated `Generate` calls give identical output for all 8 files.
3. `terraform fmt -check` passes on generated output when Terraform is present.
4. `terraform init -backend=false -plugin-dir` + `terraform validate` pass on
   generated output when the mirror is supplied, with no network or AWS access.
5. `terraform test` passes on the module when the mirror is supplied, and
   `terraform fmt -check` passes on `s3.tftest.hcl`.
6. Missing Terraform or mirror skips with a message naming what was not verified;
   `STACKSMITH_REQUIRE_TERRAFORM=1` makes it fail.
7. No Terraform subprocess has AWS credentials in its environment.
8. No file outside `t.TempDir()` is created or modified by tests; `git status`
   is clean after `go test ./...`.
9. `docs/learning/generator.md` exists.
10. `go test ./...`, `go vet ./...`, `go build ./...` pass; `gofmt` clean; core
    non-test Go files unchanged; embedded module tests still pass. The PR states
    which Terraform checks ran and which were skipped.

### 4.8 Risks and security

- False confidence from skips: verbose skip reasons, the require flag, and the PR
  stating actual results.
- Provider mirror trust: developer-supplied, hash not verified; no credentials
  present; `validate` and mocked `test` never call AWS. No auto-discovery outside
  the supplied path.
- `terraform test` scope creep: it must stay on `mock_provider` with
  `command = plan`. If any test block uses `apply` or a real provider, stop and
  return to the user. Verify before implementing.
- Subprocess surface: fixed args, cleaned env, timeouts, no shell.
- Network: `CHECKPOINT_DISABLE=1` and `-plugin-dir`; if init still tries the
  network, the test fails rather than falling back.
- Pure-library boundary: all side effects in `_test.go`; purity test guards it.
- Version drift: a mirror with another aws version fails init, correctly.
- Not covered: real AWS behavior, real `plan`/`apply`, state (Milestone 5); report
  as unverified, not passed.

### 4.9 Testing

Per `.agents/skills/test-writing/SKILL.md`: real `Generate` and real Terraform, no
mocks. Prove sensitivity by corrupting a generated byte / module path in an
uncommitted scratch run and recording the failure. Verify skip paths with
`PATH` lacking `terraform` and with the env var unset. Run with `-count=2`, and
`-race` if cheap.

### 4.10 Ordered implementation steps

1. Confirm `s3.tftest.hcl` uses only mocked plan runs; fix its whitespace; confirm
   drift-guard tests still pass.
2. Add parity and determinism tests; prove sensitivity.
3. Add Terraform helper (temp dir, env, skip logic) and the fmt test.
4. Add init/validate and `terraform test` checks; run against the local mirror
   under `examples/manual-root/.terraform/providers`, and with the env var unset.
5. Write `docs/learning/generator.md`.
6. gofmt, vet, build, test (with and without env vars), purity, clean `git status`.
7. Independent review with `staff-engineering-reviewer`; update approval log.

### 4.11 Walkthrough hand-off

After the PR, provide the AGENTS.md data-flow walkthrough, stating which
Terraform checks ran and which were skipped.

## 5. Approval log

- STACK-5 design approved by the user (including the `(nil, nil)` valid-input
  assumption).
- 2026-10-12: user approved this spec for STACK-5 implementation on branch
  `feature/generator` (off `origin/main`). STACK-6 to STACK-8 remain unplanned.
- 2026-10-12: user approved the STACK-6 spec (section 2) for implementation on
  branch `feature/STACK-6-render-root-files`. STACK-7 and STACK-8 remain unplanned.
- 2026-10-13: STACK-7 spec (section 3) drafted on branch
  `feature/STACK-7-bundle-s3-module` with user decisions on embed location and
  test approach. Status: draft, NOT approved; awaiting user review. STACK-8
  remains unplanned.
- 2026-10-13: user approved decisions 4-6 and the STACK-7 spec (section 3) for
  implementation on branch `feature/STACK-7-bundle-s3-module`, including the
  drift-guard test in this slice. STACK-8 remains unplanned.
- 2026-10-13: STACK-8 spec (section 4) drafted on branch
  `feature/STACK-8-verify-generator` with user decisions 1-8. Status: draft, NOT
  approved; awaiting user review.
- 2026-10-13: user approved the STACK-8 spec (section 4), decisions 1-8, for
  implementation on branch `feature/STACK-8-verify-generator`.
- 2026-10-13: STACK-8 implemented; independent review passed with no blocking
  findings, and three non-blocking findings were fixed.
