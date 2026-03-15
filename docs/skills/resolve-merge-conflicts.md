# Skill: Resolve merge conflicts with main

## Purpose
Provide a repeatable workflow for detecting and resolving merge conflicts between the current branch and `main`.

## Procedure
1. Fetch the latest `main` and ensure full history is available:
   - `git fetch origin main:refs/remotes/origin/main`
   - `git fetch --unshallow origin` (if `git merge-base` fails).
2. Identify conflicts without modifying files:
   - `git merge-tree $(git merge-base HEAD origin/main) HEAD origin/main`
3. Merge and resolve:
   - `git merge origin/main`
   - Resolve conflicts, then `rg '<<<<<<<|=======|>>>>>>>'` to confirm no markers remain.
4. Run the relevant tests and rebuild as needed.

## Reusable prompt
```
Fetch origin/main, find merge conflicts with git (merge-base + merge-tree), merge origin/main, resolve conflicts, remove conflict markers, run the relevant tests, and report the conflicting files you fixed.
```
