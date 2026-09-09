# StackSmith Core v0.1 — implementation plan

Status: consolidated design, awaiting final approval. No implementation authorized.
Branch: `planning/stacksmith-core`.

Product choices below were agreed through one-question-at-a-time discussion.
The detailed implementation contracts, checks, and milestone sequence are the
proposed synthesis for final review. Implement incrementally after approval,
explaining Terraform and architecture as each milestone is introduced.

## 1. Goal and success criteria

StackSmith Core is a small, offline compiler from infrastructure intent and
platform standards to readable Terraform using an approved module:

```text
Developer spec + platform standards
  -> strict parsing
  -> schema and semantic validation
  -> resolve platform values and permitted developer inputs
  -> select the approved module
  -> generate Terraform root configuration + copy the module
```

The first infrastructure type is one private AWS S3 bucket per specification.
Start with a hand-written Terraform module and root configuration before writing
the Go generator. S3 teaches resources, modules, providers, state, and lifecycle
without first requiring a VPC, subnets, or a database's ongoing costs.

v0.1 succeeds when:

- Equivalent YAML and JSON inputs produce byte-identical named files using the
  same StackSmith build and bundled module.
- Invalid inputs explain what is wrong and why, and produce no output files.
- Output uses the approved S3 module and passes Terraform formatting and validation.
- Tests cover security invariants, platform override rejection, deterministic
  generation, and refusal to overwrite existing workspaces.
- The user can inspect the output and manually initialize, plan, apply, and
  destroy an empty sandbox bucket. Real AWS execution requires separate user
  authorization and credentials; automated checks do not deploy resources.

## 2. Exact v0.1 scope

### Included

- Go core library and thin CLI with `validate` and `generate` commands.
- Strict YAML and JSON for both developer specs and platform standards.
- One `Bucket` kind and one version-controlled standards document.
- One bundled, approved module identified by `s3-private/v1`.
- One platform-selected AWS account and region; initially `us-east-2` (Ohio).
- Initial allowed environments `dev` and `prod`, sharing identical rules and
  the same account. The allowed list is platform configuration, not hardcoded.
- Fixed security/tag contract, platform-controlled expiration and naming prefix.
- Readable, self-contained Terraform output, one root directory per bucket.
- Focused automated checks and incremental Terraform learning documentation.

### Excluded

- AI, MCP, APIs, SaaS, multi-tenancy, frontend, GitHub, and Atlantis integrations.
- Terraform execution, AWS calls, credential handling, state management, drift
  detection, imports, deployment orchestration, and automated migrations.
- Complete Application abstractions, databases, networking, IAM access grants,
  other cloud providers, or additional infrastructure kinds.
- Remote module distribution, arbitrary module sources, registries, plugins,
  independent company-module releases, and general-purpose policy engines.
- Standards inheritance/profiles, per-environment rules/accounts, multi-region
  provider aliases, arbitrary naming templates, custom tags, and escape hatches.
- Automatic workspace updates, overwrite/force options, backend generation,
  remote state setup, Terraform workspace management, or generated secrets.
- Customer-managed KMS keys, Object Lock, replication, independent backups,
  current-object expiration, and stronger deletion-protection controls.

## 3. Language and dependencies

Use **Go**, as agreed. It supports a typed library and portable compiled CLI,
fits the user's Go learning goal, and has HashiCorp's native HCL tooling for
safe Terraform generation and formatting.

TypeScript offers familiar types/tooling but requires a JavaScript runtime or
packaging step. Python is convenient for prototyping but offers less compile-time
checking. Both work; neither is substantially better for this small generator.
Go's learning cost is real, so use ordinary functions and two packages rather
than frameworks or speculative interfaces.

Use Go's standard library for CLI flags, JSON, file operations, embedding, and
tests. Add a maintained YAML parser and HashiCorp HCL v2 (`hclwrite`, with its
cty value types). Do not add an AWS SDK, CLI framework, templating language, or
policy framework. Confirm dependency versions during the relevant milestone.

## 4. Input contracts

### Developer specification — agreed

```yaml
apiVersion: stacksmith/v1alpha1
kind: Bucket
metadata:
  name: orders-assets
spec:
  environment: dev
  owner: orders-team
```

Each file describes exactly one bucket. `metadata.name` is a logical name, not
the final AWS bucket name. `owner` describes responsibility; it is not an
identity assertion or authorization grant. There are no developer fields for
public access, encryption, retention, tags, module source, account, or region.

`apiVersion` versions StackSmith's input contract, not Terraform or AWS.

### Platform standards — agreed

