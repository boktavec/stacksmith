# Feature spec: CLI (Milestone 4)

Status: **STACK-9 (section 1) APPROVED by user; planner re-review AGREE; plan sections 3, 7, 8 amended.** STACK-9 (section 1) is the only story
designed in detail; STACK-10 to STACK-12 are outlined and get their own detailed
sections before implementation.
Parent plan: `docs/stacksmith-core-plan.md`, sections 7-8 and section 9,
"Milestone 4 - expose the small CLI safely."
Predecessor: `docs/spec/features/generator/SPEC.md` (Milestone 3, complete).
Tracking: YouTrack STACK-2 (milestone); stories STACK-9 to STACK-12, one PR each,
implemented in order.

## Story chain

| Story | Title | Status in this spec |
| --- | --- | --- |
| STACK-9 | M4.1: Add shared CLI input loading and validate command | Designed (section 1) |
| STACK-10 | M4.2: Add the generate command and validate output destinations | Outline only |
| STACK-11 | M4.3: Make generated file writes exclusive and failure-safe | Outline only |
| STACK-12 | M4.4: Document CLI workflows and verify end-to-end behavior | Outline only |

---

## 1. STACK-9: shared input loading and `validate`

### 1.1 Goal

A thin `stacksmith validate --spec <file> --standards <file>` command that reads
two explicit files (bounded), calls the existing core parser and `core.Validate`,
and reports structured diagnostics with the right exit code. Establishes the
input-loading and exit-code path that `generate` (STACK-10) reuses.

### 1.2 Scope

- `cmd/stacksmith/main.go`: ~5 lines, `os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))`.
- `internal/cli/`: root cobra command, `validate` subcommand, input loader
  (extension -> `core.Format`, bounded read), diagnostic presenter (text and JSON),
  exit-code mapping.
- Tests in `internal/cli/` calling `Run` directly with in-memory writers and
  files under `t.TempDir()` (no subprocess). Reuse `examples/` and
  `core/testdata` fixtures where they fit.
- One new dependency: `github.com/spf13/cobra` (see 1.5, decision D2).

### 1.3 Non-goals

- `generate`, destination checks, or any file writing (STACK-10/11).
- Hidden config search, environment variables, prompts, or stdin input.
- Any change to core parsing/validation behavior or its public API.
- AWS access or Terraform execution.
- Shell completion, version command, or other cobra extras (disable the
  auto-generated `completion` command).

### 1.4 Repository facts

- `core.ParseSpec(data, format)` and `core.ParseStandards(data, format)` return
  `(value, []Diagnostic)`; `core.Validate(spec, standards) []Diagnostic`.
- `core.Diagnostic{Document, Field, Message}`; `Message` already includes a
  correction. `core.Format` is `FormatYAML | FormatJSON`.
- Core already rejects documents over `maxDocumentBytes = 1 << 20` (unexported).
- `go.mod` has no CLI dependency; there is no `cmd/` or `internal/` yet.
- `examples/bucket.yaml|json` and `examples/standards.yaml` exist.

### 1.5 Decisions

