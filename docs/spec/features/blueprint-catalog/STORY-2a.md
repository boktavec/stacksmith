# STACK-21 implementation plan: Story 2a, strict v2alpha1 Blueprint contracts (no overrides)

Status: **Design approved by the user (2026-10-13).** Approval to implement is not permission to commit, push, open a PR or deploy.
Parent: STACK-19. Predecessor: STACK-20 (merged). Unblocks: STACK-25 (Story 2b).
Authoritative contract: [`SPEC.md`](SPEC.md), section 4 and Story 2a. This plan adds the decisions from the planning interview.

## 1. Scope

**Included**

- `stacksmith/v2alpha1` `Blueprint` spec and `PlatformStandards` types, with strict YAML and JSON schemas.
- A strict pass that checks only `apiVersion` and `kind`, run before the full schema walk.
- Semantic validation of name, environment, account, region, blueprint (unknown vs unsupported) and tier.
- Deleting the whole bucket path, while keeping the reusable Terraform test helpers.
- New network examples.
- Rewiring `stacksmith validate`.
- A superseded note in `docs/learning/generator.md` and a small update to `docs/strict-parsing.md`.

**Non-goals**

- Override policy and dynamic-key maps (Story 2b).
- Terraform rendering and `File`/`Generate` (Story 3).
- The `generate` command (Story 4a).
- Compatibility with the old Bucket document.
- Support for any blueprint other than `network`.
- Any AWS access, credentials, or Terraform init/plan/apply/destroy.

## 2. Decisions

| # | Decision | Source |
| --- | --- | --- |
| D1 | `decodeDocument` runs a strict header-only pass (only `apiVersion` and `kind` are known) before the full schema walk. A wrong or missing version or kind is reported before unknown fields. | user Q1 |
| D2 | The diagnostic `Document` labels are `blueprint spec` and `platform standards`. | user Q2 |
| D3 | Keep `core.Spec`, `core.SpecMetadata` and `core.ParseSpec`. The inner type becomes `BlueprintSpec{Blueprint, Environment, Tier string; Parameters map[string]string}`. | user Q3 |
| D4 | `Validate(spec, standards) []Diagnostic` keeps its signature. It gets the catalog from an unexported `mustLoadCatalog()`, which panics with a build-defect message if the embedded catalog fails to decode. | user Q4 |
| D5 | The tier is checked only once the blueprint is supported. Unknown and unsupported IDs each get one distinct `spec.blueprint` diagnostic. Lists in messages are sorted. | user Q5 |
| D6 | A non-empty `spec.parameters` is rejected by the parser's existing unknown-field diagnostic, via an empty `fieldSet()` schema entry. `blueprintOverrides` is rejected the same way. | user Q6 |
| D7 | `expectedKind`'s special case for `spec.environments[i]` is keyed on `schema.kinds["spec.environments"] == arrayValue` instead of the schema name, so the header pass does not type-check array items. | approved recommendation |
| D8 | `docs/strict-parsing.md` changes "bucket name" to "blueprint name" and gains one sentence about the version-first pass. | approved recommendation |
| D9 | The examples leave out `parameters`. A separate test covers `parameters: {}` in both formats. | approved assumption |

## 3. Requirements and acceptance criteria

**AC1. YAML/JSON parity**

- `examples/network.yaml` and `examples/network.json` parse to `reflect.DeepEqual` `Spec` values.
- Both pass `Validate` against `examples/standards.yaml` with no diagnostics.
- Every rejection case in the parse table produces the same `(Document, Field, message substring)` in both formats. The line-number prefix appears in YAML only.

**AC2. Unknown vs unsupported blueprint**

- `spec.blueprint: netwrok` gives exactly `unknown blueprint "netwrok"; run "stacksmith catalog list" to see available blueprints.`
- `spec.blueprint: rds` gives exactly `blueprint "rds" is in the catalog but not supported by this StackSmith version; supported: network.`
- Neither case produces a `spec.tier` diagnostic. Only `network` reaches the tier check.

**AC3. Stable field-specific rejections.** Each case below produces the listed document, field and message substring.

