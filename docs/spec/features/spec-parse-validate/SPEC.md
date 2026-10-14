# Feature spec: parse and validate the contract (Milestone 2)

Status: **Superseded** by `docs/spec/features/blueprint-catalog/SPEC.md` (approved 2026-10-13; YouTrack STACK-19). The `stacksmith/v1alpha1` Bucket contract described here is removed in that feature's Story 2a; its strict-parsing rules carry forward there. Kept as a historical record only.

Historical status: complete.
Parent plan: `docs/stacksmith-core-plan.md`, section 9, "Milestone 2 — parse
and validate the contract." First Go code in the repo; no HCL generation or
CLI output-writing yet (milestones 3-4).

## 1. Goal

A `core` Go package that strictly decodes developer specs and platform
standards (YAML or JSON) into typed values, and validates them against the
semantic rules already fixed in the plan (section 4), returning structured
diagnostics. No file writing, no HCL, no CLI wiring.

## 2. In scope

- `go.mod`/`go.sum` — module path `github.com/boktavec/stacksmith`.
- `core/spec.go` — developer spec Go types (`apiVersion`, `kind`, `metadata.name`,
  `spec.environment`, `spec.owner`).
- `core/standards.go` — platform standards Go types (`aws.accountId`,
  `aws.region`, `environments`, `bucket.module`, `bucket.bucketNamePrefix`,
  `bucket.noncurrentVersionExpirationDays`).
- `core/diagnostic.go` — the shared `Diagnostic` type: document identity,
  field path, reason, correction hint; stable ordering.
- `core/parse.go` — `ParseSpec` / `ParseStandards`: strict YAML (`yaml.v3`,
  `KnownFields(true)` + duplicate-key check on the decoded node tree) and
  strict JSON (`json.Decoder` with `DisallowUnknownFields` + a token-stream
  duplicate-key check) for both formats, one document per file, 1 MiB input
  cap. Reject YAML aliases, merge keys, custom tags, non-string mapping keys,
  and scalar type coercion. Structural failures (malformed input,
  unknown/duplicate keys, wrong types, nulls for required fields, unsupported
  `kind`/`apiVersion`/module ID, oversized input) return `Diagnostic`s, not bare
  Go errors.
- `core/validate.go` — `Validate(spec, standards) []Diagnostic`: the semantic
  rules from plan section 4 (slug formats, composed-name S3 legality,
  environment membership, 12-digit account ID, region syntax, owner
  constraints, positive/non-overflowing expiration days). Runs after both
  inputs parse successfully; collects every violation in one pass rather than
  stopping at the first.
- `core/core_test.go` — table-driven tests, invalid cases as inline
  YAML/JSON string literals, covering: YAML/JSON equivalence for the same
  logical input, unknown field, duplicate key, wrong type, missing required
  field, null for required field, unsupported kind/apiVersion/module,
  oversized document, each semantic rule's rejection case, and multi-error
  collection (one input with two+ problems produces two+ diagnostics).
- `examples/bucket.yaml`, `examples/bucket.json` (equivalent valid specs),
  `examples/standards.yaml` (valid standards) — read via `go:embed` or plain
  file reads in tests to prove the shipped examples actually parse and
  validate.
- Short learning note under `docs/` on strict decoding (why stdlib
  `encoding/json` and naive YAML aren't strict enough by default), added
  alongside the parse.go commit per CLAUDE.md's "explain as it arises" rule.

## 3. Out of scope

- HCL generation, embedded module copying, root/output files (milestone 3).
- `cmd/stacksmith` CLI, flags, exit codes (milestone 4).
- Any resolved/merged internal configuration type (milestone 3's concern).
- Real AWS/Terraform execution or validation.

## 4. Public API (agreed)

```go
package core

type Diagnostic struct {
    Document string // e.g. "bucket spec", "platform standards"
    Field    string // dotted path, e.g. "spec.environment"
    Message  string // reason + correction, plan-style single line
}

func ParseSpec(data []byte, format Format) (Spec, []Diagnostic)
func ParseStandards(data []byte, format Format) (PlatformStandards, []Diagnostic)
func Validate(spec Spec, standards PlatformStandards) []Diagnostic
```

`Format` is a small typed enum (`FormatYAML`, `FormatJSON`). A non-empty
`[]Diagnostic` from either parse function means the returned struct is not
usable; callers must not pass it to `Validate`. `Validate` assumes both
inputs already parsed cleanly.

## 5. Semantic rules (from plan section 4, restated for the todo list)

- Name/prefix/environment: lowercase ASCII slug, letters/digits, single
  hyphens, alphanumeric start/end.
- Composed bucket name (`[prefix-]name-environment-accountid-region`) must
  satisfy S3 naming rules including the 63-character limit and reserved
  prefixes/suffixes. Reject; never truncate.
- `environments` (standards): nonempty, no duplicates; spec's `environment`
  must be a member.
- `accountId`: exactly 12 ASCII digits, leading zeros preserved (string type,
  not numeric).
- `region`: valid AWS region-code syntax (offline syntax check only).
- `owner`: nonblank, no control characters, no leading/trailing whitespace,
  within S3 tag-value length limit.
- `noncurrentVersionExpirationDays`: positive whole number, no fractional
  values, no overflow of the chosen Go integer type.
- `bucket.module`: must be the one supported ID, `s3-private/v1`.

## 6. Checks (acceptance criteria)

- `go build ./...` and `go vet ./...` pass.
- `go test ./...` passes, covering every case in section 2's test list.
- `gofmt -l .` reports no files.
- Shipped `examples/bucket.yaml` and `examples/bucket.json` parse to equal
  `Spec` values; both validate cleanly against `examples/standards.yaml`.
- No file writing, network, or subprocess calls anywhere in `core`.

## 7. Deliverable checklist (maps to Phase 3 "Todos" once approved)

1. `go.mod` — module `github.com/boktavec/stacksmith`, add `gopkg.in/yaml.v3`.
2. `core/spec.go` — `Spec` type and nested structs.
3. `core/standards.go` — `PlatformStandards` type and nested structs.
4. `core/diagnostic.go` — `Diagnostic` type, `Format` enum.
5. `core/parse.go` — `ParseSpec`, `ParseStandards`, strict YAML/JSON decode
   helpers, size cap, duplicate-key checks for both formats.
6. `core/validate.go` — `Validate` and the semantic rule checks from section 5.
7. `core/core_test.go` — table-driven parse/validate tests per section 2.
8. `examples/bucket.yaml`, `examples/bucket.json`, `examples/standards.yaml`.
9. Short learning note under `docs/` on strict YAML/JSON decoding.

## 8. Open questions resolved during Phase 0

- Module path: `github.com/boktavec/stacksmith`.
- YAML library: `gopkg.in/yaml.v3` (`KnownFields` + manual duplicate-key
  check on the node tree).
- JSON strictness: stdlib `encoding/json` (`DisallowUnknownFields`) plus a
  `json.Decoder`/`Token()` walk to catch duplicate object keys.
- Diagnostic collection: gather all semantic violations in one `Validate`
  call; parsing still fails fast since later checks can't run on a document
  that didn't decode.
- Public API shape: separate `ParseSpec`/`ParseStandards`/`Validate`, no
  combined resolved-config entry point (that's milestone 3).
- Parse failures use the same `Diagnostic` type as validation failures.
- Invalid-input test fixtures live inline in `core_test.go`, not as separate
  `testdata/` files.

No other product decisions remain open for this milestone — the input
contracts and semantic rules are already fixed by
`docs/stacksmith-core-plan.md` section 4 and are not re-decided here.
