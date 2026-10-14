# StackSmith Core v0.1 — implementation plan

Status: approved. Amended 2026-10-13 for the approved catalog-backed blueprint
feature (`docs/spec/features/blueprint-catalog/SPEC.md`, YouTrack STACK-19). That
pivot replaces the original S3-bucket generator with catalog-backed `network`
blueprint generation.

The original S3 milestones (1 Terraform foundation, 2 parse/validate, 3 generator,
and the CLI `validate` command from milestone 4) are complete and remain in Git history. Their
Bucket-specific contract is superseded; see section 12. The feature spec is
authoritative for the details of the blueprint contract. This plan holds the
product-level design and the rules that still apply.

## 1. Goal and success criteria

StackSmith Core is a small, offline compiler from infrastructure intent and
platform standards to readable Terraform, using reviewed company blueprints:

```text
Blueprint spec + platform standards + embedded catalog snapshot
  -> strict parsing
  -> catalog lookup, schema and semantic validation
  -> merge the catalog tier preset with standards-approved overrides
  -> execute the reviewed embedded blueprint template
  -> return the Terraform root files + vendored blueprint module
```

The blueprints come from FoggyOps' `foggy-blueprints` repository: a catalog
(`catalog.yaml`), Go `text/template` root templates, and hand-authored Terraform
modules. StackSmith bundles a manually refreshed, pinned snapshot of it (commit
`ed819adb2c5c71790c41a0510f604b38f1c9a6ed`) and never fetches it at runtime.
The first and only generated blueprint is `network` (a VPC over
`terraform-aws-modules/vpc`). Every catalog entry is listed, and the CLI marks
which ones StackSmith can generate.

v0.1 succeeds when:

- `stacksmith catalog list` shows every snapshot entry and its support status, offline.
- Equivalent YAML and JSON inputs produce byte-identical named files using the
  same StackSmith build and bundled snapshot.
- Invalid or unsupported inputs explain what is wrong and why, and produce no output files.
- Generated `network` workspaces pass `terraform fmt -check` and `terraform validate`.
- Tests cover security invariants (override policy, template-injection safety,
  account guard), deterministic generation, and refusal to overwrite existing workspaces.
- Generation never accesses AWS. Any real AWS execution requires separate user
  authorization and credentials. Automated checks do not deploy resources.

## 2. Exact v0.1 scope

### Included

- Go core library and thin CLI with `catalog list`, `validate`, and `generate`.
- Strict YAML and JSON for both developer specs and platform standards.
- One generic `Blueprint` kind (`stacksmith/v2alpha1`) and one version-controlled
  `PlatformStandards` document.
- An embedded, pinned `foggy-blueprints` snapshot with a SHA-256 manifest. It holds
  the full catalog, and only the `network` template and module.
- One platform-selected AWS account and region (for example, `us-east-2`).
- Platform-owned allowed environments. Catalog tiers (`dev`, `prod`) are separate
  from environments.
- Tier presets from the catalog, plus typed, standards-approved per-tier parameter overrides.
- Generated workspace layout `roots/<name>/` + `modules/foggy-network/`.
- Focused automated checks and incremental Terraform learning documentation.

### Excluded

- AI, MCP, APIs, SaaS, multi-tenancy, frontend, GitHub, and Atlantis integrations.
- Terraform execution by StackSmith, AWS calls, credential handling, state
  management, drift detection, imports, deployment orchestration, and automated migrations.
- Generating any catalog entry other than `network`. Also excluded: multi-root dependency
  graphs, SSM producer/consumer orchestration, and cross-root state.
- Changes to `foggy-blueprints`, remote catalog fetching or sync, Git submodules,
  user-defined templates, arbitrary module sources, plugins, and general-purpose
  policy engines.
- Vendoring third-party Registry modules (`terraform-aws-modules/vpc/aws`) into output.
- Standards inheritance/profiles, per-environment accounts, multi-region provider
  aliases, custom tags, and escape hatches.
- Automatic workspace updates, overwrite/force options, backend generation,
  remote state setup, Terraform workspace management, or generated secrets.
