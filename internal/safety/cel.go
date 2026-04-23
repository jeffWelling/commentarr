package safety

import (
	"fmt"

	"github.com/google/cel-go/cel"
)

// Action is what a failing rule does.
type Action string

const (
	ActionBlockReplace Action = "block_replace"
	ActionBlockImport  Action = "block_import"
	ActionWarn         Action = "warn"
	ActionLogOnly      Action = "log_only"
)

// Rule is a compiled CEL expression ready to evaluate.
type Rule struct {
	program cel.Program
}

// CompiledRule pairs a compiled rule with its name + action.
type CompiledRule struct {
	Name     string
	Compiled *Rule
	Action   Action
}

// CompileRule parses + type-checks + compiles a CEL expression. The
// expression must evaluate to a bool.
func CompileRule(expr string) (*Rule, error) {
	env, err := newEnv()
	if err != nil {
		return nil, fmt.Errorf("cel env: %w", err)
	}
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("cel compile: %w", iss.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("cel rule must return bool, got %s", ast.OutputType())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("cel program: %w", err)
	}
	return &Rule{program: prg}, nil
}

// Evaluate runs the rule against facts. (true, nil) means "passed".
func (r *Rule) Evaluate(f Facts) (bool, error) {
	out, _, err := r.program.Eval(factsToMap(f))
	if err != nil {
		return false, fmt.Errorf("cel eval: %w", err)
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("cel eval: expected bool, got %T", out.Value())
	}
	return b, nil
}

// EvaluateCEL runs every rule against facts and returns a Result with
// one Violation per failing rule.
func EvaluateCEL(f Facts, rules []CompiledRule) Result {
	var r Result
	for _, cr := range rules {
		ok, err := cr.Compiled.Evaluate(f)
		if err != nil {
			r.Violations = append(r.Violations, Violation{Rule: cr.Name, Detail: err.Error()})
			continue
		}
		if !ok {
			r.Violations = append(r.Violations, Violation{Rule: cr.Name})
		}
	}
	return r
}

// newEnv declares every Facts field as a top-level CEL variable.
func newEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("classifier_confidence", cel.DoubleType),
		cel.Variable("classifier_commentary_track_count", cel.IntType),
		cel.Variable("audio_track_count", cel.IntType),
		cel.Variable("original_audio_track_count", cel.IntType),
		cel.Variable("video_bitrate_mbps", cel.DoubleType),
		cel.Variable("original_video_bitrate_mbps", cel.DoubleType),
		cel.Variable("container", cel.StringType),
		cel.Variable("file_magic_matches_extension", cel.BoolType),
		cel.Variable("file_size_bytes", cel.IntType),
		cel.Variable("release_title", cel.StringType),
		cel.Variable("release_group", cel.StringType),
		cel.Variable("indexer", cel.StringType),
		cel.Variable("seeders", cel.IntType),
		cel.Variable("duration_seconds", cel.DoubleType),
	)
}

// factsToMap converts a Facts struct into the map shape CEL expects.
func factsToMap(f Facts) map[string]any {
	return map[string]any{
		"classifier_confidence":             f.ClassifierConfidence,
		"classifier_commentary_track_count": int64(f.ClassifierCommentaryTrackCount),
		"audio_track_count":                 int64(f.AudioTrackCount),
		"original_audio_track_count":        int64(f.OriginalAudioTrackCount),
		"video_bitrate_mbps":                f.VideoBitrateMbps,
		"original_video_bitrate_mbps":       f.OriginalVideoBitrateMbps,
		"container":                         f.Container,
		"file_magic_matches_extension":      f.FileMagicMatchesExtension,
		"file_size_bytes":                   int64(f.FileSizeBytes),
		"release_title":                     f.ReleaseTitle,
		"release_group":                     f.ReleaseGroup,
		"indexer":                           f.Indexer,
		"seeders":                           int64(f.Seeders),
		"duration_seconds":                  f.DurationSeconds,
	}
}