| Case | Field |
| --- | --- |
| Unknown field (`spec.owner`, in a v2alpha1 document) | `spec.owner` |
| Duplicate key | the duplicated path |
| Wrong type (`spec.tier: 3`; JSON `"tier":3`) | `spec.tier` |
| `null` in a required field | the field |
| Missing required field (`spec.tier` absent) | `spec.tier`, "required field is missing" |
| `apiVersion` missing, wrong type, or wrong value | `apiVersion` |
| `kind` wrong (e.g. `Application`; `Blueprint` in the standards document) | `kind` |
| Name not a slug (`Shared_Net`, `-a`, `a--b`, `a-`) | `metadata.name` |
| Name of 64 characters (63 accepted) | `metadata.name`, "63" |
| Bad account (11 digits, letters) | `spec.aws.accountId` |
| Bad region (`useast2`) | `spec.aws.region` |
| Environment not a slug, or not in the list | `spec.environment` |
| Standards environments empty, not a slug, or duplicated | `spec.environments` / `spec.environments[i]` |
| Tier not in catalog (`staging` on `network`) | `spec.tier`: `tier "staging" is not defined for blueprint "network"; use one of: dev, prod.` |
| Non-empty `spec.parameters` (`az_count: "3"`; also unquoted `3`) | `spec.parameters.az_count`, "unknown field" |
| `spec.blueprintOverrides` in standards | `spec.blueprintOverrides`, "unknown field" |

**AC4. `parameters` absent or empty.** An absent `spec.parameters` and `parameters: {}` (YAML and JSON) are both accepted, and `len(spec.Spec.Parameters) == 0`.

**AC5. Old Bucket document.** The v1alpha1 Bucket document, in YAML and JSON, gives `blueprint spec: apiVersion: unsupported apiVersion "stacksmith/v1alpha1"; use "stacksmith/v2alpha1".` A v1alpha1 standards document gives the equivalent `platform standards` diagnostic.

**AC6. No bucket code remains.**

- All of the following are deleted:
  - `core/generate.go`, `core/render.go`, `core/resolve.go`
  - `core/render_test.go`, `core/resolve_test.go`, `core/verify_test.go`
  - `core/modules/s3/`, `core/testdata/bucket/`
  - `examples/bucket.yaml`, `examples/bucket.json`, `examples/manual-root/`
- `grep -rni bucket core internal cmd examples` matches only upstream catalog data in `core/snapshot/` (for example the `s3-event-worker` summary).

**AC7. CLI**

- `stacksmith validate --spec examples/network.yaml --standards examples/standards.yaml` and the `.json` spec each print `spec and standards are valid` on stdout and exit 0.
- Invalid input sends diagnostics to stderr and exits 1. `--format json` keeps its shape. Usage errors exit 2.
- Every existing behavioural CLI test is kept, updated only for the new fixtures, the label, and the version-first ordering.

**AC8. Checks pass**

- `gofmt -l .` produces no output.
- `go vet ./...`, `go build ./...` and `go test ./...` pass.

**AC9. Safety**

- No AWS calls or credentials.
- No Terraform commands are run by any test added or kept active in this story.
- Core stays free of filesystem, network and subprocess effects. `go:embed` is compile-time only.

## 4. Technical design

### 4.1 `core/spec.go`

- `Spec{APIVersion, Kind, Metadata SpecMetadata, Spec BlueprintSpec}`.
- `SpecMetadata{Name}`.
- `BlueprintSpec{Blueprint, Environment, Tier string; Parameters map[string]string}`, with `yaml`/`json` tags `blueprint`, `environment`, `tier`, `parameters`.
- Doc comments point to spec section 4. They state that `Parameters` is always empty in 2a and is filled in by Story 2b.

### 4.2 `core/standards.go`

- `PlatformStandards{APIVersion, Kind, Spec StandardsSpec}`.
- `StandardsSpec{AWS AWSStandards, Environments []string}`.
- `AWSStandards{AccountID, Region}`.
- `BucketStandards` is removed.

### 4.3 `core/parse.go`

- **Schema metadata.** Add `apiVersion` and `kind` fields to `documentSchema`, holding the expected values.
- **`specSchema`** (renamed from `bucketSpecSchema`):
  - `name: "blueprint spec"`, `apiVersion: "stacksmith/v2alpha1"`, `kind: "Blueprint"`
  - fields: `"": apiVersion, kind, metadata, spec`; `metadata: name`; `spec: blueprint, environment, tier, parameters`; `"spec.parameters": fieldSet()`
  - kinds: objects for `""`, `metadata`, `spec`, `spec.parameters`; strings for `apiVersion`, `kind`, `metadata.name`, `spec.blueprint`, `spec.environment`, `spec.tier`
  - required: `apiVersion`, `kind`, `metadata.name`, `spec.blueprint`, `spec.environment`, `spec.tier`