- Compatibility with the old `stacksmith/v1alpha1` `Bucket` spec, or keeping the S3
  generator as a second path.

## 3. Language and dependencies

Use **Go**, as agreed. It supports a typed library and portable compiled CLI,
fits the user's Go learning goal, and has HashiCorp's native HCL tooling.

Use Go's standard library for JSON, file operations, embedding, and tests. Use the
standard library's `text/template` only to execute the trusted, embedded upstream
blueprint template unchanged. Developer values are template *data*, never template
source. Use a maintained YAML parser (`gopkg.in/yaml.v3`) and HashiCorp HCL v2
(`hclwrite`, with cty) where StackSmith writes its own HCL. Use `spf13/cobra` for
the CLI commands and flags (milestone 4 decision: the user chose it over stdlib `flag`). Do not add an AWS SDK,
third-party templating language, or policy framework. Confirm dependency versions
in the relevant story.

## 4. Input contracts

The exact contract is in the blueprint-catalog spec, section 4. In summary:

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
    az_count: "3"   # optional; strings only; must be allowed by standards
```

Each file describes exactly one blueprint instance. `metadata.name` is a
lowercase ASCII slug of at most 63 characters. It is used as the instance name, the root
directory `roots/<name>/`, and the SSM publish prefix `/foggy/network/<name>/`.
`spec.blueprint` must exist in the bundled catalog. Known entries other than
`network` fail as unsupported, and unknown IDs fail as unknown. `tier` must be a tier of
that catalog entry. `environment` must be allowed by standards; it is a tag value,
not a tier alias. There are no developer fields for module source, provider
version, account, region, tags, template path, or output path.

`apiVersion` versions StackSmith's input contract, not Terraform or AWS.

### Platform standards

```yaml
apiVersion: stacksmith/v2alpha1
kind: PlatformStandards
spec:
  aws:
    accountId: "123456789012" # Example only; replace before Terraform execution
    region: us-east-2
  environments: [dev, prod]
  blueprintOverrides:         # optional; absent = no overrides
    network:
      tiers:
        dev:
          parameters:
            az_count: {minimum: 2, maximum: 3}