| ID | Decision | Why |
| --- | --- | --- |
| D1 | `cmd/stacksmith` + `internal/cli`; `Run(args []string, stdout, stderr io.Writer) int`. **Amends plan section 7** ("two Go packages") | Pure library vs adapter (plan milestone 4); tests assert exit code and output without a subprocess. |
| D2 | Use **cobra** (user choice; the recommended alternative was stdlib `flag`). **Amends plan section 3** ("no CLI framework") | Accepted cost: one dependency for two commands. Disable the `completion` command; `SilenceUsage`, `SilenceErrors`, `SetOut/SetErr` to the injected writers. |
| D3 | `--spec` and `--standards` are required string flags on `validate`; no defaults | "Explicit flags, no hidden search." |
| D4 | Format per file by extension, case-sensitive `.yaml`/`.yml`/`.json`; spec and standards may differ. Other/missing extension (including `.YAML`) -> exit 1, message names the flag and lists the exact allowed extensions | Plan milestone 4 check on YAML/JSON combinations. The flags were well-formed, so it is an input error, not usage. |
| D5 | Read bound: `io.ReadAll(io.LimitReader(f, limit+1))`, `limit = 1 << 20`; the bytes (including the extra one) are passed to core unchanged, so oversize yields **core's existing diagnostic** ("document exceeds the 1 MiB limit") | One user-facing rule, still never loads an unbounded file. The CLI constant only bounds the read; duplicated from core's unexported constant, with a comment and a boundary test. In `--format json` oversize is therefore a diagnostic. |
| D6 | Input must be a regular file: `os.Stat(path)` first (follows symlinks; symlinked **input** files are allowed), reject directories/devices/FIFOs with exit 1, then open, then re-check `f.Stat()` | `os.Open` on a FIFO blocks until a writer appears, so the check must precede the open. The remaining stat-to-open race is acceptable for input because the read is bounded. |
| D7 | Load both files first; a load failure (missing file, bad extension, non-regular) stops at the **first** failing file, spec before standards, as plain stderr, exit 1. Then parse each (core returns at most one parse diagnostic per document). If both parse, run `core.Validate`; otherwise report the parse diagnostics, spec first, and skip semantic validation | No semantic errors derived from an unusable document. Order matches core's `Validate`. |
| D8 | Text output (default): one line per diagnostic to **stderr**, `<document>: <field>: <message>` (no trailing colon when field is empty); success line `spec and standards are valid` to **stdout**. **Only non-printable/control runes in `Document`, `Field` and `Message` are escaped** (e.g. `strconv.Quote` per rune, or `unicode.IsPrint`); printable text, including `"`, is left byte-for-byte unchanged so core's own `%q` output is not double-escaped. Load-error lines (which echo the user's file path) use the same escaping | `Field` is built from the user's own keys and reaches the terminal raw otherwise (see 1.8). |
| D9 | `--format text\|json`, a **`validate`-local** flag (user-requested; amends plan section 8). JSON: one object on **stdout** for both outcomes: `{"valid":true,"diagnostics":[]}` / `{"valid":false,"diagnostics":[{"document","field","message"}]}`; exit codes unchanged. Usage errors (2) and load errors (1, D7) stay plain text on stderr in both modes. Other `--format` values -> exit 2; `RunE` checks `--format` **before** `loadInput`, so a bad format plus a missing file still exits 2. `diagnostics` is always an array | One stream to parse; STACK-10 decides its own flags. |
| D10 | Exit codes: 0 valid; 1 invalid input (diagnostics, load errors); **2 for every error from cobra's `Execute` that is not a `runError` (input or I/O failure)** (unknown/missing flag, missing required flag, unknown subcommand, extra args, no subcommand); output write failures are I/O failures, exit 1 (plan section 8) | Cobra has no single hook for these errors; this one rule is robust. |
| D11 | Help: root has a `RunE` returning a usage error when no subcommand is given (exit 2). `--help`/`-h` print help to stdout and exit 0; this is stated and tested. The auto `help` and `completion` commands are disabled | Cobra's default for a no-run root is help + exit 0, which would violate the exit-code contract. |
| D12 | `Run` converts nil args to `[]string{}` (cobra falls back to `os.Args` for nil) and builds a fresh command tree per call; no package-level command variables | Test isolation and no accidental real-CLI args. |
| D13 | `loadInput` returns `(data, format, err)`; callers must not use `format` when `err != nil` (`FormatYAML` is the zero value) | Avoid a silent wrong-format bug. |

### 1.6 Design

```text
main -> cli.Run(args, stdout, stderr) int
  newRootCmd() (fresh, silenced) -> validate cmd (RunE)
     loadInput(path) -> (data, core.Format, error)   // shared with STACK-10
     core.ParseSpec / core.ParseStandards
     core.Validate           // only when both parsed
     present(diags, format) -> exit code
```

- `RunE` returns a `runError` for load and output-write failures; `Run` maps any other
  `Execute` error to exit 2 (D10). An invalid document is not an error value:
  the command prints diagnostics and returns an exit code through a result, so
  the JSON path is the same code as the text path.
- `loadInput` is the only function touching the filesystem; keep it in
  `input.go` for STACK-10 to reuse.
- Core stays free of I/O; the CLI imports `core` only through its public API.

### 1.7 Acceptance criteria

