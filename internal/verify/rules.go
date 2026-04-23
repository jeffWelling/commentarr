// Package verify scores candidate release titles by heuristic rules
// defined in DESIGN.md § 5.6. Pure functions — no I/O, no state beyond
// the passed-in rules.
package verify

import "regexp"

// Rule is one regex + score contribution. Negative scores are penalties.
type Rule struct {
	Name    string
	Pattern *regexp.Regexp
	Score   int
}

// DefaultRules returns the v1 ruleset. Users may override at runtime
// (Plan 3 or later introduces the CEL safety engine; this is a
// simpler, regex-only layer that runs pre-download).
func DefaultRules() []Rule {
	return []Rule{
		{Name: "criterion", Pattern: regexp.MustCompile(`(?i)criterion`), Score: 10},
		{Name: "commentary", Pattern: regexp.MustCompile(`(?i)commentary`), Score: 10},
		// Scene release titles use "." as a word separator ("Special.Edition")
		// so every multi-word pattern accepts [\s._-] between words.
		{Name: "special_edition", Pattern: regexp.MustCompile(`(?i)special[\s._-]*edition`), Score: 5},
		{Name: "collector", Pattern: regexp.MustCompile(`(?i)collector`), Score: 5},
		{Name: "directors_cut", Pattern: regexp.MustCompile(`(?i)director'?s?[\s._-]*cut`), Score: 5},
		{Name: "remastered", Pattern: regexp.MustCompile(`(?i)remastered`), Score: 3},
		{Name: "webrip_penalty", Pattern: regexp.MustCompile(`(?i)web[-.]?rip`), Score: -5},
		{Name: "cam_penalty", Pattern: regexp.MustCompile(`(?i)\b(cam|hdcam|telecine|ts)\b`), Score: -10},
	}
}

// Reason records one matched rule.
type Reason struct {
	Rule  string
	Score int
}

// Verdict is the output of ScoreTitle.
type Verdict struct {
	Score   int
	Reasons []Reason
}

// ScoreTitle runs every rule against title and returns the aggregate
// score plus the list of matched rules. sizeBytes is reserved for a
// future size-based heuristic (DESIGN.md notes commentary editions tend
// to bloat size by 300MB+); we don't implement that signal yet because
// we'd need the original file's size for comparison.
func ScoreTitle(title string, sizeBytes int64, rules []Rule) Verdict {
	v := Verdict{}
	for _, r := range rules {
		if r.Pattern.MatchString(title) {
			v.Score += r.Score
			v.Reasons = append(v.Reasons, Reason{Rule: r.Name, Score: r.Score})
		}
	}
	return v
}