```

The platform owns and reviews this document. StackSmith owns the parameter types
(`network`: `cidr` IPv4 CIDR, `az_count` integer, `single_nat` boolean). It also
enforces the module's own limits: `az_count` 1–8, and a CIDR from /16 to /24 in
canonical network form. Standards can only enable and constrain overrides per supported
blueprint, tier, and parameter, using `minimum`/`maximum` (`az_count` only), an
`allowed` list, or `{}` (any value within the limits). A parameter with no policy cannot be
overridden. Catalog presets are validated against the same schema.

### Strict parsing and validation

Both formats decode into the same Go types and use the same semantic validator.
Accept one document/object per file. Reject unknown and duplicate keys at every
level, trailing documents/data, wrong types, missing required fields, nulls in
place of required values, and unsupported kinds/versions. Do not silently coerce
types, drop fields, trim names, or truncate identifiers. Parameter values are
strings: `az_count: 3` (YAML integer) is a type error, so write `"3"`. Empty and
non-canonical values are rejected, never normalized.

Keep YAML JSON-compatible: string mapping keys; no aliases, merge keys, or custom
tags. Use parser facilities to detect duplicates and coercions. JSON duplicate-key
detection needs explicit handling. Bound input size before parsing (1 MiB per
document). The embedded catalog is trusted, reviewed content. It is decoded with
`yaml.v3` `KnownFields(true)` into structs, and its correctness is enforced by tests.

Semantic rules:

- Names and environment entries use lowercase ASCII slugs: letters/digits
  separated by single hyphens, starting and ending alphanumeric.
- Allowed environments form a nonempty list without duplicates; the selected
  environment must belong to it.
- Account ID is a string of exactly 12 ASCII digits, preserving leading zeros.
- Region must have valid AWS region-code syntax. Availability, opt-in status,
  AZ count, CIDR overlap, account existence, and permissions are not checked offline.

Diagnostics carry document identity, field path, reason, and a helpful correction;
include source location when available. Keep diagnostic ordering stable.

## 5. Standards, blueprints, validation, and output boundaries

| Layer | Responsibility | Not its responsibility |
| --- | --- | --- |
| Catalog snapshot (upstream, pinned) | Blueprint IDs, summaries, tier presets, provider constraints, template, module logic | StackSmith's input contract or policy |
| Platform standards | Account/region, allowed environments, per-tier override policy | Arbitrary executable policies |
| Terraform child module (`modules/foggy-network`) | AWS resources (VPC, subnets, NAT, flow logs, SSM publishes) | Credentials, provider configuration, backend/state ownership |
| Core validation | Strict contract, catalog lookup, typed parameters, policy enforcement, safe names/strings | Proving live AWS compliance or checking credentials |
| Generated root module | Configure the provider, call the module (wiring only), account guard, outputs | Reimplement the module's AWS resources |
| Terraform CLI/provider | Resolve data sources, plan/apply changes, track state, check the account | Decide StackSmith's developer-facing contract |

Template safety: the upstream `quote` helper is Go `%q`, not an HCL escaper, and
`az_count`/`single_nat` sit in unquoted HCL positions. Every string reaching the
template must therefore match `^[A-Za-z0-9./-]+$`. Validation guarantees this, and
the renderer re-checks it, failing closed.

Tags passed to the module are fixed by StackSmith: `managed-by=stacksmith`,
`blueprint=network`, `Environment=<environment>`. Unlike foggy-cli, which sets
`Environment` to the tier, StackSmith keeps environment and tier distinct.

## 6. Naming and account safeguards

The instance name is the developer's `metadata.name`. The VPC name and the SSM
paths derive from it in the module (`/foggy/network/<name>/<key>`). Renaming an
instance changes infrastructure identity; it is a migration, not a label edit.

The generated root configures the AWS provider with the standards region and
`allowed_account_ids` containing the expected account ID, so wrong-account
credentials fail during Terraform execution. This is the primary safeguard. As
defense in depth, `account_guard.tf` reads `aws_caller_identity` and puts a
precondition on an `aws_account_id` output. StackSmith emits these expressions
without accessing AWS.

An account ID is an identifier, not a credential. Credentials come from the
user's normal AWS authentication and are never generated or stored here.
Initial support targets the standard commercial AWS partition. Environments are
metadata, not account isolation.

## 7. Architecture and repository layout

Go packages: importable `core`, the CLI adapter `internal/cli` (its testable
`Run(args, stdout, stderr) int`), and a thin `main` package.

```text
cmd/stacksmith/main.go          # os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
internal/cli/                   # Arguments, bounded reads, safe output, exit codes
core/
  spec.go, standards.go         # v2alpha1 input types
  parse.go                      # Strict YAML/JSON decoding of user documents
  validate.go                   # Validation, typed parameters, override policy
  catalog.go                    # Embedded catalog decoding, supported table
  generate.go                   # Resolution, template execution, StackSmith-owned HCL
  snapshot/foggy-blueprints/
    MANIFEST                    # Upstream URL, commit SHA, per-file SHA-256
    catalog.yaml
    templates/network.tf.tmpl
    modules/network/{main,variables,outputs}.tf
examples/
  network.yaml, network.json, standards.yaml
docs/
  stacksmith-core-plan.md, spec/features/, learning/