```yaml
apiVersion: stacksmith/v1alpha1
kind: PlatformStandards
spec:
  aws:
    accountId: "123456789012" # Example only; replace before Terraform execution
    region: us-east-2
  environments:
    - dev
    - prod
  bucket:
    module: s3-private/v1
    bucketNamePrefix: acme   # Optional; illustrative, not a chosen company prefix
    noncurrentVersionExpirationDays: 30
```

The platform owns and reviews this document and the approved module. All fields
shown are required except `bucketNamePrefix`, which defaults to empty. There
are no implicit account, region, owner, environment, or expiration defaults.
The initial policy value is 30 days, but the platform may change it; developers
cannot override it.

Selecting the module selects its fixed security and tag contract. Do not repeat
these invariants as configurable booleans or a generic tag-mapping language.
Company customization in v0.1 is intentionally limited to this typed contract.

### Strict parsing and validation

Both formats decode into the same Go types and use the same semantic validator.
Accept one document/object per file. Reject unknown and duplicate keys at every
level, trailing documents/data, wrong types, missing required fields, nulls in
place of required values, unsupported kinds/versions, and unsupported module IDs.
Do not silently coerce types, drop fields, trim names, or truncate identifiers.

Keep YAML JSON-compatible: string mapping keys; no aliases, merge keys, or custom
tags. Use parser facilities to detect duplicates and coercions rather than
assuming a decoder's defaults are strict. JSON duplicate-key detection also
needs explicit handling; Go's ordinary struct decoding alone is insufficient.
Bound input size before parsing (proposed limit: 1 MiB per document).

Proposed semantic rules:

- Names, nonempty prefixes, and environment entries use lowercase ASCII slugs:
  letters/digits separated by single hyphens, starting and ending alphanumeric.
- The fully composed bucket name must satisfy S3 naming restrictions, including
  reserved prefixes/suffixes and the 63-character limit. Reject, never truncate.
- Allowed environments form a nonempty list without duplicates; the selected
  environment must belong to it.
- Account ID is a string of exactly 12 ASCII digits, preserving leading zeros.
- Region must have valid AWS region-code syntax. Availability, opt-in status,
  name availability, account existence, and permissions are not checked offline.
- Owner is nonblank, has no control characters or surrounding whitespace, and
  fits S3's tag-value length limit. Apply tag limits to all emitted values.
- Expiration days must be a positive whole number representable by the chosen
  Go integer type; reject fractional values and overflow.

Diagnostics carry document identity, field path, reason, and a helpful correction;
include source location when available. Keep diagnostic ordering stable. Example:

```text
bucket spec: spec.public: unknown field; s3-private/v1 is private-only.
bucket spec: spec.environment: "staging" is not allowed; expected dev or prod.
```

## 5. Standards, modules, validation, and output boundaries

| Layer | Responsibility | Not its responsibility |
| --- | --- | --- |
| Platform standards | Select approved module, account/region, allowed environments, prefix, expiration | Arbitrary executable policies or developer overrides |
| Terraform child module | AWS resources, fixed security/tag invariants, lifecycle implementation, input checks | Credentials, provider configuration, backend/state ownership |
| Core validation | Strict input contract, standards validation, permitted values, safe names, override rejection | Proving live AWS compliance or checking credentials |
| Generated root module | Configure provider, pass resolved inputs, call approved module, expose outputs | Reimplement the module's AWS resources |
| Terraform CLI/provider | Resolve data sources, plan/apply changes, track state, check account and AWS constraints | Decide StackSmith's developer-facing contract |

Platform policy means infrastructure rules. An S3 bucket policy is specifically
an AWS permissions document; it is one mechanism used to implement those rules.

### Approved S3 contract

- All four S3 Block Public Access settings enabled; no public-mode input.
- Bucket-owner-enforced ownership, disabling ACLs.
- Explicit SSE-S3 default encryption, not customer-managed KMS.
- Bucket policy denying non-TLS access to the bucket and its objects.
- Versioning enabled, with no disable switch.
- Noncurrent versions expire after the platform-provided period (initially 30 days).
- Explicit `force_destroy = false`, with no override, in both environments.
- Four required tags, with no arbitrary developer tag map:

| Tag | Source |
| --- | --- |
| `Name` | Developer's logical name |
| `Environment` | Validated developer environment |
| `Owner` | Developer's owning team |
| `ManagedBy` | Fixed module value `stacksmith` |

Proposed child-module inputs: `bucket_name`, `name` (logical name), `environment`,
`owner`, and `noncurrent_version_expiration_days`. The module constructs the tag
map itself. Validate its inputs too, since the hand-written root calls it without
StackSmith. Expose `bucket_name` and `bucket_arn` as child and root outputs.

