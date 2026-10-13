# Feature spec: catalog-backed blueprints

Status: **Draft for user review.** The user approved the design decisions in the planning interview; this document still needs review before implementation.
Repository: StackSmith only. `foggy-blueprints` is an upstream read-only source for this effort.
Upstream snapshot: `FoggyOps-LLC/foggy-blueprints` commit `ed819adb2c5c71790c41a0510f604b38f1c9a6ed`.

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
7. Tier values come from the catalog. A developer may override only parameters explicitly enabled for that blueprint and tier in platform standards. Standards specify parameter type and valid constraints; omitted policy means no override. Unknown parameter names and disallowed values are errors.
8. Execute the existing embedded Go `text/template` and implement only its required helpers. Embedded template code is trusted source; untrusted spec values are validated and passed as template data, never used as template source.
9. Match the blueprint workspace layout: `roots/<metadata.name>/` and `modules/foggy-network/`, so the network template's `../../modules/foggy-network` source resolves unchanged.
10. Preserve provider safeguards: standards region, provider `allowed_account_ids`, and caller-identity account check as defense in depth. Use provider constraints from the catalog (AWS `~> 6.0` for this snapshot).
11. Keep strict YAML and JSON support. Remove the old Bucket API, bucket generation, bundled S3 module, and bucket-only examples/tests/docs; retain shared parser, diagnostics, and safe CLI behavior where applicable.
12. The first release includes catalog listing, `validate`, and `generate` for `network`; validation rejects unsupported catalog entries with an explicit diagnostic.
13. Generated workspaces must pass `terraform fmt -check` and `terraform validate`. Validation may run `terraform init` to fetch the network module's Registry dependency and provider; generation itself stays offline. No plan, apply, destroy, AWS API calls, or AWS credentials are used.

## 3. Scope

### Included

- Replace `Bucket` developer and standards types with the versioned generic Blueprint contract.
- Strict parse and semantic validation for YAML/JSON spec and standards documents.
- A bundled catalog snapshot and explicit embedded-file list for the snapshot, network template, and network module.
- A read-only catalog listing that displays every snapshot entry's ID, summary, and supported/not-yet-supported status.
- Tier resolution, constrained override validation, and deterministic network root rendering from the existing Go template.
- Vendoring the network module into generated output at `modules/foggy-network/`.
- Provider versions, provider configuration, account safeguards, and output workspace layout.
- Adapt existing CLI validation/generation and safe destination writing to the new contract; add catalog listing.
- Tests, fixtures, Terraform formatting/validation checks, and concise usage/limitations documentation.
- Amend `docs/stacksmith-core-plan.md` and superseded feature specs before implementation so they agree with this contract.

### Excluded

- Any change to `foggy-blueprints`, its catalog schema, templates, modules, or release process.
- Generating EKS, RDS, DNS, bastion, Argo CD, s3-event-worker, or other catalog entries.
- Generating multiple blueprint roots or dependency graphs in one invocation; orchestrating SSM producer/consumer deployments; managing cross-root state.
- Remote catalog fetching, a sync command, Git submodules, automatic snapshot updates, or independently versioned module distribution.
- Importing the full upstream repository or vendoring third-party Terraform Registry modules into Stacksmith's generated output.
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
  parameters: {}
```

- One Blueprint instance per document and generated workspace.
- `metadata.name` is a lowercase ASCII slug. It becomes the template `.Name`, SSM publish prefix component, and root directory name; it is never treated as a filesystem path supplied without validation.
- `spec.blueprint` must exist in the bundled catalog. Known catalog entries other than `network` fail as unsupported in this Stacksmith version; unknown IDs fail as unknown.
- `environment` must be present in the platform standards list and is a tag value, not a tier alias.
- `tier` must name a tier in the selected catalog entry. For `network`, the snapshot provides `dev` and `prod` presets.
- `parameters` is optional and defaults to an empty map. Keys and values are strings. Unknown keys, unapproved overrides, or values outside the standards policy are rejected. An empty map uses the exact catalog tier preset.
- No developer-controlled module source, provider version, account, region, tags, template path, or filesystem output path is accepted in the spec.

### Platform standards document

```yaml
apiVersion: stacksmith/v2alpha1
kind: PlatformStandards
spec:
  aws:
    accountId: "123456789012"
    region: us-east-2
  environments: [dev, prod]
  blueprintOverrides:
    network:
      tiers:
        dev:
          parameters:
            az_count:
              type: integer
              minimum: 2
              maximum: 8
