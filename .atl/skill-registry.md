# Skill Registry

**Delegator use only.** Any agent that launches sub-agents reads this registry to resolve compact rules, then injects them directly into sub-agent prompts. Sub-agents do NOT read this registry or individual SKILL.md files.

See `_shared/skill-resolver.md` for the full resolution protocol.

## User Skills

| Trigger | Skill | Path |
|---------|-------|------|
| when drafting or posting feedback, review comments, maintainer replies, Slack messages, or GitHub comments | comment-writer | /home/ad373971/.config/opencode/skills/comment-writer/SKILL.md |
| when implementing a change, preparing commits, splitting PRs, or planning chained or stacked PRs | work-unit-commits | /home/ad373971/.config/opencode/skills/work-unit-commits/SKILL.md |
| when writing guides, READMEs, RFCs, onboarding docs, architecture docs, or review-facing documentation | cognitive-doc-design | /home/ad373971/.config/opencode/skills/cognitive-doc-design/SKILL.md |
| when a PR would exceed 400 changed lines, when planning chained PRs, stacked PRs, or reviewable slices | chained-pr | /home/ad373971/.config/opencode/skills/chained-pr/SKILL.md |
| when creating a GitHub issue, reporting a bug, or requesting a feature | issue-creation | /home/ad373971/.config/opencode/skills/issue-creation/SKILL.md |
| when creating a pull request, opening a PR, or preparing changes for review | branch-pr | /home/ad373971/.config/opencode/skills/branch-pr/SKILL.md |
| when user asks to create a new skill, add agent instructions, or document patterns for AI | skill-creator | /home/ad373971/.config/opencode/skills/skill-creator/SKILL.md |
| when writing Go tests, using teatest, or adding test coverage | go-testing | /home/ad373971/.config/opencode/skills/go-testing/SKILL.md |
| when user says "judgment day", "judgment-day", "review adversarial", "dual review", "doble review", "juzgar", "que lo juzguen" | judgment-day | /home/ad373971/.config/opencode/skills/judgment-day/SKILL.md |
| when reviewing code for security issues, auditing a PR, running a security pass, or the user asks for a security check | security-review | /home/ad373971/.config/opencode/skills/security-review/SKILL.md |
| when fixing a bug, investigating an error, diagnosing unexpected behavior, or the user reports something is broken | bug-fix | /home/ad373971/.config/opencode/skills/bug-fix/SKILL.md |
| when writing or updating documentation, adding docstrings/JSDoc/godoc, creating READMEs, documenting APIs, or the user asks to document something | doc-writer | /home/ad373971/.config/opencode/skills/doc-writer/SKILL.md |
| when implementing changes in Python with TDD, writing Python tests, or testing lambdas/AWS integrations with pytest | python-testing-tdd | /home/ad373971/.config/opencode/skills/python-testing-tdd/SKILL.md |
| when creating or updating a pull request in Azure DevOps, resolving conflicts, or verifying post-merge | azure-pr-workflow | /home/ad373971/.config/opencode/skills/azure-pr-workflow/SKILL.md |

## Compact Rules

### comment-writer
- Lead with action, keep short, explain technical why, match thread language.

### work-unit-commits
- One deliverable per commit. Tests/docs in same commit as change they verify. >400 lines → chain.

### cognitive-doc-design
- Lead with outcome. Chunk for scanning. Use tables/checklists. Make review path explicit.

### chained-pr
- Split >400 lines unless size:exception. Each slice autonomous (CI, scope, rollback, verify).

### issue-creation
- Use templates. Start status:needs-review. No PR without status:approved.

### branch-pr
- Link approved issue. One type:label. Branch = type/description. Conventional Commits.

### skill-creator
- Only for reusable patterns. Complete frontmatter. Register in index when done.

### go-testing
- Table-driven tests. teatest for TUI. Golden files for views. Cover success/error/edge.

### judgment-day
- Two blind judges in parallel. Fix confirmed only. Re-judge. Escalate after 2 rounds.

### security-review
- Group by severity. Explain WHY. Propose concrete fix. Flag clean areas for confidence.

### bug-fix
- Diagnose → Root cause → Fix → Verify. Distinguish symptom from cause. Document.

### doc-writer
- Document WHY. Match project style. Runnable examples. Flag complexity smells.

### python-testing-tdd
- RED→GREEN→REFACTOR. Runtime tests (not AST). Evidence: exact command + result.

### azure-pr-workflow
- Branch: feat|fix/<work-item-id>-<desc>. Template completo. Preflight gates: PR_BLOCKED.

## Project Conventions

| File | Path | Notes |
|------|------|-------|
| — | — | No project-level convention files found in /opt/nt-cli |

Read the convention files listed above for project-specific patterns and rules. All referenced paths have been extracted — no need to read index files to discover more.
