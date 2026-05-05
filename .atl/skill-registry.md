# Skill Registry

**Delegator use only.** Any agent that launches sub-agents reads this registry to resolve compact rules, then injects them directly into sub-agent prompts. Sub-agents do NOT read this registry or individual SKILL.md files.

See `_shared/skill-resolver.md` for the full resolution protocol.

## User Skills

| Trigger | Skill | Path |
|---------|-------|------|
| when drafting or posting feedback, review comments, maintainer replies, Slack messages, or GitHub comments | comment-writer | /home/ad373971/.config/opencode/skills/comment-writer/SKILL.md |
| when implementing a change, preparing commits, splitting PRs, or planning chained or stacked PRs | work-unit-commits | /home/ad373971/.config/opencode/skills/work-unit-commits/SKILL.md |
| when writing guides, READMEs, RFCs, onboarding docs, architecture docs, or review-facing documentation | cognitive-doc-design | /home/ad373971/.config/opencode/skills/cognitive-doc-design/SKILL.md |
| when a PR would exceed 400 changed lines, when planning chained PRs, stacked PRs, or reviewable slices | gentle-ai-chained-pr | /home/ad373971/.config/opencode/skills/chained-pr/SKILL.md |
| when creating a GitHub issue, reporting a bug, or requesting a feature | issue-creation | /home/ad373971/.config/opencode/skills/issue-creation/SKILL.md |
| when creating a pull request, opening a PR, or preparing changes for review | branch-pr | /home/ad373971/.config/opencode/skills/branch-pr/SKILL.md |
| when user asks to create a new skill, add agent instructions, or document patterns for AI | skill-creator | /home/ad373971/.config/opencode/skills/skill-creator/SKILL.md |
| when writing Go tests, using teatest, or adding test coverage | go-testing | /home/ad373971/.config/opencode/skills/go-testing/SKILL.md |
| when user says "judgment day", "judgment-day", "review adversarial", "dual review", "doble review", "juzgar", "que lo juzguen" | judgment-day | /home/ad373971/.config/opencode/skills/judgment-day/SKILL.md |

## Compact Rules

### comment-writer
- Start with the actionable point, do not preamble.
- Keep comments short (1-3 paragraphs or tight bullets).
- Explain the technical why when requesting changes.
- Focus on highest-value feedback, avoid preference pile-ons.
- Match thread language (Spanish: Rioplatense voseo).
- Do not use em dashes.

### work-unit-commits
- Each commit must represent one deliverable work unit.
- Never split by file type when behavior is coupled.
- Keep tests/docs in the same commit as the change they verify.
- Ensure each commit is reviewable, rollback-safe, and story-driven.
- If projected diff is >400 lines, pre-slice into chained PR units.
- Use Conventional Commit messages focused on outcome.

### cognitive-doc-design
- Lead with outcome/decision first, then context.
- Use progressive disclosure: quick path first, details later.
- Chunk content into scan-friendly sections and short lists.
- Prefer recognition aids: tables, checklists, templates.
- Make review path explicit, including scope boundaries.
- Keep each section tied to one decision/work unit.

### gentle-ai-chained-pr
- Split PRs exceeding 400 changed lines unless `size:exception` is approved.
- Keep each slice autonomous: green CI, clear scope, rollback, verification.
- State start/end boundaries, dependencies, and follow-ups in every PR.
- Use one strategy per chain (stacked-to-main or feature-branch chain).
- Include chain diagram and status table, mark current PR.
- For >2 child PRs, create a draft tracker PR map.

### issue-creation
- Use templates only, blank issues are disallowed.
- New issues start with `status:needs-review`.
- No PR until maintainer adds `status:approved`.
- Route questions to Discussions, not Issues.
- Require complete reproduction/problem context fields.
- Search for duplicates before creating a new issue.

### branch-pr
- Every PR must link an approved issue (`Closes/Fixes/Resolves #N`).
- Add exactly one `type:*` label per PR.
- Use branch name pattern `type/description` with lowercase safe chars.
- Follow Conventional Commit format.
- Ensure required automated checks pass before merge.
- Do not use AI attribution trailers in commits.

### skill-creator
- Create skills only for reusable, non-trivial patterns.
- Keep frontmatter complete (`name`, `description+Trigger`, `license`, `metadata`).
- Put actionable rules in Critical Patterns, keep examples minimal.
- Use local references/assets, avoid web-only dependency in skill docs.
- Follow naming conventions (`{tech}`, `{project-component}`, `{action-target}`).
- Register new skills in AGENTS index.

### go-testing
- Prefer table-driven tests for behavior matrices.
- Test Bubbletea model transitions directly via `Update`.
- Use teatest for interactive TUI flows.
- Use golden files for stable view output snapshots.
- Cover success/error paths and edge cases explicitly.
- Use `t.TempDir()` and mocks for side-effect isolation.

### judgment-day
- Launch two blind judges in parallel, never sequential.
- Synthesize confirmed vs suspect findings before fixing.
- Classify warnings: real vs theoretical; only real warnings block.
- Fix only confirmed issues, then re-judge in parallel.
- After two fix rounds with remaining blockers, ask user before continuing.
- Never approve without required clean convergence criteria.

## Project Conventions

| File | Path | Notes |
|------|------|-------|
| — | — | No project-level convention files found in /opt/nt-cli |

Read the convention files listed above for project-specific patterns and rules. All referenced paths have been extracted — no need to read index files to discover more.
