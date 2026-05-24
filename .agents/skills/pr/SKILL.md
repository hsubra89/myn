---
name: pr
description: Write GitHub pull request titles and descriptions, including draft planning PR descriptions. Use when the user asks to create, open, publish, draft, update, or rewrite a PR title or description.
---

# Create PR

Use this skill to write the PR title and description.

## PR Title

Write a short, concrete title that names the change in the vocabulary of the codebase.

- Prefer an imperative or noun-phrase title, such as `Normalize workflow node lookup responses`.
- Name the affected product concept or module when useful.
- Avoid vague titles like `Fix stuff`, `Cleanup`, or `Updates`.
- For draft planning PRs, make the planning state clear when it is not obvious from the title.

## PR Description

Every PR description must include `Problem` and `Approach`, in that order:

```md
## Problem
[What is broken, inconsistent, risky, missing, or hard to use. Explain why it matters to users, workflow authors, API consumers, maintainers, or the system.]

## Approach
[What this PR changes and why this shape was chosen. Include key contract/API decisions, migration or compatibility notes, and what is intentionally left unchanged.]
```

Do not use a summary-only body. If the PR is small, keep the required sections short.

## Notes And Observations

Add notes directly in the description when the reader should know something about this patch that is not obvious from the diff.

Use a `## Notes` section when there are multiple items or the note is important:

```md
## Notes
- [Known limitation, follow-up, intentionally excluded behavior, reviewer context, or surprising local observation.]
```

Use notes for:

- known unrelated failures or environmental issues
- behavior intentionally left unchanged
- compatibility or migration details
- reviewer context that explains why the patch is shaped this way
- follow-up work that should not block this PR

Do not add notes just to restate the diff.

## Draft Planning PRs

For a draft PR that only contains documentation for work that is about to be done:

- Problem: describe the problem the upcoming work intends to solve.
- Approach: describe the planned approach and say that the current PR only contains planning/documentation.
- Notes: call out assumptions, open questions, known risks, or implementation work that has not started.

## Quality Bar

- The Problem section should describe the user-facing or maintainer-facing issue, not just the files changed.
- The Approach section should make the design decision reviewable without reading the diff first.
- Notes should make review easier by surfacing context the diff cannot show on its own.
