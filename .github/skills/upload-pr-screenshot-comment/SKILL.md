---
name: upload-pr-screenshot-comment
description: Post a PR comment that embeds an externally accessible screenshot image.
---

# Skill: Upload screenshot as PR comment

## Purpose
Ensure UI screenshots shared during a Copilot session are visible to reviewers by posting them as a PR comment with an externally accessible image URL.

## Procedure
1. Capture or generate the screenshot file locally.
2. Do **not** commit screenshot files to the repository.
3. Open the active PR comment composer in the GitHub web UI and attach the local image file so GitHub uploads it as a PR attachment.
4. Use the generated Markdown image link from the composer and post the comment.
5. Include a short caption explaining what the screenshot shows.

## Reusable prompt
```
Take a screenshot of the UI change, attach it in the active PR comment composer so GitHub hosts it as an attachment, and post a comment with the embedded image. Do not commit the image file.
```
