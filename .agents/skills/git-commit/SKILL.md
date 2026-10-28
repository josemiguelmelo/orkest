---
name: git-commit
description: Prepare and create focused Git commits for Orkest changes when a commit is requested.
---

# Git commits in Orkest

Use when asked to create a Git commit.

1. Check `git status` and review the diff. Preserve unrelated changes and stage only the intended files.
2. Check for secrets and follow `AGENTS.md` verification requirements.
3. Use `<type>(<scope>): <summary>` (e.g. `fix(api): handle timeout`). Omit scope if unnecessary; mark breaking changes with `!` before `:`. Add a body only if needed.
4. Commit only when asked. Report the short hash and checks run or blocked.
