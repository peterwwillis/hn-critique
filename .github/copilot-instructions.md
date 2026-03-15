# Copilot instructions

When completing a series of operations (especially after resolving conflicts or rebasing), always verify mergeability before closing the task:

1. Fetch main and ensure history is complete: `git fetch origin main:refs/remotes/origin/main` (use `git fetch --unshallow origin` if needed).
2. Check for conflicts against main:
   - `git merge-base HEAD origin/main`
   - `git merge-tree $(git merge-base HEAD origin/main) HEAD origin/main`
3. Scan for conflict markers in the working tree: `rg '<<<<<<<|=======|>>>>>>>'`.
4. If conflicts are found, resolve them, re-run the checks above, and only then finalize.