```

Create files only as their story needs them. Embed explicit files, not broad globs.
Core exposes ordinary parse, validate, catalog-listing, and generation functions
operating on supplied bytes and typed values, returning structured diagnostics or
named file contents. There are no terminal, environment-variable, disk, network, or
subprocess dependencies in core (`go:embed` is compile-time). `Generate` always runs
validation, and invalid inputs return no files.

## 8. CLI and generated artifacts

```sh
stacksmith catalog list
stacksmith validate --spec examples/network.yaml --standards examples/standards.yaml
stacksmith generate --spec examples/network.yaml --standards examples/standards.yaml --out ./shared-network
```

Use explicit flags, no hidden config search or interactive prompts. Support
`.yaml`, `.yml`, and `.json`; report unsupported formats. Exit codes: 0 success, 1 for
invalid input, generation, or I/O failures, and 2 for command usage errors. `validate`
accepts `--format text|json` (presentation only). `catalog list` prints an aligned
text table on stdout, with no flags.

Generated workspace:

```text
shared-network/
  roots/shared-network/
    versions.tf        # required_version ">= 1.6"; aws from the catalog entry (~> 6.0)
    providers.tf       # region + allowed_account_ids
    main.tf            # upstream network template, executed unchanged
    account_guard.tf   # caller-identity output precondition
  modules/foggy-network/
    main.tf, variables.tf, outputs.tf   # copied byte-for-byte from the snapshot
