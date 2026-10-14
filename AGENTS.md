# AGENTS.md

This file is the single source of agent guidance for StackSmith. It applies to every AI coding client and overrides conflicting instructions in tool-specific agent prompts. Keep project-specific requirements here, not in generated or nested instruction files.

## Project

StackSmith is an opinionated, offline-first infrastructure-as-code compiler built around Terraform. Developers provide a small declarative specification; platform-owned standards control security, account, region, naming, and approved modules. The current v0.1 scope and architecture are documented in `docs/stacksmith-core-plan.md`.

The project is also a learning exercise. Explain important Go and Terraform concepts as they arise, while keeping implementation straightforward and production-safe.

## Engineering principles

- Start simple; add abstractions, dependencies, services, and infrastructure only for a concrete requirement.
- Read the relevant code, tests, docs, and repository instructions before changing anything.
- Preserve security, validation, deterministic output, and safe workspace/file behavior.
- Keep the core library offline and free of filesystem, network, and subprocess effects unless the approved design explicitly changes that boundary.
- Treat generated Terraform as user-owned infrastructure: never run apply/destroy or access AWS without explicit authorization.
- Test real code paths; don't weaken tests to obtain a passing result.
- Stay within scope and report what was and was not verified.

## Skills

Read the relevant skill file before that kind of work:

| Skill | File | When to use it |
| --- | --- | --- |
| Pragmatic development | [`.agents/skills/pragmatic-development/SKILL.md`](.agents/skills/pragmatic-development/SKILL.md) | Default for implementation and review. |
| Test writing | [`.agents/skills/test-writing/SKILL.md`](.agents/skills/test-writing/SKILL.md) | Whenever writing or reviewing tests. |
| Debugging | [`.agents/skills/debugging/SKILL.md`](.agents/skills/debugging/SKILL.md) | Bugs, incidents, failing jobs, or wrong output. |
| Distributed-systems review | [`.agents/skills/distributed-systems-review/SKILL.md`](.agents/skills/distributed-systems-review/SKILL.md) | External dependencies, concurrency, production reliability, or security boundaries; apply lightly to local pure functions. |

## MCP and external tools

- **`context7`** provides current third-party library, framework, and tool documentation (Go modules such as `yaml.v3`, Cobra, the Terraform AWS provider and registry modules). Planners, implementers, and reviewers should use it whenever a spec or implementation depends on a version-specific API, configuration, or behavior, rather than relying on memory. Record the library version and relevant finding in the spec or PR. StackSmith's repository and agreed spec remain the authority for StackSmith requirements and decisions.

Use browser tools only when the feature has a runnable UI to verify. Tool availability varies by client; check before relying on a server and report unavailable checks honestly. Keep MCP credentials out of the repository, documentation, logs, and handoffs.

Use the YouTrack MCP for project tracking. The project is **StackSmith** (`STACK`) and its board is **StackSmith v0.1**. Before creating projects, boards, or issues, list existing items to avoid duplicates. Treat `docs/stacksmith-core-plan.md` and feature specs as the requirements source of truth.

- Break each milestone into ordered, small user stories that can each be implemented and reviewed as a separate PR. Scope each slice naturally so its PR remains comfortably reviewable (roughly under 500 changed lines), but do not put a numeric line limit in the story itself; split the work further when needed.
- Give each mini story its own focused implementation context. Include the story's scope, predecessor/dependencies, relevant design/spec references, explicit non-goals, and testable acceptance criteria. Each story must build on completed predecessor stories; don't implement later slices ahead of the chain.
- Make acceptance criteria concrete and independently verifiable, including relevant tests/checks and safety constraints. Keep the parent milestone's goal visible across the sequence; update the plan/spec first if a necessary change alters its contract.
- The project's configured issue Type supports `Feature` (use it for these milestone stories); `User Story` is not accepted. Preserve explicit authorization gates for AWS/Terraform operations in issue descriptions.

Keep YouTrack credentials out of the repo, and don't delete or close issues or projects without explicit user direction.

## Planning and implementation workflow

For each non-trivial feature or architectural change:

