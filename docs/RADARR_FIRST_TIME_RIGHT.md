# Getting commentary on the first download (Radarr custom formats)

By default, Commentarr is a **retrofit** tool: Radarr/Sonarr download
whatever scores highest by their rules (which don't know about
commentary tracks), and Commentarr later notices the file is missing
commentary and goes hunting for an alternate. That's two downloads
for any title that needs commentary.

You can shift some of that work to download time by teaching
Radarr's quality profiles about commentary releases. Commentarr
doesn't need to be involved at this stage — it's pure Radarr config.

## The custom format

Radarr's "custom formats" are regex / metadata rules attached to
quality profiles. Each format that matches contributes a configurable
score to the release, biasing Radarr's release picker toward (or
away from) certain release patterns. We add a format that matches
the same keywords Commentarr's title-regex verifier uses
(see `internal/verify/rules.go`).

In Radarr → **Settings → Custom Formats → Add (`+`)**:

| Field | Value |
|---|---|
| Name | `Commentary track present` |
| Conditions → Add → Release Title | regex `(?i)\bcommentary\b` |

Add a second condition for Criterion / Director's Cut / Special
Edition / Collector keywords (these correlate with commentary tracks
even when the title doesn't say "commentary" outright):

| Field | Value |
|---|---|
| Name | `Commentary edition` |
| Conditions → Add → Release Title | regex `(?i)criterion` |
| Conditions → Add → Release Title | regex `(?i)director'?s?[\s._-]*cut` |
| Conditions → Add → Release Title | regex `(?i)\bDC\b` |
| Conditions → Add → Release Title | regex `(?i)special[\s._-]*edition` |
| Conditions → Add → Release Title | regex `(?i)collector` |

(Multiple Release Title conditions on one format are OR'd by default.)

Save both formats.

## The quality profile

In Radarr → **Settings → Profiles → \<your profile\>**:

- Scroll to **Custom Formats**.
- Set **Minimum Custom Format Score** to `0`. (Don't reject
  releases with no commentary signal — they may still be the best
  available.)
- Score `Commentary track present`: **+100**.
- Score `Commentary edition`: **+50**.

These numbers are per-profile and sit alongside Radarr's quality
ranking, so a 1080p release with the commentary tag still beats a
2160p release without it (and a 1080p release with the commentary
tag also beats a 720p release with the commentary tag, because
Radarr's quality ladder still applies).

For Sonarr, the same flow works under
**Settings → Profiles → Quality**.

## What this gets you

- For titles where a commentary release exists at the moment Radarr
  picks: the commentary release is preferred, and Commentarr never
  has to retrofit.
- For titles where no commentary release exists at that moment:
  Radarr picks something else, the file lands, Commentarr scans,
  the title goes into the wanted queue, and the daemon eventually
  finds an alternate (the existing flow).
- For titles where the commentary edition appears *later* (Criterion
  releases come out years after the regular BD): Commentarr's
  periodic re-search of resolved titles eventually finds it. See
  `OPEN_QUESTIONS.md` Q8 part B.

## Limitations

- This only helps when there's a commentary-tagged release available
  at Radarr's grab time. Many modern BD releases don't tag the
  commentary in the title at all (e.g. Deadpool 2016: 177 candidates,
  3 of which had "Commentary" in the title — see the live homelab
  test report).
- The scoring is a heuristic. A poorly-named release that *does*
  have commentary won't match; a misleading release name that
  matches the regex won't actually have commentary. Commentarr's
  classifier is the source of truth — this is just a head start.
- Radarr custom formats apply to the *grab* decision. Once a title
  is in your library, Commentarr is the loop that keeps refining it.

## Why this lives in commentarr's docs

The custom-format rules deliberately mirror the same keywords
Commentarr's verifier uses
(`internal/verify/rules.go::DefaultRules`). Keep them in sync — if
we change the verifier rubric, update this page (and ideally
generate the regex list from the same source).

This is the cheapest implementation of OPEN_QUESTIONS.md Q8 part A
("can we get it right the first time"). The deeper integrations
(indexer-side proxy, native Radarr API integration) are tracked
there.