Use the AWS provider's separate resources for bucket configuration where required:
bucket, public access block, ownership controls, encryption, versioning, lifecycle,
and bucket policy. The lifecycle configuration must follow versioning enablement.
Explain each resource rather than hiding it behind generated code.

### Important S3 semantics

- Private means no public access, not VPC-only access. Authorized identities may
  still reach S3's public service endpoints; access grants are outside this MVP.
- Expiration is measured from when a version becomes noncurrent, not original
  upload time. Deletion is asynchronous and permanent for eligible old versions.
  This rule does not expire current versions or promise an exact deletion time.
- Versioning is not independent backup or guaranteed minimum retention. Users
  with sufficient permissions can permanently delete versions sooner.
- `force_destroy = false` prevents Terraform from automatically emptying a bucket.
  Empty buckets can still be destroyed. Cleaning a versioned bucket requires
  deliberately removing all versions and delete markers, not just visible objects.
- This is not `prevent_destroy`, Object Lock, or organization-wide deletion
  protection. The lifecycle rule still deletes eligible noncurrent versions.

## 6. Naming and account safeguards

Agreed naming convention:

```text
[<prefix>-]<name>-<environment>-<account-id>-<region>
acme-orders-assets-dev-123456789012-us-east-2
```

The optional prefix belongs to platform standards. Component ordering is fixed;
changing that pattern requires a code change in v0.1. No randomness or silent
shortening. The `Name` tag remains the logical name.

The generated root uses Terraform's `aws_caller_identity` data source to read
the executing account ID. StackSmith emits the expression without accessing AWS.
The region comes from standards. Configure the AWS provider with
`allowed_account_ids` containing the expected account ID so wrong-account
credentials fail during Terraform execution. The child inherits this provider.

An account ID is an identifier, not a credential. Credentials come from the
user's normal AWS authentication setup and are never generated or stored here.
Initial support targets the standard commercial AWS partition.

S3 names are shared across accounts and regions within a partition. Including
account and region reduces collisions but does not reserve names or guarantee
availability. Changing prefix, logical name, environment, account, or region
can change infrastructure identity and require replacement; it is a migration,
not a harmless label edit.

`dev` and `prod` are metadata, not account isolation. Both use the one configured
account and the same security/retention rules in v0.1.

## 7. Architecture and repository layout

Two Go packages: importable `core` and the CLI's `main` package.

```text
cmd/stacksmith/
  main.go                    # Arguments, bounded file reads, safe output, exit codes
  main_test.go               # CLI/output safety checks
core/
  spec.go                    # Developer input types
  standards.go               # Platform input types and contract
  parse.go                   # Strict YAML/JSON decoding
  validate.go                # Validation and internal resolved configuration
  generate.go                # HCL generation and embedded module assets
  core_test.go               # Focused checks; golden fixtures only as needed
  modules/s3/
    main.tf                  # Canonical Terraform source, not a generated copy
    variables.tf
    outputs.tf
    versions.tf
    tests/                   # Native Terraform module-contract tests
examples/
  bucket.yaml
  bucket.json
  standards.yaml
  manual-root/               # Hand-written root used before the Go generator
    ...
docs/
  stacksmith-core-plan.md
  ...                        # Small learning guides added with milestones
go.mod
go.sum
.gitignore
README.md
LICENSE
```

Create files only as their milestone needs them. No scaffolding all packages now.
The manual root references the canonical module; Go embeds those same `.tf` files
from `core/modules/s3/`. Tests are not copied into generated deployments.

Core exposes ordinary parsing, validation, and generation functions operating on
supplied bytes/typed values and returning structured diagnostics or named file
contents. Final signatures can be chosen when implementing the small public API.
No terminal, environment-variable, disk, network, or subprocess dependencies in
core. A future adapter can construct typed inputs and call the same validator.

An internal resolved configuration combines validated developer inputs with
platform values. It is not a second public spec or persisted intermediate format.
Generation always runs shared validation; library callers cannot bypass it by
forgetting to call `validate` first. Invalid inputs return no generated files.
Use HCL values for input strings; never concatenate untrusted input as HCL source.

## 8. CLI and generated artifacts

Proposed commands:

```sh
stacksmith validate --spec examples/bucket.yaml --standards examples/standards.yaml
stacksmith generate --spec examples/bucket.yaml --standards examples/standards.yaml --out ./orders-assets-dev
```

Use explicit flags, no hidden config search or interactive prompts. Support
`.yaml`, `.yml`, and `.json`; report unsupported formats. CLI handles paths, bounded
reads, presentation, exit codes, and output. Use exit 0 for success, 1 for invalid
input/generation/I/O failures, and 2 for command usage errors.

