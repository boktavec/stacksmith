# Feature spec: catalog-backed blueprints

Status: **Approved by the user (2026-10-13), including the section 10 defaults O1-O8.** Implementation proceeds story by story; approval to implement is not permission to publish or deploy.
Repository: StackSmith only. `foggy-blueprints` is an upstream read-only source for this effort.
Upstream snapshot: `FoggyOps-LLC/foggy-blueprints` commit `ed819adb2c5c71790c41a0510f604b38f1c9a6ed`.
Parent milestone: YouTrack STACK-19. Story 1 is STACK-20.

## 1. Goal

Replace StackSmith's bucket-only contract and generator with a catalog-backed blueprint flow that can list the company's catalog and validate/generate one supported blueprint (`network`) using the existing catalog, template, and module contracts.

The compiler remains offline at generation time and keeps its pure-core boundary:

```text
Blueprint spec + platform standards + embedded catalog snapshot
  -> strict parse
  -> catalog lookup + validation
  -> merge selected tier defaults with standards-approved overrides
  -> execute the reviewed embedded network template
  -> return Terraform workspace files + vendored network module
```

StackSmith generates configuration; it does not deploy infrastructure, access AWS, manage Terraform state, or provide an authorization boundary.

## 2. Approved decisions

1. This is one StackSmith feature, with no changes to `foggy-blueprints`.
2. Bundle a manually refreshed upstream snapshot and record its Git commit. Generation and catalog listing do not fetch the network.
3. Bundle the full upstream catalog for discovery, plus only the `network` template and module for generation.
4. Initially generate `network` only. The CLI catalog listing shows all catalog entries and clearly marks support status. A known but unsupported entry is distinct from an unknown identifier.
5. Keep the current platform-standards boundary for AWS account, region, and allowed environments. Standards also control tier-specific parameter override policy.
6. The developer spec uses generic `kind: Blueprint`, `apiVersion: stacksmith/v2alpha1`, `metadata.name`, `spec.blueprint`, `spec.environment`, `spec.tier`, and a generic string-to-string `spec.parameters` map. Tier is explicit and distinct from environment. The metadata name is also the blueprint instance/root name.
7. Tier values come from the catalog. A developer may override only parameters explicitly enabled for that blueprint and tier in platform standards. Omitted policy means no override. Unknown parameter names and disallowed values are errors.
8. Execute the existing embedded Go `text/template` and implement only its required helpers. Embedded template code is trusted source; untrusted spec values are validated and passed as template data, never used as template source.
9. Match the blueprint workspace layout: `roots/<metadata.name>/` and `modules/foggy-network/`, so the network template's `../../modules/foggy-network` source resolves unchanged.
10. Preserve provider safeguards: standards region, provider `allowed_account_ids`, and caller-identity account check as defense in depth. Use provider constraints from the catalog (AWS `~> 6.0` for this snapshot).
11. Keep strict YAML and JSON support. Remove the old Bucket API, bucket generation, bundled S3 module, and bucket-only examples/tests/docs; retain shared parser, diagnostics, and safe CLI behavior where applicable.
12. The first release includes catalog listing, `validate`, and `generate` for `network`; validation rejects unsupported catalog entries with an explicit diagnostic.
13. Generated workspaces must pass `terraform fmt -check` and `terraform validate`. Validation may run `terraform init` to fetch the network module's Registry dependency and provider; generation itself stays offline. No plan, apply, destroy, AWS API calls, or AWS credentials are used.

### Review-driven decisions (this revision)

14. **Parameter types are built into StackSmith, not declared by standards.** Standards only enable and constrain overrides (section 4).
15. **Module-intrinsic bounds are enforced regardless of policy:** `az_count` 1 to 8, IPv4 CIDR prefix /16 to /24 in canonical network form. These come from the module's `cidrsubnet(var.cidr, 4, i + 8)` subnet carving and AWS VPC limits.
16. **Catalog tier presets go through the same typed validation as overrides.** A preset that fails is a snapshot defect, caught by tests. It is never rendered.
17. **Empty parameter values are rejected.** Upstream `param` silently substitutes the template default for empty values, so an empty value must never reach it.
18. **Safe charset invariant for template data** (section 5). Upstream `quote` is Go `%q`, not an HCL escaper. It does not escape `${`/`%{` and can emit escapes HCL does not support.
19. **The `Environment` tag is the developer's environment, not the tier.** This intentionally differs from foggy-cli, which sets `Environment` to the tier (`blueprints.go:152`). StackSmith keeps environment and tier distinct (decision 6).
20. **Provider pin changes from exact to constraint.** The old plan pinned the exact AWS provider (`5.100.0`). Generated roots now use the catalog's per-entry `aws_provider_version` (`~> 6.0`). Users lock exact provider versions via Terraform's `.terraform.lock.hcl` in their deployment workspace.
21. **The supported set is hard-coded in StackSmith**, not inferred from catalog paths. Catalog `template`/`module` strings are only compared for equality with StackSmith's expected values for supported entries. They are never resolved as filesystem paths.
22. **The embedded catalog is trusted, reviewed content.** It is parsed with `yaml.v3` `KnownFields(true)` into Go structs, not with the user-input parser. A parse failure is a snapshot defect caught by tests, not a user-facing diagnostic path.