- **`standardsSchema`**:
  - `name: "platform standards"`, `apiVersion: "stacksmith/v2alpha1"`, `kind: "PlatformStandards"`
  - fields: `"": apiVersion, kind, spec`; `spec: aws, environments`; `spec.aws: accountId, region`
  - kinds: objects for `""`, `spec`, `spec.aws`; strings for `apiVersion`, `kind`, `spec.aws.accountId`, `spec.aws.region`; array for `spec.environments`
  - required: `apiVersion`, `kind`, `spec.aws.accountId`, `spec.aws.region`, `spec.environments`
- **`headerSchema(s)`** returns `documentSchema{name: s.name, kinds: {"": objectValue, "apiVersion": stringValue, "kind": stringValue}}`. With `fields` left nil, the reader does no unknown-field checks, because reading a missing key from a nil map is safe in Go and returns `ok == false`.
- **`decodeDocument`** order:
  1. Size check, then UTF-8 check (unchanged).
  2. Read with `headerSchema(schema)`. On failure, return that diagnostic.
  3. If `apiVersion` is missing, return the required-missing diagnostic at `apiVersion`. If its value differs, return `unsupported apiVersion %q; use %q.`
  4. Do the same for `kind`, with `unsupported kind %q; use %q.`
  5. Read with the full schema, check required fields, run the typed strict decode, and check for trailing data (all unchanged).
- Factor the YAML/JSON switch into a small `readDocument(data, format, schema)` so both passes share it.
- **`ParseSpec` / `ParseStandards`** become thin wrappers. The checks that ran after decoding (apiVersion, kind, `spec.bucket.module`) are removed.
- **`expectedKind`** is changed per D7.

### 4.4 `core/catalog.go`

- Factor the existing read-and-decode into `loadCatalog() (catalogFile, error)`. `Catalog()` keeps its exported behaviour and returns errors as it does now.
- Add `mustLoadCatalog() catalogFile`, which panics with `"embedded catalog snapshot is invalid (build defect): " + err`.

### 4.5 `core/validate.go`

Diagnostics are collected in this fixed order:

1. `metadata.name` is not a slug.
2. `metadata.name` has more than 63 bytes: `exceeds the 63-character limit; shorten the name.` A slug is ASCII, so `len` counts characters.
3. `spec.environment` is not a slug.
4. Standards `spec.environments`: empty, then each item not a slug, then each duplicate.
5. `spec.environment` is not allowed (gated on the slug check, as now).
6. `spec.aws.accountId`.
7. `spec.aws.region`.
8. `spec.blueprint`: unknown, or unsupported.
9. `spec.tier`: checked only if the blueprint is supported. Membership in the sorted keys of `catalog.Blueprints[id].Tiers`.

Helpers:

- `sortedKeys` is a small helper for maps with string keys.
- The supported list is the sorted keys of `supportedBlueprints`, joined with `", "`.
- Messages that echo user values use `%q`. The CLI already escapes control characters.

Removed: the owner checks, the 256-character tag limit, `composeBucketName`, `invalidBucketName`, `isIPAddress`, and the `maxTagValueCharacters` constant. Any imports that become unused (`strconv`, `unicode`, `unicode/utf8`) are dropped as well.

### 4.6 `core/diagnostic.go`

Only the comment example changes, from `"bucket spec"` to `"blueprint spec"`.

### 4.7 `core/terraform_test.go`

- Keep `skipOrFail`, `terraformEnv`, `terraformCmd`, `requireTerraformEnv` and `terraformTimeout`.
- Delete `pluginDirEnv`, `terraform`, `mustTerraform`, `requireTerraform`, `requireProviderMirror`, the `requireMockedPlanOnly` regexes, `writeFiles`, `generatedExampleDir` and all three tests.
- Prune imports (`encoding/json`, `regexp`, `runtime`, `strconv`, `strings`).
- Add a file comment saying the helpers are kept for Story 3.
- `go vet` does not flag unused unexported functions. The Go compiler does reject unused imports.

### 4.8 Examples

- `examples/standards.yaml`: `stacksmith/v2alpha1`, `PlatformStandards`, account `"123456789012"` with the existing "example only" comment, `us-east-2`, environments `[dev, prod]`.
- `examples/network.yaml`: `shared-network`, `network`, `dev`, `dev`, with no `parameters`.
- `examples/network.json`: the same content as a single line, like the current `bucket.json`.

### 4.9 `internal/cli/cli.go`