Each output is self-contained:

```text
orders-assets-dev/
  versions.tf
  providers.tf
  main.tf
  outputs.tf
  modules/s3/
    main.tf
    variables.tf
    outputs.tf
    versions.tf
```

Root `main.tf` calls `source = "./modules/s3"`. Copy approved child-module files
unchanged; do not emit raw AWS resources into the root. Terraform still downloads
its provider during `init`; a bundled module does not make Terraform execution
offline or remove the need to install Terraform.

Module ID `s3-private/v1` identifies the supported contract, not immutable bytes
across every future StackSmith release. Module changes require a StackSmith
release in v0.1. Record the generator version and module ID in a stable generated
comment. Exact reproducibility requires the same generator build and bundled
module bytes, not just the same input `apiVersion` or module ID.

Use deterministic block/file ordering and sorted map keys. No timestamps, random
suffixes, machine-specific absolute paths, or credentials in generated content.
Do not promise identical AWS plans: credentials, state, provider versions, and
live infrastructure affect Terraform independently of deterministic generation.

### Output safety and regeneration — agreed

Generate only into a new or empty directory. No overwrite or `--force` option.
Validate and render everything before writing. Use exclusive file creation,
reject unsafe/symlink destinations, and never overwrite state, lock files, or
existing content. Handle partial-write errors honestly; only clean up files
created by the current run, never preexisting user files. Test these behaviors.

For changes to an existing deployment:

1. Generate into a temporary review directory.
2. Inspect the configuration and module diff.
3. Manually copy intended configuration changes into the original workspace.
4. Run Terraform there, preserving its state and dependency lock file.

Never apply the review directory independently for the same bucket. Two states
would compete for one resource. Do not copy/replace state as part of regeneration.

### State and dependency versions

Use Terraform's default local state for the learning MVP, one authoritative root
per bucket. Local state is not a team backend; protect it, retain it, and never
commit it. Ignore state/backups, `.terraform/`, saved plan files, and real variable
files in this repository. Do not ignore `.terraform.lock.hcl` globally: commit
provider lock files with maintained root configurations.

Select and record tested Terraform and AWS provider versions in milestone 1;
choose a Terraform version supporting native mock-provider tests. Root output
pins the tested AWS provider version; child requirements declare compatibility.
`terraform init` creates the provider dependency lock file, which is not fabricated
by StackSmith. Provider upgrades are deliberate, reviewed changes, not implicit
consequences of regeneration. Go dependencies are recorded in `go.mod`/`go.sum`.

## 9. Incremental implementation and learning roadmap

### Milestone 1 — understand the actual Terraform

Build only the canonical S3 child module, a hand-written example root, a focused
native Terraform contract test, and the minimum documentation/ignore rules.
Do not build the generator yet. Choose tested tool/provider versions first.

Teach: HCL blocks and expressions; resources versus data sources; provider
configuration and inheritance; root versus child modules; variables and outputs;
resource references/dependency ordering; versioning, lifecycle, and bucket policy.

Check: `terraform fmt -check -recursive`, initialize without a remote backend,
`terraform validate`, and native mock-provider plan tests for module invariants,
input rejection, tags, lifecycle, and `force_destroy = false`. Provider downloads
may need network access; mock tests must not create AWS resources. Review the
hand-written root and account guardrail together before any real AWS run.

### Milestone 2 — parse and validate the contract

Add Go input types, strict YAML/JSON decoding, shared validation, diagnostics, and
examples. No generated HCL or CLI output-writing yet.

Teach: intent versus implementation; schema versus semantic rules; platform-owned
values versus developer inputs; offline validation versus provider validation.

Check: `go test ./...` covering equivalent formats, unknown/duplicate fields,
wrong types, missing values, invalid environment/module/account/name, forbidden
security/retention overrides, oversized input, and useful diagnostics.

### Milestone 3 — generate the root we already understand

Add internal resolution, HCL generation, embedded module copying, and root outputs.
Preserve the hand-written module's semantics; do not introduce a second module.

Teach: module composition; deterministic configuration versus runtime data-source
values; stable names and resource addresses; provider/module version identity.

Check: repeated output and YAML/JSON equivalence, sorted maps, literal escaping,
invalid input yielding no files, byte-identical module copies, and readable golden
output. Run Terraform format/validate checks on a generated fixture and inspect
the account allowlist and approved module inputs.

### Milestone 4 — expose the small CLI safely

Add `validate`/`generate`, explicit flags, exit codes, bounded reads, and new/empty
directory output. Document review/copy regeneration instead of in-place updates.