## 3. Scope

### Included

- Replace `Bucket` developer and standards types with the versioned generic Blueprint contract.
- Strict parse and semantic validation for YAML/JSON spec and standards documents.
- A bundled catalog snapshot with a SHA-256 manifest, plus the network template and module, embedded by explicit file list.
- A read-only catalog listing that displays every snapshot entry's ID, summary, and supported/not-yet-supported status.
- Tier resolution, constrained override validation, and deterministic network root rendering from the existing Go template.
- Vendoring the network module into generated output at `modules/foggy-network/`.
- Provider versions, provider configuration, account safeguards, and output workspace layout.
- Rewire the existing CLI `validate`, add `catalog list`, and build `generate` with safe destination writing (no `generate` command exists yet).
- Tests, fixtures, Terraform formatting/validation checks, and concise usage/limitations documentation.
- Amend `docs/stacksmith-core-plan.md` and superseded feature specs before implementation so they agree with this contract.

### Excluded

- Any change to `foggy-blueprints`, its catalog schema, templates, modules, or release process.
- Generating EKS, RDS, DNS, bastion, Argo CD, s3-event-worker, or other catalog entries.
- Generating multiple blueprint roots or dependency graphs in one invocation; orchestrating SSM producer/consumer deployments; managing cross-root state.
- Remote catalog fetching, a sync command, Git submodules, automatic snapshot updates, or independently versioned module distribution.
- Importing the full upstream repository or vendoring third-party Terraform Registry modules into StackSmith's generated output.
- A generic policy language, arbitrary module/template paths, plugins, or user-defined templates.
- AWS access, credentials, Terraform plan/apply/destroy, state/backend generation, drift detection, or automated migration of existing workspaces.
- A compatibility mode for the old Bucket spec or keeping the S3 generator as a second supported path.

## 4. Input contracts

### Developer Blueprint document

```yaml
apiVersion: stacksmith/v2alpha1
kind: Blueprint
metadata:
  name: shared-network
spec:
  blueprint: network
  environment: dev
  tier: dev
  parameters:
    az_count: "3"
```

- One Blueprint instance per document and generated workspace.
- `metadata.name` is a lowercase ASCII slug (letters/digits separated by single hyphens, starting and ending alphanumeric) of at most **63** characters. It becomes the template `.Name`, the SSM publish prefix component (`/foggy/network/<name>/...`), and the root directory name. Path use happens only after validation.
- `spec.blueprint` must exist in the bundled catalog. Known catalog entries other than `network` fail as unsupported in this StackSmith version. Unknown IDs fail as unknown. The two diagnostics are distinct.
- `environment` must be present in the platform standards list and is a tag value, not a tier alias.
- `tier` must name a tier in the selected catalog entry. For `network`, the snapshot provides `dev` and `prod` presets.
- `parameters` is optional and defaults to an empty map. Keys and values are **strings** in both YAML and JSON. Unquoted YAML numbers or booleans (`az_count: 3`) are type errors and must be written as `"3"`. This matches the catalog's string-typed presets. Empty values, unknown keys, unapproved overrides, and out-of-policy values are rejected. An empty or absent map uses the exact catalog tier preset.
- No developer-controlled module source, provider version, account, region, tags, template path, or filesystem output path is accepted in the spec.

### Built-in network parameter schema

StackSmith owns this schema. Standards cannot add or retype parameters.

| Parameter | Type | Canonical form (also required of presets) | Module-intrinsic bounds |
| --- | --- | --- | --- |
| `cidr` | IPv4 CIDR | Dotted-quad IPv4 prefix, no leading zeros, host bits zero (equals Go `netip.ParsePrefix(v).Masked().String()`) | Prefix length /16 to /24 |
| `az_count` | integer | ASCII decimal digits, no sign, no leading zero | 1 to 8 |
| `single_nat` | boolean | Exactly `true` or `false` | none |

