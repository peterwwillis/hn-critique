# Design Notes: Content Limits

This document describes the limits applied throughout the hn-critique pipeline,
their impact on output quality, and recommendations for improving coverage.

---

## Overview of Limits

The crawler applies several size caps at different stages of processing. When
content exceeds a cap it is **silently truncated**, which can result in
incomplete or less accurate AI critiques. The limits are defined in
`internal/config/config.go` and can be overridden per-model in
`hn-critique.toml`.

### Default limits (all models except `openai/gpt-4.1-mini`)

| Parameter | Default | Description |
|---|---|---|
| `article_body_bytes` | 2 097 152 (2 MiB) | Maximum raw HTML body downloaded from the article URL. |
| `article_text_chars` | 8 000 | Maximum characters of plain text extracted from the HTML. |
| `article_prompt_bytes` | 6 000 | Maximum bytes of article text injected into the AI prompt. |
| `comment_prompt_bytes` | 20 000 | Maximum bytes of formatted comment text injected into the AI prompt. |
| `comment_depth` | 3 | Maximum recursive depth when fetching nested comments. |
| `top_comments` | 20 | Maximum number of top-level comments fetched per story. |
| `child_comments` | 5 | Maximum number of child comments fetched per parent comment. |

### Limits for `openai/gpt-4.1-mini` (and alias `gpt-4.1-mini`)

| Parameter | Value |
|---|---|
| `article_body_bytes` | 4 194 304 (4 MiB) |
| `article_text_chars` | 50 000 |
| `article_prompt_bytes` | 50 000 |
| `comment_prompt_bytes` | 200 000 |
| `comment_depth` | 4 |
| `top_comments` | 40 |
| `child_comments` | 10 |

---

## Processing Pipeline and Where Truncation Occurs

```
Article URL
  │
  ▼
[HTTP fetch]  ──── capped at article_body_bytes (raw HTML body)
  │
  ▼
[HTML text extraction]  ──── capped at article_text_chars (plain-text characters)
  │
  ▼
[AI article prompt]  ──── capped at article_prompt_bytes (bytes sent to model)
  │
  ▼
[AI model generates ArticleCritique]

HN API (comments)
  │
  ▼
[comment fetch]  ──── capped at top_comments / child_comments / comment_depth
  │
  ▼
[AI comments prompt]  ──── capped at comment_prompt_bytes (bytes sent to model)
  │
  ▼
[AI model generates CommentsCritique]
```

Truncation at any stage can produce an incomplete or less accurate result:

- **`article_body_bytes`**: If the raw HTML body is cut short, the text
  extraction step may miss sections of the article entirely.
- **`article_text_chars`**: If extracted plain text is truncated, the article
  may be analysed without its concluding argument, methodology section, or
  references.
- **`article_prompt_bytes`**: A second truncation pass applied inside the AI
  prompt builder. If `article_text_chars` is larger than
  `article_prompt_bytes`, the model only sees a portion of the extracted text.
- **`comment_prompt_bytes`**: Long comment threads may be cut mid-comment or
  after only a few comments, causing the model to miss significant discussion.
- **`top_comments` / `child_comments` / `comment_depth`**: Comment counts and
  depth directly limit how much of the discussion the model sees. Popular
  stories on Hacker News can have hundreds of top-level comments; only the
  first `top_comments` are fetched.

---

## Impact on Ratings and Critiques

| Limit exceeded | Effect |
|---|---|
| Article text truncated | Summary and main-points may be incomplete; truthfulness assessment based on partial content; rating may be under- or over-conservative. |
| Article prompt truncated | Model only sees a subset of already-extracted text; same risks as above, often worse because this is the final cut. |
| Comment prompt truncated | Model may miss the most-informative comments; overall discussion summary skewed toward the first few comments. |
| Top/child comments capped | Popular stories with many high-quality replies analysed with only a fraction of the discussion. |

---

## Recommendations

### Raise limits when possible

The default limits are conservative and intended to work within small model
context windows. If you are using a model with a large context window (e.g.
`openai/gpt-4.1-mini`, `gpt-4o`, or any model with ≥ 128 k tokens), raise
the limits to maximise coverage:

```toml
[models."my-model"]
[models."my-model".limits]
article_text_chars   = 50000
article_prompt_bytes = 50000
comment_prompt_bytes = 200000
top_comments         = 40
child_comments       = 10
comment_depth        = 4
```

### Align `article_text_chars` with `article_prompt_bytes`

A common misconfiguration is setting `article_text_chars` much higher than
`article_prompt_bytes`. The text extractor stores up to `article_text_chars`
characters but the prompt builder then silently truncates to
`article_prompt_bytes`. Setting them to the same value ensures a single,
predictable truncation point.

### Choosing values relative to model context

As a rough guide, one token ≈ 4 bytes for English text. Common context limits:

| Model | Context tokens | Safe article bytes | Safe comment bytes |
|---|---|---|---|
| `gpt-4o-mini` | 128 k | ~50 000 | ~200 000 |
| `gpt-4.1-mini` | 1 M | ~400 000 | ~1 600 000 |
| `llama3.2` (3B) | 128 k | ~50 000 | ~200 000 |
| `mistral` (7B) | 32 k | ~12 000 | ~50 000 |

Subtract overhead for the prompt template itself (≈ 600 bytes for articles,
≈ 700 bytes for comments) and reserve tokens for the model's JSON output
(≈ 1 000–2 000 tokens).

---

## Operational Awareness

Starting with the version that implemented this design document, the crawler
emits log warnings whenever content is truncated:

```
  ⚠  article text truncated: fetched 8000 chars (limit: 8000); critique may be incomplete
  ⚠  article prompt truncated: text is 9500 bytes, limit is 6000; critique may be incomplete
  ⚠  comment prompt truncated: total comment text ~25000 bytes exceeds limit of 20000; comments critique may be incomplete
```

At the end of each run, the crawler prints a summary of all stories whose
analysis may be affected:

```
=== Incomplete Results Summary ===
2 stories may have incomplete analysis due to content truncation.
Review the following pages for potential inaccuracies:
  [3] "Some Long Article" (example.com)
      Reasons: article text truncated, article prompt truncated
      Critique: https://peterwwillis.github.io/hn-critique/critique/2024/01/15/12345.html
      Comments: https://peterwwillis.github.io/hn-critique/comments/2024/01/15/12345.html
```

Use the `-site-url` flag on the crawler to customise the base URL used in the
summary (defaults to `https://peterwwillis.github.io/hn-critique`).
