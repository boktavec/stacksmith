> **Superseded.** This page describes the removed Bucket generator. It is
> superseded by `docs/spec/features/blueprint-catalog/SPEC.md` and will be
> rewritten in Story 5.

# Generator: usage and learning notes (Milestone 3)

`core.Generate` turns a validated spec plus platform standards into the files of a
Terraform root. It is a pure function: it returns bytes and never touches the
filesystem, network, or Terraform. Writing files is a future adapter's job
(Milestone 4).

## Minimum usage

```go
spec, diags := core.ParseSpec(specBytes, core.FormatYAML)           // or FormatJSON
// check diags, then:
standards, diags := core.ParseStandards(standardsBytes, core.FormatYAML)
// check diags, then:
files, diags := core.Generate(spec, standards)
if len(diags) != 0 {
    // invalid input: diagnostics, and files is nil
}
for _, f := range files {
    // f.Path is slash-separated and relative to the output root,
    // f.Content is the exact bytes to write.
}
```

A valid input yields 8 files in a fixed order: `versions.tf`, `providers.tf`,
`main.tf`, `outputs.tf`, then `modules/s3/{main,variables,outputs,versions}.tf`.
Validation always runs inside `Generate`, so callers cannot skip it.

## Teaching points

- **Determinism.** Output has no timestamps, map-order effects, or machine data, so
  the same input always yields the same bytes. That is what makes golden files and
  reviewable diffs possible.
- **Format parity.** YAML and JSON are only two encodings of one spec; both must
  produce identical output, so the tests compare them directly.
- **Embedded module.** The canonical S3 module under `core/modules/s3` is embedded
  with `go:embed` and emitted unchanged. Its `tests/` directory and provider lock
  file are never bundled. The generated root pins `hashicorp/aws` to exactly
  `5.100.0`; the module keeps the looser `~> 5.0`.
- **`terraform validate` needs a provider.** Even with no real resources, `init`
  must install the AWS provider so Terraform knows each resource's schema.
  `-plugin-dir` installs it from a local directory instead of the network.
- **`mock_provider` and `command = plan`.** The module's native `terraform test`
  files replace the AWS provider with a mock and only plan, so they assert on
  configuration without credentials or AWS calls.

## Running the Terraform checks offline

The Terraform checks are Go tests in `core/terraform_test.go`. They write generated
output to `t.TempDir()` and run only `version`, `fmt`, `init -backend=false`,
`validate`, and `test`, with an allowlisted environment (no `AWS_*` variables, no
user CLI config, update checks off).

```sh
# fmt only (needs just the terraform binary, 1.7.0 or newer)
go test ./core -run Terraform -v

# fmt + init/validate + module tests: point at an unpacked provider mirror whose
# layout is <dir>/registry.terraform.io/hashicorp/aws/5.100.0/<os_arch>/
STACKSMITH_TF_PLUGIN_DIR=/path/to/mirror go test ./core -run Terraform -v

# fail instead of skip when anything above is unavailable (use in CI)
STACKSMITH_REQUIRE_TERRAFORM=1 STACKSMITH_TF_PLUGIN_DIR=/path/to/mirror go test ./core -run Terraform -v
```

Without the binary or the mirror, a test is skipped with a `NOT VERIFIED: ...`
message naming what was not checked. A skip is not a pass.

## What is not verified

- Real AWS behavior: no real `plan`, `apply`, or `destroy`, no backend, no state.
- The generated root is not diffed against `examples/manual-root`.
- JSON input is not run through Terraform separately; parity with YAML covers it.
- The provider mirror is developer-supplied and its hashes are not verified.