Values that are valid but non-canonical (for example `010.0.0.0/16`, `10.0.0.1/16`, `03`, `True`) are rejected, never normalized. Offline checks cannot confirm that the region has `az_count` available AZs.

### Platform standards document

```yaml
apiVersion: stacksmith/v2alpha1
kind: PlatformStandards
spec:
  aws:
    accountId: "123456789012"
    region: us-east-2
  environments: [dev, prod]
  blueprintOverrides:          # optional; absent = no overrides anywhere
    network:
      tiers:
        dev:
          parameters:
            az_count:
              minimum: 2
              maximum: 3
            single_nat: {}     # any canonical boolean
            cidr:
              allowed: ["10.10.0.0/16", "10.20.0.0/16"]
```

Contract (the field names above are fixed):

- Account ID is exactly 12 ASCII digits. Region follows AWS region-code syntax. Neither is supplied by the developer spec.
- Environments are a nonempty list of unique valid slugs. Actual values are platform-owned.
- `blueprintOverrides` keys must be **supported** blueprint IDs (`network`). Unknown or unsupported IDs are errors. `tiers` keys must be tiers of that catalog entry. `parameters` keys must be in the built-in schema. Typos are errors, not ignored.
- Each parameter policy is an object with optional constraints:
  - `minimum`, `maximum`: YAML/JSON integers. Valid only for `az_count`. Each must lie within the module-intrinsic bounds, and `minimum <= maximum`.
  - `allowed`: a nonempty list of unique strings. Each entry must itself be a canonical, in-bounds value for the parameter's type.
  - `allowed` cannot be combined with `minimum`/`maximum`.
  - An empty policy `{}` permits any canonical value within the module-intrinsic bounds.
- A parameter with no policy entry for the selected tier cannot be overridden. There is no fallback to module/template defaults when an override is malformed or denied.
- Strict YAML/JSON parsing rejects duplicate/unknown fields, invalid types, nulls for required fields, trailing documents/data, and oversized documents, retaining the existing parser's safe behavior. The user-input parser (`core/parse.go`) must be extended to support dynamic-key maps (`spec.parameters`, `blueprintOverrides.<id>.tiers.<tier>.parameters.<name>`). Today it supports only fixed paths.

### Tier and parameter resolution

1. Resolve the selected catalog entry and tier. Fail before rendering if either is unknown or unsupported.
2. Copy the tier's catalog preset values. Each must pass the built-in schema's canonical-form and bounds checks (decision 16).
3. For each developer parameter: reject empty values, require a matching built-in parameter and an explicit standards policy for the selected tier, and validate canonical form, intrinsic bounds, and policy constraints.
4. Merge only validated overrides onto the preset. The merged map always contains all three network parameters with non-empty values, so `param` never falls back to its default.
5. Strings reach the template only under the safe charset invariant (section 5). Never splice unvalidated text into a raw HCL position.
6. Build the template tags from fixed StackSmith values: `managed-by=stacksmith`, `blueprint=network`, and `Environment=<validated environment>` (decision 19).

Offline validation cannot prove CIDR overlap with other VPCs, available AZ count in the live region, account existence, permissions, name uniqueness, or live AWS compliance. Terraform validation is not a deployment check.

## 5. Catalog snapshot and generation contract

### Source and snapshot management

- Snapshot files live under `core/snapshot/foggy-blueprints/`, mirroring upstream relative paths (`catalog.yaml`, later `templates/network.tf.tmpl` and `modules/network/*.tf`).
- `core/snapshot/foggy-blueprints/MANIFEST` records the upstream repository URL and full commit SHA. It then lists one `<sha256>  <path>` line per embedded file (`shasum -a 256` output format), so refreshing means copying the files and regenerating these lines.
- A test verifies that every embedded snapshot file has exactly one manifest line with a matching SHA-256, and that the manifest lists no extra files. Matching the bytes to the upstream commit is a manual refresh step documented in the manifest/README, because tests do not reach upstream.
- Snapshot refresh is a deliberate maintainer change: replace the catalog and selected network assets, update the SHA and hashes, inspect the diff, run catalog/template/module contract tests, and review the generated Terraform diff.
- No runtime Git/network access in StackSmith core or CLI.
- Embed explicit files, not a broad `go:embed` glob. Exclude repository metadata, workflows, tests, lock files, and unrelated assets. (Upstream `modules/network` has exactly `main.tf`, `variables.tf`, `outputs.tf`; its `terraform {}` block is in `main.tf`.)
- Supported entries are a hard-coded StackSmith table: `network` -> template `templates/network.tf.tmpl`, module `modules/network`. A test asserts that every supported ID exists in the catalog and that its catalog `template`/`module` strings equal the table values. Catalog paths of unsupported entries are never checked or opened.
- The catalog is decoded with `yaml.v3` `KnownFields(true)` into structs covering the snapshot's shape:
  - top-level `aws_provider_version` and `blueprints`
  - per entry: `summary`, `required`, `publishes`, `consumes`, `aws_provider_version`, `providers` (list of `{name, source, version}`), `tiers` (map of tier -> map of string -> string; may be empty, as for `dns`), `template`, `module`
  - `yaml.v3` already rejects duplicate keys.

