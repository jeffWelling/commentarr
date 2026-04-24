# Safety Rules Reference

Every time Commentarr is about to import a downloaded release, it
evaluates a **safety profile** — a set of rules that must pass before
the file is placed into your library. If a rule fails, the import is
either blocked or downgraded (depending on the rule's action), and the
failure is recorded on the title.

There are two layers of rules:

1. **Built-in rules** — toggles in the config (or Helm values). These
   cover the four checks that almost every user wants.
2. **CEL expressions** — arbitrary boolean expressions against a
   well-known set of facts. This is where you express policies
   Commentarr's maintainers haven't anticipated.

## Built-in rules

| Toggle | What it checks | Reason to disable |
|---|---|---|
| `ClassifierConfidenceThreshold` | Classifier confidence on the commentary track ≥ N | You're manually reviewing every import. |
| `RequireAudioTracksGE`          | New file has ≥ original audio-track count | Original had a bloated track list you're deliberately replacing. |
| `RequireVideoBitratePct`        | New/original video-bitrate ratio ≥ `VideoBitrateMinRatio` (default 0.8) | Replacing a remux with an encode deliberately. |
| `RequireMagicMatch`             | File magic bytes match extension | You trust your sources and want faster imports. |

Built-in rules fail with an internal name (`classifier_confidence`,
`audio_track_count`, `video_bitrate`, `file_magic_matches_extension`)
so operators can grep logs and metrics.

## CEL rules

Commentarr uses [Google's CEL](https://cel.dev/) — a small expression
language with no side effects and strong types. Rules are short boolean
expressions written against the `Facts` struct below; returning `true`
means "passed."

A rule has three fields:

| Field | Type | Meaning |
|---|---|---|
| `name`   | string | Human-readable identifier, used in logs & metrics. |
| `expr`   | string | The CEL expression itself. |
| `action` | enum   | What happens on failure: `block_import`, `block_replace`, `warn`, `log_only`. |

### Fact schema

All facts are available as top-level CEL variables:

```
classifier_confidence             double    0.0..1.0
classifier_commentary_track_count int       count of tracks labelled commentary

audio_track_count                 int       in the new file
original_audio_track_count        int       in the file being replaced (0 if no original)

video_bitrate_mbps                double    new file, probed
original_video_bitrate_mbps       double    original file (0 if unknown)

container                         string    e.g. "mkv", "mp4", "avi"
file_magic_matches_extension      bool      magic-byte check agreed with extension
file_size_bytes                   int

release_title                     string    raw title from the indexer
release_group                     string    scene group (parsed, may be "")
indexer                           string    indexer label
seeders                           int
duration_seconds                  double    ffprobe container duration
```

### Action semantics

- **`block_import`** — abort the import; new file is moved nowhere. The
  title stays in its previous state (usually `wanted`). Use this for
  hard safety violations.
- **`block_replace`** — import proceeds but forces `sidecar` placement
  instead of `replace`. Original is preserved. This is the default
  action when you want "be safer than my configured placement mode."
- **`warn`** — import proceeds normally, but an `import.verdict_degraded`
  webhook fires and the rule is recorded on the title's audit log.
- **`log_only`** — identical to `warn` minus the webhook. Useful while
  tuning a new rule.

### Examples

**Reject low-confidence classifications.**

```yaml
- name: low_confidence
  expr: classifier_confidence >= 0.85
  action: block_import
```

**Never replace an MKV with an AVI, even if confidence is high.**

```yaml
- name: no_mkv_to_avi_downgrade
  expr: "!(original_audio_track_count > 0 && container == 'avi')"
  action: block_replace
```

**Warn when the release title doesn't mention commentary-adjacent
terminology.**

```yaml
- name: title_mentions_commentary_signal
  expr: |
    release_title.matches('(?i)(criterion|special[\\s._-]*edition|director[s\\s._-]*cut|commentary)')
  action: warn
```

**Block releases from indexers with zero seeders.**

```yaml
- name: dead_torrent
  expr: seeders >= 1
  action: block_import
```

**Block when duration differs from the original by more than 2 minutes**
(titles with alternate cuts show up all the time — this guard prevents
importing an alt cut and calling it the commentary version).

```yaml
- name: duration_drift
  expr: |
    !(original_video_bitrate_mbps > 0) ||
    duration_seconds >= 0 &&
    (duration_seconds > (3000.0 - 120.0) && duration_seconds < (3000.0 + 120.0))
  action: block_replace
```

### Tips

- Keep expressions short. CEL supports `&&`, `||`, `!`, arithmetic,
  string methods (`startsWith`, `endsWith`, `contains`, `matches`), and
  list/map membership — but compiling rules takes time, so deeply
  nested boolean trees slow your import pipeline.
- Every rule compiles at startup. A syntactically-invalid rule will
  prevent Commentarr from starting — check `commentarr_safety_compile_errors_total`.
- Rules are evaluated independently. If three rules fail, three
  violations are recorded and three actions apply (the most restrictive
  wins).

### Metrics

```
commentarr_safety_rule_evaluations_total{rule="…", result="pass|fail"}
commentarr_safety_compile_errors_total{rule="…"}
```

### Debugging

The `import.rejected` webhook payload includes the full list of
violations:

```json
{
  "event": "import.rejected",
  "timestamp": "2026-04-23T22:15:00Z",
  "data": {
    "title_id": "abc123",
    "release_title": "The.Departed.2006.Criterion.BluRay.1080p.x264-FOO",
    "violations": [
      { "rule": "low_confidence", "detail": "" },
      { "rule": "no_mkv_to_avi_downgrade", "detail": "" }
    ]
  }
}
```

If the CEL engine itself errors (type mismatch, bad method call), the
violation has a non-empty `detail` and the rule is reported as failed.
Fix the expression and push it to the API — no restart needed; the
profile repo recompiles on save.