1. **Inspect first.** Read `AGENTS.md`, the relevant parts of `docs/`, existing specs/ADRs, affected code and tests, and applicable skills. Consult current library documentation when correctness depends on version-specific behavior.
2. **Plan before implementation.** Use the `senior-engineering-planner` subagent to clarify requirements one question at a time, examine alternatives and risks, and produce an implementation-ready spec. If the user cannot interact with the subagent directly, the main agent relays the planner's one question per turn to the user (verbatim options and recommendation), resumes the planner with the user's verbatim answer, and has the planner review any spec the main agent drafts; the main agent must not run the interview itself and present it as the planner's. If subagents are unavailable, conduct a planning-only phase and say so; never claim an agent ran when it did not.
3. **Save the spec.** Use `docs/spec/features/<feature-name>/SPEC.md` when that feature folder exists; otherwise create it. Capture scope, non-goals, acceptance criteria, decisions, risks, security, and ordered implementation/check steps. Present the draft for user review and incorporate feedback before implementation.
4. **Implement only against the approved spec.** Use `senior-software-engineer`. For UI work, involve `senior-ux-design-engineer` when available; this repository's current v0.1 scope has no UI. Keep parallel agents in separate worktrees and never have agents edit the same files concurrently.
5. **Review independently.** Use `staff-engineering-reviewer` with the spec, diff, and implementation handoff. Send actionable findings to the implementer and repeat review/fix until there are no blocking findings. Escalate planning-level decisions or unresolved risks to the user.
6. **Verify and report.** Run relevant formatting, build, vet, tests, and Terraform checks that do not deploy. Report actual results and any checks not run.
7. **Hand off a data-flow walkthrough.** After creating a PR for a story, give the user a walkthrough they can use to verify the code does what it should: (a) project context and where the story sits in the milestone chain; (b) the data flowing in, with concrete example values and its source; (c) what the story does to that data, step by step; (d) the data flowing out, including the invalid-input path and anything not written or executed; (e) the contracts with neighboring code that nothing checks automatically; (f) a table mapping each claim to the file, function, or test that proves it, plus a hands-on check the user can run. Base it on the code as merged to the branch, not on the plan, and say what was not verified.

For trivial edits, research questions, or standalone build/test requests, do not invoke the full workflow. Follow the user's requested scope and existing feature spec. Do not treat an existing draft or plan as approved unless the user says to proceed.

## Specs, milestones, and approvals

- Use the project's established feature specs under `docs/spec/features/` and the design in `docs/stacksmith-core-plan.md` as the source of product requirements.
- Respect any explicit approval gates in the feature spec. Approval to plan is not automatically approval to implement; approval to implement is not permission to publish or deploy.
- Keep the spec current if implementation reveals a material gap or changed contract; stop and ask before expanding scope.
- Prefer a feature branch (`feature/<feature-name>`) for feature work. Do not create/switch branches or worktrees unless the user or approved workflow authorizes it.

## Subagents

Agent definitions live in `.claude/agents/` (Claude Code) and `.pi/agents/` (Pi). Pi definitions delegate to the corresponding Claude prompt as the role source of truth. Subagents do not invoke each other; the main agent coordinates planning, implementation, review, and any UX handoff. Use the client-supported subagent mechanism. In Pi, use the `pi-subagents` extension configured under `.pi/`; if it is unavailable, report the blocker and continue only with work that does not require pretending a subagent ran.

## Repository-specific rules

- Use `docs/stacksmith-core-plan.md` as the design reference. Do not silently change scope or contracts.
- Keep Terraform visible and understandable; explain resources and lifecycle behavior as they are introduced.
- No AWS access, Terraform apply/destroy, or destructive cleanup without explicit user authorization.
- Do not commit, push, open PRs, merge, or deploy unless explicitly requested. Do not add AI attribution or `Co-Authored-By` trailers.
- Keep Terraform state, caches, saved plans, and real variable files out of Git. Keep `.terraform.lock.hcl` trackable; use sanitized `*.example.tfvars` names for examples.
- Never inspect or modify `.env` or `.env.*` files except `.env.example`.
- Stage explicit paths; inspect `git status` for generated and untracked files before any requested commit.
- Do not add nested or tool-generated agent instruction files. Root `AGENTS.md` is authoritative.

## Completion checklist

Before completing meaningful work:

- Run relevant Go formatting, `go build ./...`, `go vet ./...`, and `go test ./...` as applicable.
- Run Terraform formatting/validation only where appropriate; do not initialize against AWS or deploy.
- Review the diff for correctness, security, maintainability, spec adherence, and unnecessary complexity.
- Update relevant documentation and learning notes for meaningful features.
- State what passed, failed, or was not run, and identify remaining risks.