### CLI catalog listing

`stacksmith catalog list` is read-only and offline. It takes no arguments or flags; extra arguments are a usage error (exit 2).

- **Output:** text only, on **stdout**. A header row then one row per entry, sorted by blueprint ID, columns aligned with `text/tabwriter`. Summaries go through the existing CLI control-character escaping.

  ```text
  BLUEPRINT        STATUS             SUMMARY
  network          supported          VPC across N AZs ...
  rds              not yet supported  PostgreSQL ...
  ```

- **Exit codes:** 0 on success. A stdout write failure, or an (unexpected) embedded-catalog load error, exits 1 with a message on stderr.
- **Not included:** `catalog show`, `--format json`, remote lookup, or a parameter-schema browser.

### Generated workspace

```text
<out>/
  roots/<metadata.name>/
    versions.tf
    providers.tf
    main.tf                 # Rendered from templates/network.tf.tmpl
    account_guard.tf        # Caller-identity defense-in-depth check
  modules/foggy-network/
    main.tf
    variables.tf
    outputs.tf
```

- **Template data.** The network template is executed with a struct containing only the fields StackSmith can source and the template uses:
  - `.Name`, `.Type` (`network`), `.Tier`, `.Params`, `.Tags`.
  - `.Account`, `.AccountID`, `.Region` and `.Prefix` from upstream `CONTRACT.md` are omitted. StackSmith has no source for `.Account` or `.Prefix`, and the network template does not reference any of them.
  - `text/template` fails on a missing struct field at execution time, and a test executes the real template, so template drift on refresh is caught.
- **Helpers.** Implement only `quote` (Go `%q`), `param` (lookup, default when missing or empty), and `hcltags` (sorted keys, `=`-aligned on `%q` key width, `fmt`-clean), matching foggy-cli's behavior.
- **Safe charset invariant.** Every string in the template data must match `^[A-Za-z0-9./-]+$`: `.Name`, `.Type`, `.Tier`, all `.Params` values, and all `.Tags` keys and values. Validation already guarantees this (slugs, catalog tier keys, canonical CIDR/integer/boolean, fixed tag keys). The renderer asserts it again before execution and fails closed (no files) if it is violated. Under this invariant, `%q` output is a valid HCL string literal without interpolation.
- **`versions.tf`:**
  - `required_version = ">= 1.6"`, matching the upstream module.
  - `required_providers` from the selected entry's **per-entry** `aws_provider_version` (`hashicorp/aws`), plus the entry's `providers` list (empty for `network`).
- **`providers.tf`:** `provider "aws"` with the standards region and `allowed_account_ids = ["<accountId>"]`. The provider allowlist is the primary account safeguard.
- **`account_guard.tf`** (defense in depth, no AWS resources, upstream template unmodified):

  ```hcl
  data "aws_caller_identity" "current" {}

  output "aws_account_id" {
    value = data.aws_caller_identity.current.account_id

    precondition {
      condition     = data.aws_caller_identity.current.account_id == "<accountId>"
      error_message = "..."
    }
  }
  ```

  This is an output precondition, like the current bucket root, not a `check` block (which only warns). The output name does not collide with the template's outputs (`vpc_id`, `private_subnet_ids`, `public_subnet_ids`).
- **Header comments.** StackSmith-authored files (`versions.tf`, `providers.tf`, `account_guard.tf`) carry a StackSmith header comment with the generator version and snapshot commit. Rendered `main.tf` starts with the upstream template's own header, `# GENERATED BY foggy — blueprint network ... Regenerated, do not edit.`, because the template is executed unchanged. Docs must explain this (see section 10).
- **Module call.** The root module call remains wiring only and uses the template's local source `../../modules/foggy-network`. The module files are copied byte-for-byte from the pinned snapshot.
- **Determinism.** Output ordering and file paths are deterministic. No timestamps, machine-specific paths, credentials, state, provider lock files, or Terraform cache are generated. Users may run Terraform init and commit the resulting provider lock file in their deployment workspace; StackSmith does not fabricate one.
- **Third-party module.** Deterministic StackSmith output does not pin the third-party VPC module to immutable bytes: its source uses the upstream `~> 6.0` constraint. Terraform resolves a compatible Registry release during init.
- **File set.** The generated workspace includes only StackSmith-owned root files and the selected module files.