Teach: pure library versus adapter; generated artifacts versus persistent state;
why copying a Terraform directory is not the same as migrating infrastructure.

Check: CLI success/errors, YAML/JSON combinations, nonempty and symlink output
refusal, sentinel state/lock/config preservation, and partial I/O failure handling.
Run `go test ./...`, `go vet ./...`, and `go build ./cmd/stacksmith`.

### Milestone 5 — walk through a real sandbox lifecycle

With explicit user authorization and a correctly configured sandbox, the user
runs `terraform init`, reviews `plan`, and manually applies a generated bucket.
Inspect outputs, tags, encryption, access controls, versioning, and state together.
Demonstrate the review/copy workflow with a nonidentity change such as owner.

Teach: desired state and reconciliation; plan versus apply; state/resource identity;
drift; destructive changes; costs and cleanup. Explain noncurrent-version expiry
without pretending a short test proves deletion after 30 days.

Check: actual AWS settings and a no-change follow-up plan. Destroy the empty
sandbox bucket deliberately. Any object/versioning experiment must include an
explicit manual cleanup plan; StackSmith must not empty the bucket. If this
milestone has not run, report real-AWS behavior as unverified, not passed.

### Milestone 6 — review and close v0.1

Finish a short usage/learning guide, limitations, dependency/version notes, and
security review. Re-run core, CLI, and Terraform checks and review adherence to
this plan. No feature expansion disguised as cleanup.

Teach: validation layers and their limits; reproducible artifacts; operational
responsibility; when a small compiler becomes a platform product.

Acceptance: all automated checks pass, actionable findings are resolved, and
manual AWS validation is either completed or explicitly identified as pending.
Agree on the next milestone with the user rather than scaffolding future adapters.

## 10. Decide now versus defer

**Decided now:** ownership boundaries, Go/two packages, strict versioned inputs,
one private bucket, S3 invariants and tags, 30-day platform expiration, account
and region controls, fixed naming plus optional prefix, bundled module ID,
determinism, readable output, thin CLI, and no-overwrite/local-state workflow.

**Choose during implementation, before relying on them:** exact tested tool and
library versions, small Go API signatures, complete module-variable checks,
version-stamping build mechanics, and safe writer details. These must preserve
this contract. Update this plan first if a material scope/contract change is needed.

**Deliberately defer:** a second infrastructure type, generic policy language,
module marketplace/registry, independently versioned company modules, automated
upgrades, remote state/team collaboration, real environment/account isolation,
IAM/RBAC enforcement, runtime drift detection, all service/AI adapters, and SaaS.

## 11. Challenges and limitations

1. **Avoid recreating Terraform.** If the spec grows to mirror S3's full API,
   StackSmith adds translation without reducing complexity. Keep the small intent
   contract and put AWS implementation in Terraform.
2. **Local standards are not a security boundary.** A user who can change standards,
   generated Terraform, or AWS directly can bypass this path. Version-control
   review governs the platform source; IAM/SCPs and deployment authorization would
   be separate organization-level controls. Do not claim universal enforcement.
3. **Bundling limits platform independence.** Companies cannot distribute arbitrary
   modules in v0.1. That is a conscious MVP constraint, not the final platform vision.
4. **Determinism is narrower than reproducible deployment.** Lock versions and
   inspect state/live plans; identical generated bytes alone are insufficient.
5. **Naming and state are long-lived contracts.** Name-policy changes can replace
   buckets. Losing state or independently applying duplicate roots causes problems.
   No automatic rename, migration, or collision-retry scheme is included.
6. **Versioning has costs and limits.** Noncurrent versions incur storage charges
   until deleted; current objects remain. Lifecycle deletion reduces recovery
   history. Thirty days is a learning default, not a compliance recommendation.
7. **Secure does not mean complete.** No VPC-only access, backup system, immutable
   retention, application access grants, or full deletion protection is promised.
8. **Manual updates and local state do not scale to teams.** They are intentional
   learning-MVP limits. Do not present this as production collaboration tooling.
9. **Hand edits are visible but not a supported escape hatch.** Output is readable,
   not tamper-proof; altered Terraform falls outside StackSmith's generated contract.
10. **Two input formats add parser work.** Strictness and equivalent behavior need
    real tests, not confidence in permissive library defaults.

## 12. Approval gate

The S3 destruction safeguard is agreed: `force_destroy = false`, no override.
The consolidated plan is ready for final review. No Terraform, Go implementation,
AWS operations, or full project scaffold has been created during planning.

Next question: approve this plan and begin **milestone 1 only**, or revise the design?