1. `validate --spec examples/bucket.yaml --standards examples/standards.yaml` exits 0 with the success line.
2. `.yaml`, `.yml`, `.json` accepted; mixed spec/standards formats succeed; `.txt`, no extension and `.YAML` exit 1 naming the flag and the allowed extensions.
3. Invalid input exits 1 with every diagnostic's document and field intact; when both files fail to parse, both diagnostics print, spec first.
4. Parses-but-fails-validation input exits 1 with the validator's diagnostics.
5. Exit 2, nothing on stdout: missing `--spec` or `--standards`, unknown flag, unknown subcommand, extra positional arg, no subcommand, `--format yaml`. `--help` exits 0.
6. Missing file, directory and (Unix only, skipped elsewhere) FIFO exit 1 without hanging or panicking. A file of 1 MiB + 1 byte yields core's oversize diagnostic and exit 1; a file of exactly 1 MiB is not rejected by the CLI bound.
7. `--format json`: valid -> `{"valid":true,"diagnostics":[]}` exit 0; invalid -> `valid:false` with all three fields per diagnostic, exit 1; output parses as JSON and `diagnostics` is `[]`, never `null`.
8. **Escape test:** a spec with an ANSI escape in an unknown key (both YAML and JSON forms) produces text-mode stderr containing no raw ESC/control bytes; JSON mode output is valid JSON. An ordinary diagnostic line (e.g. one containing `"x"`) is printed byte-for-byte as core produced it.
9. `validate` writes no files (temp dir contents unchanged). `Run(nil, ...)` does not read `os.Args`.
10. `gofmt -l .` clean; `go vet ./...`, `go test ./...`, `go build ./cmd/stacksmith` pass; `go.mod`/`go.sum` gain only cobra and its required transitive modules; `go mod verify` passes.

### 1.8 Risks and security

- **Unbounded/hostile input:** regular-file check before open, bounded read (D5, D6). Residual: stat-to-open race, mitigated by the bound.
- **Terminal injection:** core quotes echoed values with `%q` in its messages, but `Diagnostic.Field` is built from the user's own keys (`joinField` on the unknown- and duplicate-field paths, YAML and JSON) and parser error text from the YAML/JSON libraries may echo raw input. The CLI therefore escapes control characters in all three fields in text mode (D8, AC 8). JSON mode is safe because `encoding/json` escapes them.
- **Dependency surface:** cobra adds transitive modules. Confirm the current version and cobra's behaviors (no-subcommand, required-flag errors, nil args, help) with Context7 before implementing; run `go mod verify`.
- **Exit-code drift:** cobra's defaults are overridden explicitly (D10-D12) and tested (AC 5).
- **Coupling to core's limit:** the duplicated constant could drift; comment plus boundary test (AC 6).

### 1.9 Plan changes required (apply first, before implementation)

`docs/stacksmith-core-plan.md`, one-line rationale each:

- **Section 3:** replace "no CLI framework" with a cobra exception (user choice; two commands, accepted dependency cost).
- **Section 7:** layout adds `internal/cli` (the CLI adapter's testable `Run`); `cmd/stacksmith/main.go` is a thin wrapper.
- **Section 8:** `validate` accepts `--format text|json` for presentation only.

### 1.10 Ordered implementation steps

1. Plan edits (1.9).
2. `go get github.com/spf13/cobra`; `internal/cli/input.go` + tests (extension, bound, non-regular file, FIFO).
3. Presenter (text with escaping, JSON), `validate`, root, exit mapping, `Run`.
4. `cmd/stacksmith/main.go`.
5. CLI tests for AC 1-9 via `Run`.
6. Checks (AC 10), `go mod tidy && git diff --stat go.mod go.sum`.
7. Reviewer handoff; PR; data-flow walkthrough (AGENTS.md step 7).

### 1.11 Learning notes to include

pure library vs adapter; why `Run` takes writers and returns an int (testable `main`);
cobra command tree vs stdlib `flag`; `io.LimitReader` and the limit+1 trick;
errors as typed values vs exit codes.

---

## 2. STACK-10 to STACK-12 (outline only)

Detailed sections are written when each story starts. Fixed inputs from the
tracker and plan:

- **STACK-10:** `generate` with `--spec`, `--standards`, `--out`; reuses `loadInput`
  and the exit-code mapping; parse + validate + render everything before the first
  write; accept only new/empty destinations; reject symlink destinations and unsafe
  path components. Whether `generate` also takes `--format` is decided then.
- **STACK-11:** exclusive creation (`O_EXCL`), cleanup of only this run's files,
  injected I/O failures, sentinel preservation; writes stay in the CLI adapter.
- **STACK-12:** CLI documentation (syntax, formats, exit codes, review/copy
  workflow) and end-to-end tests across both commands.