## 6. Safety and compatibility

- **Pure core.** Core remains pure: no filesystem, network, terminal, subprocess, credential, or AWS effects (`go:embed` is compile-time). The CLI owns file reads/writes; test helpers own Terraform subprocess calls.
- **Template safety.** Template source is trusted, version-controlled embedded content. Developer values are never parsed as Go templates. Every value is validated and canonicalized, and the safe charset invariant holds before execution. Terminal diagnostics are escaped as current CLI behavior requires.
- **CLI rules.** Preserve current CLI rules: explicit flags, bounded reads, no hidden config search/prompts, stable exit codes (0/1/2).
- **Generation output.**
  - Generation creates a new or empty destination only and validates/renders before writing.
  - Writes are exclusive. Symlink/unsafe destinations are rejected.
  - User content, state and lock files are never overwritten.
  - Partial-failure cleanup removes only files and directories created by this run.
- **Terraform validation.**
  - No Terraform plan/apply/destroy, and no AWS credentials in validation subprocess environments.
  - `terraform init -backend=false` needs Terraform Registry access to download the AWS provider and the network module's `terraform-aws-modules/vpc/aws` dependency. The existing `STACKSMITH_TF_PLUGIN_DIR` provider mirror cannot supply a Registry module, so `terraform validate` of generated output always needs Registry access.
  - This network access is test/validation-only. The StackSmith build and generation path remain offline.
- **Legacy bucket output.** Existing generated bucket directories are user-owned. The new generator does not delete, overwrite, migrate, or reinterpret them. The old bucket spec/generator is removed from StackSmith source support, and the migration consequence is documented.
- **No enforcement.** Local platform standards are not a security boundary. Generated Terraform is reviewable configuration, not proof of permissions or compliance.

## 7. Ordered implementation stories

Every story gets a focused implementation context, builds on its predecessor, keeps `go build ./...`, `go vet ./...` and `go test ./...` green, and has its own PR/review. Stories use YouTrack Type `Feature`; the parent milestone is STACK-19. No story authorizes AWS operations. Stories 4a, 4b and 5 supersede the outlined CLI stories STACK-10, STACK-11 and STACK-12 (see section 8).

### Story 1 (STACK-20): bundle the pinned catalog snapshot and list entries

**Goal:** Make the full catalog discoverable offline and establish a reviewed snapshot boundary.

**Scope:**
- Add `core/snapshot/foggy-blueprints/catalog.yaml` and `MANIFEST` (URL, SHA, SHA-256 line).
- Embed both by explicit path.
- Decode the catalog with `yaml.v3` `KnownFields(true)` into structs.
- Add the hard-coded supported table (`network` with its expected template/module strings).
- Expose a core function returning entries (ID, summary, supported) sorted by ID.
- Add `stacksmith catalog list` per section 5, and update the root command's "a command is required" usage message.

**Non-goals:**
- Embedding the network template or module (Story 3).
- Spec/standards migration, Terraform generation, fetching/syncing, support for non-network entries.
- Generic catalog path resolution or traversal machinery.

**Acceptance criteria:**
- `MANIFEST` records `https://github.com/FoggyOps-LLC/foggy-blueprints` and `ed819adb2c5c71790c41a0510f604b38f1c9a6ed`. A test proves every embedded snapshot file matches its manifest SHA-256 and that the manifest has no extra or missing entries. The PR description records the manual byte comparison against the upstream commit.
- Decoding the real snapshot succeeds and yields all seven upstream entries (`argocd`, `bastion`, `dns`, `eks`, `network`, `rds`, `s3-event-worker`).
- Decoding fixtures with an unknown field, a duplicate key, or a wrong type fails.
- A test asserts each supported ID exists in the catalog with `template`/`module` equal to the table values.
- `catalog list` prints the header and all entries in ID order on stdout, marks only `network` `supported`, and exits 0. Extra arguments exit 2. It performs no runtime file or network reads (embedded data only).
- Only `catalog.yaml` and `MANIFEST` are added under the snapshot directory.
- Existing bucket `validate` behavior is unchanged.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.

### Story 2a: replace Bucket inputs with v2alpha1 Blueprint/standards (no overrides)

**Goal:** Define and validate the new documents, and remove the entire bucket path in one step so the build stays green.

**Predecessor:** Story 1 (catalog lookup and supported status).

