---
name: upload-pr-screenshot-comment
description: Post a PR comment that embeds an externally accessible screenshot image.
---

# Skill: Upload screenshot as PR comment

## Purpose
Ensure UI screenshots shared during a Copilot session are visible to reviewers by posting them as a PR comment with an externally accessible image URL.

## Procedure
1. Capture or generate the screenshot file locally.
2. Add the image to a stable repo path (for example `docs/pr-screenshots/`) on the current branch.
3. Push the branch update.
4. Find the active PR for the branch.
5. Post a PR comment that embeds the image using Markdown:
   - `![Screenshot](https://raw.githubusercontent.com/<owner>/<repo>/<branch>/<path>)`
6. Include a short caption explaining what the screenshot shows.

## Reusable prompt
```
Take a screenshot of the UI change, add it to docs/pr-screenshots on the current branch, and post a PR comment with an embedded Markdown image that reviewers can open in the browser.
```