```

The example is illustrative only. The exact Go/YAML field names and validation messages are settled in the implementation story, but the contract is fixed:

- Account ID is exactly 12 ASCII digits; region follows AWS region-code syntax. Neither is supplied by the developer spec.
- Environments are nonempty, unique valid slugs. Both `dev` and `prod` are allowed by the example only; actual values are platform-owned.
- Override policy is keyed by supported blueprint, tier, and parameter. Each configured override declares its type and valid constraints. The initial network parameter schema covers `cidr` (IPv4 CIDR), `az_count` (integer), and `single_nat` (boolean); policy may constrain values/ranges. Do not accept policy keys for undeclared parameters.
- A parameter with no policy entry cannot be overridden. Do not silently fall back to module/template defaults when an override is malformed or denied.
- Strict YAML/JSON parsing rejects duplicate/unknown fields, invalid types, nulls for required fields, trailing documents/data, and oversized documents, retaining the existing parser's safe behavior.

### Tier and parameter resolution

1. Resolve the selected catalog entry and tier; fail before rendering if either is unknown or unsupported.
2. Copy the tier's catalog preset values.
3. For each developer parameter, require a matching known network parameter and an explicit standards policy for the selected tier; validate the type and constraints.
4. Merge only validated overrides onto the preset.
5. Canonicalize number and boolean values before rendering, so raw HCL template positions cannot receive expression syntax. Strings are emitted through the template's `quote` helper. Never splice unvalidated parameter text into a raw HCL position.
6. Build the template tags from fixed Stacksmith values: `managed-by=stacksmith`, `blueprint=network`, and `Environment=<validated environment>`.

Offline validation cannot prove CIDR overlap with other VPCs, available AZ count in the live region, account existence, permissions, name uniqueness, or live AWS compliance. Terraform validation is not a deployment check.

## 5. Catalog snapshot and generation contract

### Source and snapshot management

- Record upstream repository URL, full commit SHA, and refresh instructions in a small manifest/documentation entry.
- Snapshot refresh is a deliberate maintainer change: replace catalog and selected network assets, update the SHA, inspect the diff, run catalog/template/module contract tests, and review the generated Terraform diff.
- No runtime Git/network access in Stacksmith core or CLI.
- Embed explicit files, not a broad `go:embed` glob. Include the complete `catalog.yaml`, `templates/network.tf.tmpl`, and the production `.tf` files under `modules/network/`; exclude repository metadata, workflows, tests, lock files, and unrelated assets.
- Tests enforce that embedded paths are intentional and that the recorded snapshot and expected network template/module paths stay aligned. Catalog paths are trusted only after they resolve to the explicit bundled assets; traversal or missing-asset references fail closed.

### CLI catalog listing

`stacksmith catalog list` is read-only and offline. It prints all entries in stable order with the catalog summary and a clear `supported` / `not yet supported` marker. The only supported entry in this feature is `network`. No `catalog show`, remote lookup, or parameter-schema browser is included.

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

- The network template is embedded and executed with `.Name`, `.Type`, `.Tier`, `.Account`, `.AccountID`, `.Region`, `.Prefix`, `.Params`, and `.Tags` fields as required by the existing template contract. Implement only helpers used by the network template (`quote`, `param`, and `hcltags`); map output is sorted for deterministic HCL.
- `versions.tf` derives `required_providers` from the catalog AWS constraint and listed provider entries. `providers.tf` uses standards region, `allowed_account_ids`, and `aws_caller_identity.current`.
- `account_guard.tf` provides the existing caller-identity check as defense in depth without adding AWS resources or modifying the upstream template. The provider account allowlist remains the primary account boundary.
- The root module call remains wiring only and uses the template's local source `../../modules/foggy-network`. The module's own resource logic is copied unchanged from the pinned snapshot.
- Output ordering and file paths are deterministic. No timestamps, machine-specific paths, credentials, state, provider lock files, or Terraform cache are generated. Users may run Terraform init and commit the resulting provider lock file in their deployment workspace; Stacksmith does not fabricate one.
- Deterministic Stacksmith output does not pin the third-party VPC module to immutable bytes: its source uses the upstream `~> 6.0` constraint. Terraform resolves a compatible Registry release during init; review/lock provider selections and upstream changes deliberately.
- The generated workspace includes only Stacksmith-owned root files and the selected module files; it does not copy Terraform tests or unrelated upstream files.

## 6. Safety and compatibility

- Core remains pure: no filesystem, network, terminal, subprocess, credential, or AWS effects. CLI owns file reads/writes; test helpers own Terraform subprocess calls.
- Template source is trusted, version-controlled embedded content. Developer values are not parsed as Go templates. Validate every value, canonicalize raw scalar positions, use `quote` for strings, and escape terminal diagnostics as current CLI behavior requires.
- Preserve current CLI rules: explicit flags, bounded reads, no hidden config search/prompts, stable exit codes, and no-overwrite output behavior. Generation creates a new or empty destination only, validates/renders before writing, uses exclusive writes, rejects symlink/unsafe destinations, and never overwrites user content/state/lock files. Partial failure cleanup removes only files created by this run.
- No Terraform plan/apply/destroy and no AWS credentials in validation subprocess environments. `terraform init -backend=false` may access the Terraform Registry to download the AWS provider and the network module's `terraform-aws-modules/vpc/aws` dependency. This network access is test/validation-only; the Stacksmith build and generation path remain offline.
- Existing generated bucket directories are user-owned. The new generator does not delete, overwrite, migrate, or reinterpret them. The old bucket spec/generator is removed from Stacksmith source support; the migration consequence must be documented before implementation.
- Local platform standards are not a security boundary. Generated Terraform is reviewable configuration, not proof of permissions or compliance.

## 7. Ordered implementation stories

Every story gets a focused implementation context, builds on its predecessor, and has its own PR/review. Stories use YouTrack Type `Feature`; the parent milestone is this feature. No story authorizes AWS operations.

### Story 1 — Bundle the pinned catalog snapshot and list entries

**Goal:** Make the full catalog discoverable offline and establish a reviewed snapshot boundary.

**Scope:** Add the catalog YAML and snapshot provenance; parse the catalog strictly enough to reject malformed structure and unsafe/missing references; embed the catalog plus only the network template/module assets; add `stacksmith catalog list` with summary and support marker. Keep core free of I/O.

**Non-goals:** Spec migration, Terraform generation, fetching/syncing, full support for non-network entries.

**Acceptance criteria:**
- The exact upstream commit SHA is recorded and matches the checked-in source used for the snapshot.
- Catalog listing returns all snapshot entries in deterministic order, marks `network` supported and all other entries not yet supported, and performs no network/filesystem reads at runtime.
- A malformed catalog, unknown catalog fields (per the chosen strict snapshot parser), path traversal, or a missing network asset fails clearly; no arbitrary path is opened from catalog data.
- Only explicitly reviewed catalog/template/network module files are embedded; tests and unrelated repository assets are absent.
- Catalog parser/list tests cover real snapshot data, unknown/unsupported IDs, deterministic order, and malformed/path-invalid fixture cases.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.

### Story 2 — Replace Bucket inputs with strict v2alpha1 Blueprint contracts

**Goal:** Define and validate the new generic developer and platform documents, removing the obsolete Bucket contract.

**Predecessor:** Story 1 (catalog lookup and supported status).

**Scope:** Add `stacksmith/v2alpha1` `Blueprint` and `PlatformStandards` structures; strict YAML/JSON parse; validate names, blueprint identity/support, environment, tier, AWS account/region, parameter keys, per-tier standards override policy, and network parameter types/constraints; remove Bucket-only core types/validation/examples/tests/docs. Preserve reusable parser and diagnostics behavior. Update plan/spec references before implementing code.

**Non-goals:** Terraform rendering, CLI file writes, legacy compatibility, supporting non-network blueprints.

**Acceptance criteria:**
- Valid YAML and JSON Blueprint/standards documents produce the same typed values and diagnostics.
- Unknown blueprint IDs differ from catalog-known-but-unsupported IDs; only `network` proceeds.
- Unknown/duplicate fields, wrong types, null required values, invalid API version/kind, bad names/account/region/environment/tier, unknown parameter keys, denied overrides, malformed values, and policy typos are rejected with stable field-specific diagnostics.
- Tier presets remain unchanged when parameters are omitted; only explicit per-tier policy permits and constrains an override.
- Numeric and boolean parameters are parsed/canonicalized before they can reach unquoted template positions; hostile HCL/template strings are rejected or quoted safely.
- No compatibility acceptance of the old Bucket spec remains; shared strict parsing and CLI diagnostic behavior are retained.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.

### Story 3 — Resolve and render the `network` blueprint workspace

**Goal:** Produce deterministic Terraform files from a validated `network` spec using the pinned existing template and module.

**Predecessor:** Story 2 (validated typed inputs).

**Scope:** Resolve the catalog tier preset and approved overrides; render the embedded Go template with a fixed data context/helper set; generate provider/version/account guard files; return generated files at the `roots/<name>` and `modules/foggy-network` paths; embed/copy network module production files unchanged. Remove the S3 generator and embedded module.

**Non-goals:** CLI filesystem writing, non-network types, upstream template changes, module dependency vendoring, Terraform plan/apply/AWS calls.

**Acceptance criteria:**
- Public `Generate` always validates first and returns no files on any diagnostic.
- A valid `network` spec returns exactly the deterministic workspace paths, with module source resolving from `roots/<name>` to `modules/foggy-network`.
- The root uses catalog provider constraints, platform region/account safeguards, fixed Stacksmith tags, catalog tier values, and only permitted validated overrides.
- Template and module production files match their pinned upstream bytes; no resource logic is generated into the root and no test/lock/cache assets are copied.
- Repeated generation and equivalent YAML/JSON inputs produce byte-identical named files; map rendering is stable; malicious values cannot inject HCL or template code.
- Tests cover path safety/name validation, template data, merge/override behavior, account guard, exact embedded assets, determinism, and invalid-input-no-output through the public API.
- Core remains free of filesystem/network/terminal/subprocess effects; `go test ./...`, `go vet ./...`, and `go build ./...` pass.

### Story 4 — Adapt CLI validation, catalog listing, and safe generation

**Goal:** Expose the new Blueprint contract through the CLI while preserving existing safe I/O behavior.

**Predecessor:** Story 3 (validated library generator).

**Scope:** Update `validate` input/help/success behavior, retain `catalog list`, and update `generate` workspace output to include nested roots and vendored module paths. Keep explicit `--spec`, `--standards`, and `--out`; preserve bounded reads, exit codes, safe path checks, exclusive file writes, partial-write cleanup, and no-overwrite behavior.

**Non-goals:** Interactive configuration, remote catalog access, force/overwrite mode, state migration, deployment, Terraform commands from the CLI.

**Acceptance criteria:**
- CLI validates and generates the new v2alpha1 examples from YAML and JSON; bucket examples are removed/replaced.
- `catalog list` identifies supported and not-yet-supported catalog entries without contacting the network.
- Invalid/unsupported specs write no output; valid output has the documented directory tree.
- Existing output safety tests still prove no overwrite of sentinels/state/lock files, no symlink traversal, and safe partial-write cleanup.
- Usage, load, validation, and I/O errors retain documented streams and exit codes; diagnostics remain terminal-safe.
- `go test ./...`, `go vet ./...`, and `go build ./cmd/stacksmith` pass.

### Story 5 — Verify Terraform output and document the new workflow

**Goal:** Prove the generated workspace is Terraform-valid and communicate the new contract and limits.

**Predecessor:** Story 4 (CLI and generated workspace).

**Scope:** Update examples and user/learning docs; test deterministic generation and run `terraform fmt -check` plus `terraform init -backend=false` and `terraform validate` on generated output. Keep Terraform checks separate from ordinary Go tests if they need network/provider availability; report/skip unavailable prerequisites explicitly and provide a strict mode for CI to require them.

**Non-goals:** Terraform plan/apply/destroy, AWS access, live checks, remote state, automatic module download by Stacksmith.

**Acceptance criteria:**
- `terraform fmt -check` and `terraform validate` pass on the generated workspace when Terraform and Registry connectivity are available; `init` uses `-backend=false`, fixed arguments, a timeout, and a sanitized environment with no AWS credentials.
- Validation may download only declared Terraform Registry dependencies; it never contacts AWS, plans, applies, or destroys.
- Missing Terraform/Registry access is reported as unverified, not success; CI's strict mode fails when required checks cannot run.
- Examples show the separate environment and tier, allowed override policy, catalog listing, validation, and generation output layout.
- Docs explicitly state the manual snapshot refresh process, upstream pin, supported-vs-listed distinction, legacy Bucket-spec break, registry dependency, local standards limitation, and no-deployment boundary.
- `gofmt`, `go test ./...`, `go test -race ./...`, `go vet ./...`, `go build ./...`, and applicable Terraform checks pass; the final review reports every unavailable check.

## 8. Required documentation/contract updates before implementation

1. Amend `docs/stacksmith-core-plan.md` so its goal, scope, examples, input contracts, module/output layout, milestones, limitations, and approval/progress sections describe catalog-backed `Blueprint`/`network` generation instead of S3 bucket generation. Preserve still-valid pure-core, parser, CLI safety, and no-AWS rules.
2. Mark the bucket-specific generator feature spec as superseded by this approved feature after the new contract is accepted; do not leave contradictory active requirements.
3. Update CLI and examples specs where their Bucket-specific contracts conflict; retain safe file handling and presentation requirements.
4. Update any YouTrack milestone descriptions affected by the contract change; do not close existing issues or alter their status without separate explicit direction.

## 9. Risks and decisions to preserve

- **Snapshot staleness:** manual pinned snapshot can drift from upstream. The commit manifest and refresh checklist make drift visible; updates remain deliberate.
- **Template raw scalar positions:** `az_count` and `single_nat` are inserted without string quoting by the upstream template. Strict type validation and canonicalization are mandatory; do not pass arbitrary spec text through.
- **Offline boundary:** generation is offline, but Terraform validation requires Registry access for the AWS provider and upstream VPC module. This is disclosed and isolated from core/runtime generation.
- **Third-party module dependency/versioning:** the output vendors FoggyOps' network wrapper but not `terraform-aws-modules/vpc/aws`; users need Registry connectivity during Terraform initialization. The wrapper's `~> 6.0` constraint is not an immutable version pin, so identical generated bytes do not guarantee identical downloaded module code. Review dependency changes deliberately and use Terraform's provider lock file where applicable.
- **Account guard timing:** `allowed_account_ids` is the primary provider safeguard. The caller-identity check is defense in depth and must not be described as preventing every resource operation before provider configuration.
- **Unsafe name/path:** metadata name feeds a directory component and SSM path. Enforce slug grammar before output path construction.
- **Unsupported catalog entries:** listing availability does not imply validation/generation support. CLI and docs must label this clearly.
- **Removal of Bucket API:** existing input documents will no longer validate. Generated bucket workspaces remain user-owned and untouched; no migration or cleanup is performed.
- **No enforcement guarantee:** a user can alter local standards or generated Terraform; organization-wide IAM/SCP controls are outside this project.