**Scope:**
- Add `stacksmith/v2alpha1` `Blueprint` and `PlatformStandards` types and strict YAML/JSON schemas. `spec.parameters` may only be absent or `{}`; standards have no `blueprintOverrides` field yet, so it is rejected as unknown.
- Validate:
  - `metadata.name`: slug, at most 63 characters
  - blueprint: unknown vs. known-but-unsupported
  - environment and tier: from the catalog
  - account and region
- Delete the whole bucket path:
  - `core/generate.go`, `render.go`, `resolve.go` and their tests
  - `core/modules/s3/`
  - `core/testdata/bucket/`
  - `examples/bucket.*` and `examples/manual-root/`
- Keep the reusable Terraform test harness helpers (`skipOrFail`, `terraformEnv`, `terraformCmd`) but drop bucket-specific tests and the provider-mirror assumption.
- Replace `examples/standards.yaml` and add `examples/network.yaml` and `examples/network.json`.
- Rewire `stacksmith validate` (help text, flag descriptions) to the new types.
- Add a short "superseded" note at the top of `docs/learning/generator.md`; Story 5 rewrites it.

**Non-goals:** override policy, Terraform rendering, `generate`, legacy compatibility, non-network support.

**Acceptance criteria:**
- Valid YAML and JSON documents produce the same typed values and diagnostics.
- Unknown IDs and catalog-known-unsupported IDs produce distinct diagnostics; only `network` proceeds.
- The following are rejected with stable field-specific diagnostics: unknown/duplicate fields, wrong types, null required values, invalid API version/kind, bad name (including more than 63 characters), bad account/region/environment/tier, non-empty `spec.parameters`, and `blueprintOverrides` in standards.
- The old Bucket document is rejected (wrong apiVersion/kind). No bucket code, module, examples or testdata remain.
- `stacksmith validate` validates the new examples (YAML and JSON) with unchanged streams and exit codes.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.

### Story 2b: override policy and typed parameter validation

**Goal:** Allow standards-approved, typed parameter overrides.

**Predecessor:** Story 2a.

**Scope:**
- Extend the strict parser for dynamic-key maps (`spec.parameters`; `blueprintOverrides.<id>.tiers.<tier>.parameters.<name>`).
- Implement the built-in network parameter schema (canonical forms, intrinsic bounds), the policy vocabulary (`minimum`/`maximum`/`allowed`/`{}`), and validation of catalog presets against the schema.
- Reject empty values.
- Produce an internal resolved parameter map (preset merged with validated overrides).

**Non-goals:** rendering, CLI changes beyond what diagnostics need, a generic policy language.

**Acceptance criteria:**
- With no parameters, the resolved map equals the catalog preset exactly, for both `dev` and `prod`.
- Both snapshot presets pass schema validation. A test with a mutated preset fixture fails.
- Each of the following is rejected with a field-specific diagnostic:
  - Developer values: empty string, unknown key, no policy for the tier, out-of-policy value, non-canonical value (`010.0.0.0/16`, `10.0.0.1/16`, `03`, `True`), out-of-bounds value (`az_count` 0 or 9, `/15`, `/25`), unquoted YAML number/boolean.
  - Policy definitions: unsupported blueprint key, unknown tier, unknown parameter, `minimum`/`maximum` on non-integer, `minimum > maximum`, out-of-bounds bound, invalid or duplicate `allowed` entry, `allowed` combined with `minimum`/`maximum`.
- Hostile strings (`${...}`, `%{...}`, quotes, newlines, `\x00`) are rejected for every parameter.
- YAML and JSON forms are equivalent.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.

### Story 3: resolve and render the `network` workspace

**Goal:** Produce deterministic Terraform files from a validated `network` spec using the pinned template and module.

**Predecessor:** Story 2b.

**Scope:**
- Embed `templates/network.tf.tmpl` and `modules/network/{main,variables,outputs}.tf` by explicit path, and extend `MANIFEST`.
- Add public `Generate(spec, standards) ([]File, []Diagnostic)` with slash-separated relative paths per section 5.
- Execute the template with the minimal data struct and the three helpers.
- Assert the safe charset invariant.
- Render `versions.tf`, `providers.tf` and `account_guard.tf` per section 5.
- Copy the module files byte-for-byte.
- Add a `terraform fmt -check -recursive` test on the generated tree, written to a temp dir. It uses the existing skip-or-fail harness: it is skipped without Terraform and fails under `STACKSMITH_REQUIRE_TERRAFORM=1`. `fmt` needs no network.

**Non-goals:** CLI writing, non-network types, upstream template changes, Registry module vendoring, `terraform init`/`validate` (Story 5), plan/apply/AWS.