- The `--spec` flag description becomes "path to the blueprint spec (.yaml, .yml or .json)".
- The validate `Short` becomes "Check a blueprint spec against platform standards without generating anything".
- `validMessage`, the root usage text, streams and exit codes are unchanged.

### 4.10 Docs

- `docs/learning/generator.md`: add a blockquote at the top saying it describes the removed Bucket generator, is superseded by `docs/spec/features/blueprint-catalog/SPEC.md`, and will be rewritten in Story 5.
- `docs/strict-parsing.md`: make the D8 edit.

## 5. Dependency-ordered implementation steps

This is one PR on branch `feature/STACK-21-blueprint-contract`, matching the STACK-20 naming. Steps 1 to 6 leave the tree unbuildable until step 6 is done, so run `go build ./...` only after step 6.

1. **Delete the bucket path.** Remove the files in AC6 and prune `core/terraform_test.go` per 4.7.
2. **Types.** Rewrite `core/spec.go` and `core/standards.go` per 4.1 and 4.2.
3. **Catalog helper.** Add `loadCatalog` and `mustLoadCatalog` per 4.4. `Catalog()` behaviour is unchanged.
4. **Parser.** Apply 4.3: the schemas, `headerSchema`, the two-pass `decodeDocument`, `readDocument`, the `expectedKind` fix, and the thinner `Parse*` functions.
5. **Validation.** Rewrite `core/validate.go` per 4.5, and update the `core/diagnostic.go` comment.
6. **Examples.** Add `network.yaml`, `network.json` and the new `standards.yaml`. Run `go build ./... && go vet ./...` and fix any compile errors.
7. **Core tests.** Rewrite `core/core_test.go`:
   - examples parity and validity (AC1)
   - the two-format parse rejection table (AC3, AC5, D6)
   - `{}` and absent `parameters` (AC4)
   - the `Validate` table, including the 63/64 boundary (AC2, AC3)
   - a stable-order test with several diagnostics
   - `mustLoadCatalog` succeeding on the real snapshot

   Keep the existing tests for oversize input, invalid UTF-8, unsupported format, and leading zeros in the account ID, adapted to the new fixtures. `catalog_test.go` stays as is.
8. **CLI.** Apply 4.9. Update `internal/cli/cli_test.go`:
   - example constants point to `network.yaml` and `network.json`
   - inline fixtures become v2alpha1 Blueprint documents
   - the expected label becomes `blueprint spec`
   - the `kind: x` byte-for-byte test now expects `blueprint spec: kind: unsupported kind "x"; use "Blueprint".`
   - the `escapeControl` literal changes from `"Bucket"` to `"Blueprint"`
   - the control-character JSON fixture becomes a v2alpha1 document with the hostile key under `spec`
   - `paddedSpec` stays valid v2alpha1 padding

   Add a test that both new examples validate (AC7).
9. **Docs.** Apply 4.10.
10. **Verify.** Run `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`, the AC6 grep, and both `go run ./cmd/stacksmith validate ...` example commands, plus one invalid run to check exit 1 and stderr. Report actual results. Do not run any Terraform command.
11. **Review and hand-off** per `AGENTS.md`: staff-engineering-reviewer review, and a data-flow walkthrough after the PR. Commit or open the PR only when the user asks, with no `Co-Authored-By` trailer, and stage explicit paths.

## 6. Risks and mitigations

- **The two-pass reader could shift existing diagnostics.** Mitigation: the rejection table is pinned in both formats. The only expected differences are the label and the version-first ordering.
- **A panic in library code.** It is reachable only through a broken embedded snapshot. The STACK-20 decode tests and the new `mustLoadCatalog` test guard against it.
- **Generic message for `spec.parameters`.** It is temporary, and Story 2b replaces it.
- **Old Bucket inputs and bucket workspaces.** Bucket inputs stop validating, which is intended. Existing generated bucket workspaces are user-owned and untouched. Story 5 documents this break.
- **No `generate` until Story 4a.** The spec accepts this intermediate state.
- **Review size.** Edits come to about 350 to 450 lines, plus large mechanical deletions. Reviewers can look at the deletions separately with `git diff --stat`.

## 7. Go learning notes for the implementer to explain briefly

- `map[string]struct{}` as a set; an empty set means "nothing is allowed".
- Reading a missing key from a nil map is safe.
- `Must…` functions panic for broken invariants, never for user input.
- Boolean gates let `Validate` collect every independent diagnostic.
- A nil map and an empty map compare differently under `reflect.DeepEqual`.
- Map iteration order is random, so lists in diagnostics are sorted.