```

The root calls `source = "../../modules/foggy-network"` and emits no resources of
its own. `main.tf` starts with the upstream template's own "GENERATED BY foggy"
header, because the template is executed unchanged. StackSmith-authored files
carry a StackSmith header with the generator version and snapshot commit.

Use deterministic file ordering and sorted map keys. Do not emit timestamps, random
suffixes, machine-specific paths, credentials, or lock files. Do not promise identical AWS
plans: credentials, state, resolved provider/module versions, and live
infrastructure affect Terraform independently of deterministic generation.

### Output safety and regeneration — agreed

Generate only into a new or empty directory. There is no overwrite or `--force` option.
Validate and render everything before writing. Use exclusive file creation,
reject unsafe/symlink destinations, and never overwrite state, lock files, or
existing content. Handle partial-write errors honestly: only clean up files and
directories created by the current run, never preexisting user files.

For changes to an existing deployment:

1. Generate into a temporary review directory.
2. Inspect the configuration and module diff.
3. Manually copy intended changes into the original workspace.
4. Run Terraform there, preserving its state and dependency lock file.

Never apply the review directory independently for the same instance, because two
states would compete for the same resources. Existing generated bucket workspaces
from the S3 era are user-owned; StackSmith does not touch, migrate, or reinterpret them.

### State and dependency versions

Use Terraform's default local state for the learning MVP, with one authoritative root
per instance. Ignore state/backups, `.terraform/`, saved plans, and real variable
files in this repository. Keep `.terraform.lock.hcl` trackable.

**Provider version change (amendment):** the S3-era plan pinned an exact AWS
provider version (`5.100.0`) in generated roots. Blueprint roots now use the
catalog entry's constraint, `~> 6.0`, and the vendored module requires the
Registry module `terraform-aws-modules/vpc/aws` `~> 6.0`. Identical generated
bytes therefore do not guarantee identical downloaded code. Users lock exact
provider versions with Terraform's `.terraform.lock.hcl` in their deployment
workspace, which StackSmith never fabricates. They should review Registry module
upgrades deliberately. `terraform init` and `validate` of generated output need
Terraform Registry access. Generation itself stays offline.

## 9. Milestones

### Completed (S3 era, superseded contract)

Milestone 1 (hand-written S3 module and root), milestone 2 (strict parse/validate),
milestone 3 (S3 generator, STACK-5..8), and the milestone 4 `validate` command
(STACK-9) are complete. Their parser, diagnostics, CLI loading/escaping/exit-code
rules, Terraform test harness, and safety lessons carry forward. The Bucket
contract and S3 module are removed by the blueprint feature's Story 2a.

### Milestone: catalog-backed blueprints (STACK-19)

Stories are defined in `docs/spec/features/blueprint-catalog/SPEC.md`, section 7.
Each is one PR, implemented in order:

1. **Story 1 (STACK-20):** pinned catalog snapshot, SHA-256 manifest, `catalog list`.
2. **Story 2a:** v2alpha1 Blueprint/standards with no overrides. Removes the whole
   bucket path and rewires `validate`.
3. **Story 2b:** typed parameter schema and the override policy.
4. **Story 3:** `Generate` for `network` (template execution, versions/providers/
   account guard, module vendoring, `terraform fmt` check).
5. **Story 4a:** `generate` command, new/empty destination checks, nested directories.
6. **Story 4b:** exclusive writes, symlink refusal, cleanup, sentinel tests.
7. **Story 5:** `terraform init -backend=false` + `validate` checks, and docs.

These replace the outlined CLI stories STACK-10..12.

Teach along the way: catalogs and pinned snapshots; Go `text/template` and why
template data differs from template source; root versus child modules and local
module sources; Registry modules and version constraints versus lock files; VPC
basics (subnets, AZs, NAT gateways, flow logs); SSM as a cross-stack contract.

### Deferred: real sandbox lifecycle

A real `network` deployment creates billable resources (NAT gateways, flow-log
storage). This is not part of the automated milestone. It needs separate user
authorization, credentials, a written plan, and a cleanup plan before any
`terraform plan`/`apply`. Until then, report real-AWS behavior as unverified.

### Close-out

Finish a short usage/learning guide, limitations, dependency/version notes, and a
security review. Re-run core, CLI, and Terraform checks. Do not disguise feature expansion as cleanup.

## 10. Decide now versus defer

**Decided now:** ownership boundaries, Go/`core` + `internal/cli`, strict versioned
inputs, pinned embedded snapshot with manifest, `network` as the only generated
blueprint, built-in parameter types with standards-gated overrides, the safe
charset rule for template data, account and region controls, deterministic readable
output, workspace layout, a thin CLI, and the no-overwrite/local-state workflow.

**Choose during implementation, before relying on them:** exact tested tool and
library versions, small Go API signatures, and the safe writer's details. These must
preserve this contract. Update this plan first if a material contract change is needed.

**Deliberately defer:** generating other catalog entries, multi-root/dependency
orchestration, catalog sync or remote fetching, a generic policy language,
independently versioned modules, remote state and team collaboration, real
environment/account isolation, IAM/RBAC enforcement, drift detection, adapters, and SaaS.

## 11. Challenges and limitations

1. **Avoid recreating Terraform.** Keep the small intent contract and leave AWS implementation in the upstream module.
2. **Local standards are not a security boundary.** A user who can change standards,
   generated Terraform, or AWS directly can bypass this path. IAM/SCPs are separate controls.
3. **Snapshot staleness.** The pinned snapshot can drift from upstream. Refresh is a
   deliberate, reviewed change (manifest SHA plus hashes, contract tests, output diff).
4. **Listed is not supported.** `catalog list` shows entries StackSmith cannot generate yet.
5. **Third-party dependency.** Output relies on Registry downloads for the VPC
   module and the provider. `~> 6.0` constraints are not immutable pins.
6. **Offline validation limits.** StackSmith cannot check CIDR overlap, AZ availability in
   the region, name uniqueness, permissions, or live compliance.
7. **Costs.** `network` creates NAT gateways and flow logs, which incur ongoing charges.
   `prod` uses one NAT gateway per AZ.
8. **Naming and state are long-lived contracts.** Renames and duplicate roots cause
   replacement or competing state. No automatic migration is included.
9. **Breaking change.** The old `Bucket` documents no longer validate. There is no compatibility mode.
10. **Manual updates and local state do not scale to teams.** These are intentional MVP limits.
11. **Hand edits are visible but not supported.** Altered Terraform falls outside StackSmith's contract.
12. **Two input formats add parser work.** Strictness and equivalence need real tests.

## 12. Approval and progress

The plan is approved. On 2026-10-13 the user approved the catalog-backed blueprint
feature (`docs/spec/features/blueprint-catalog/SPEC.md`), which supersedes the
S3-bucket contract. `docs/spec/features/generator/SPEC.md` and
`docs/spec/features/s3-terraform-foundation/SPEC.md` are marked superseded. In
`docs/spec/features/cli/SPEC.md`, STACK-9 is retained and STACK-10..12 are superseded.
Implementation proceeds story by story from Story 1 (STACK-20). No AWS operations have
been performed.