**Acceptance criteria:**
- `Generate` always validates first and returns no files on any diagnostic.
- A valid spec returns exactly the seven documented paths, with module source `../../modules/foggy-network` resolving from `roots/<name>/`.
- `versions.tf` has `required_version = ">= 1.6"` and AWS `~> 6.0` from the per-entry catalog value.
- `providers.tf` has the standards region and `allowed_account_ids`.
- `account_guard.tf` has the data source plus the `aws_account_id` output precondition with the standards account ID.
- Tags are exactly `Environment=<environment>`, `blueprint=network`, `managed-by=stacksmith`.
- Rendered `main.tf` contains the catalog tier values or permitted overrides.
- Module files match their manifest SHA-256. A test executes the real template, so a missing template field or a helper change fails.
- A renderer test proves an invariant violation (injected via internal data) yields no files.
- Repeated generation, and YAML vs. JSON inputs, produce byte-identical files.
- `terraform fmt -check` passes on the output when Terraform is available.
- Core remains free of filesystem/network/terminal/subprocess effects. `go test ./...`, `go vet ./...`, and `go build ./...` pass.

### Story 4a: add `generate` with destination checks and nested directories

**Goal:** Expose generation through the CLI.

**Predecessor:** Story 3.

**Scope:**
- Add `stacksmith generate --spec <file> --standards <file> --out <dir>` (all required; reuse `loadInput`). Validate and render fully before touching the filesystem.
- `--out` must either not exist (its parent must exist) or be an existing empty directory.
- Create `roots/<name>/` and `modules/foggy-network/` and write the files.
- On diagnostics, use the same text presentation as `validate` and write nothing.

**Non-goals:** exclusive-create hardening, symlink refusal, partial-failure cleanup (4b); `--force`, `--format json` for generate, Terraform commands, remote catalog access.

**Acceptance criteria:**
- Generates the documented tree from the YAML and JSON examples.
- Invalid/unsupported specs exit 1 and leave `--out` absent or empty.
- A non-empty or non-directory `--out` exits 1 without writing.
- Missing flags exit 2.
- Streams and exit codes match `validate`'s documented rules.
- `go test ./...`, `go vet ./...`, and `go build ./cmd/stacksmith` pass.

### Story 4b: harden output writes

**Goal:** Make generation writes exclusive and failure-safe.

**Predecessor:** Story 4a.

**Scope:**
- Use exclusive file creation (`O_CREATE|O_EXCL`).
- Use `Lstat` checks, refusing a symlinked `--out` and symlinked intermediate directories.
- Clean up on partial failure, removing only files and directories created by this run.
- Add sentinel tests.

**Non-goals:** overwrite/force mode, state migration, deployment.

**Acceptance criteria:**
- Tests prove no overwrite of pre-existing sentinel files, including `terraform.tfstate` and `.terraform.lock.hcl`.
- Tests prove refusal of a symlinked `--out` and of symlinks inside the destination.
- An injected write failure leaves only pre-existing content.
- Diagnostics remain terminal-safe.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./cmd/stacksmith` pass.

### Story 5: verify Terraform output and document the workflow

**Goal:** Prove the generated workspace is Terraform-valid and communicate the new contract and limits.

**Predecessor:** Story 4b.

**Scope:**
- Run `terraform init -backend=false -input=false` and `terraform validate` on a generated workspace, reusing the skip-or-fail harness. This requires Registry access.
- Rewrite `docs/learning/generator.md` and the README usage.
- Finalize the examples.

**Non-goals:** Terraform plan/apply/destroy, AWS access, live checks, remote state, automatic module download by StackSmith.

**Acceptance criteria:**
- `terraform fmt -check` and `terraform validate` pass on the generated workspace when Terraform and Registry connectivity are available.
- `init` uses `-backend=false`, fixed arguments, a timeout, and the sanitized environment with no AWS credentials. Validation downloads only declared Registry dependencies.
- Missing Terraform or Registry access is reported as NOT VERIFIED, not success. `STACKSMITH_REQUIRE_TERRAFORM=1` makes it fail.
- Docs explicitly state:
  - the manual snapshot refresh process (MANIFEST)
  - the upstream pin
  - the supported-vs-listed distinction
  - the legacy Bucket-spec break
  - the Registry dependency and the `~> 6.0` constraints, with lock-file advice
  - the foggy `GENERATED` header in `main.tf`
  - the Environment-tag difference from foggy-cli
  - the local standards limitation
  - the no-deployment boundary
- `gofmt`, `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, and applicable Terraform checks pass. The final review reports every unavailable check.

## 8. Required documentation/contract updates before implementation

1. Amend `docs/stacksmith-core-plan.md` so its goal, scope, examples, input contracts, module/output layout, milestones, limitations, and approval/progress sections describe catalog-backed `Blueprint`/`network` generation instead of S3 bucket generation.
   - Call out explicitly the change from an exact AWS provider pin (`5.100.0`, plan section 8) to the catalog's `~> 6.0` constraint plus user-owned lock files.
   - Call out the use of stdlib `text/template` for the trusted upstream template.
   - Preserve still-valid pure-core, parser, CLI safety, and no-AWS rules.
2. Mark the bucket-specific generator feature spec (`docs/spec/features/generator/SPEC.md`) and `docs/spec/features/s3-terraform-foundation/SPEC.md` as superseded by this feature once it is approved; do not leave contradictory active requirements.
3. Update `docs/spec/features/cli/SPEC.md`:
   - STACK-9 (`validate`) stays as implemented and is rewired in Story 2a.
   - STACK-10, STACK-11 and STACK-12 are superseded by Stories 4a, 4b and 5 here.
   - Retain the safe file-handling and presentation requirements.
4. Update affected YouTrack descriptions:
   - STACK-19/20.
   - New Feature issues for Stories 2a, 2b, 3, 4a, 4b and 5.
   - A note on STACK-10..12 that they are superseded.
   - Do not close existing issues or alter their status without separate explicit direction.

## 9. Risks and decisions to preserve

- **Snapshot staleness:** a manual pinned snapshot can drift from upstream. The MANIFEST (SHA plus per-file SHA-256) and the refresh checklist make drift visible; updates remain deliberate.
- **Template raw scalar positions:** `az_count` and `single_nat` are inserted unquoted, and `quote` is not an HCL escaper. Canonical typed values and the safe charset invariant are mandatory.
- **`param` default fallback:** empty values would silently become template defaults. The merged map must be complete, and empty values are rejected.
- **Offline boundary:** generation is offline, but Terraform validation requires Registry access for the AWS provider and the upstream VPC module.
- **Third-party module dependency/versioning:** the output vendors FoggyOps' network wrapper but not `terraform-aws-modules/vpc/aws`. The `~> 6.0` constraints are not immutable pins; identical generated bytes do not guarantee identical downloaded code.
- **Account guard timing:** `allowed_account_ids` is the primary safeguard. The output precondition is defense in depth and does not prevent every resource operation.
- **Unsafe name/path:** the metadata name feeds a directory component and an SSM path. Slug grammar and the 63-character limit are enforced before path construction.
- **Unsupported catalog entries:** listing does not imply support. The CLI and docs label this clearly.
- **Removal of Bucket API:** existing input documents no longer validate. Generated bucket workspaces remain user-owned and untouched.
- **Intermediate story states:** after 2a and before 4a, StackSmith has `validate` and `catalog list` but no generation. This is acceptable for unreleased intermediate PRs.
- **No enforcement guarantee:** a user can alter local standards or generated Terraform. Organization-wide IAM/SCP controls are outside this project.

## 10. Open decisions for user review

The planner chose these defaults. Confirm or change each before implementation.

| # | Decision | Default chosen | Alternative(s) and tradeoff |
| --- | --- | --- | --- |
| O1 | `metadata.name` max length | 63 characters | Shorter (e.g. 32) keeps AWS Name tags/SSM paths compact, but is arbitrary; longer is unnecessary. |
| O2 | Root `required_version` | `">= 1.6"` (matches upstream module) | Keep the old `">= 1.7.0"`, which was only needed for native mock-provider tests that no longer exist. |
| O3 | Empty override policy `{}` | Permits any canonical value within intrinsic bounds | Require at least one constraint (stricter, more verbose standards). |
| O4 | `cidr` constraint form | `allowed` enum only | Add a "within supernet" constraint (more flexible, more code); defer until needed. |
| O5 | `main.tf` header | Keep the upstream "GENERATED BY foggy" header unchanged, and document it | Prepend a StackSmith comment line before template output: clearer provenance, but `main.tf` is no longer pure template output. |
| O6 | `catalog list` format | Aligned text table on stdout, no `--format` | Add `--format json` like `validate` (more code, no current consumer). |
| O7 | Account guard output name | `aws_account_id` | Any non-colliding name; it appears in `terraform output`. |
| O8 | Story 4a write semantics | Ordinary create after the new/empty check; exclusive create and cleanup land in 4b | Use `O_EXCL` already in 4a (tiny change that narrows the 4a/4b gap). |
