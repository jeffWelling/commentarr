// Package importer orchestrates the post-download pipeline: validate,
// classify, evaluate safety rules, place, trash, emit webhook events.
package importer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/jeffWelling/commentarr/internal/classify"
	"github.com/jeffWelling/commentarr/internal/metrics"
	"github.com/jeffWelling/commentarr/internal/placer"
	"github.com/jeffWelling/commentarr/internal/safety"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/trash"
	"github.com/jeffWelling/commentarr/internal/validate"
	"github.com/jeffWelling/commentarr/internal/webhook"
)

// Outcome categorizes an import's terminal state.
type Outcome string

const (
	OutcomeSuccess         Outcome = "success"
	OutcomeSafetyViolation Outcome = "safety_violation"
	OutcomeNonCompliant    Outcome = "non_compliant"
	OutcomeError           Outcome = "error"
)

// Deps are the services the Importer needs.
type Deps struct {
	Classify    *classify.Service
	Placer      *placer.Placer
	Trash       *trash.Trash
	Webhook     *webhook.Dispatcher
	SafetyCfg   safety.BuiltinConfig
	SafetyRules []safety.CompiledRule
	Library     string
}

// Request describes one import to perform.
type Request struct {
	NewFilePath      string
	OriginalFilePath string
	TitleID          string
	Title            string
	Year             string
	Edition          string
}

// Result describes what the import did.
type Result struct {
	Outcome     Outcome
	FinalPath   string
	TrashedPath string
	Violations  []safety.Violation
	Error       error
}

// Importer is the orchestrator.
type Importer struct {
	deps Deps
}

// New returns an Importer.
func New(deps Deps) *Importer { return &Importer{deps: deps} }

// Import runs the full pipeline on req.
func (im *Importer) Import(ctx context.Context, req Request) (Result, error) {
	r := Result{}

	// 1. Validate.
	vr, err := validate.ValidateFile(req.NewFilePath, validate.DefaultAllowList())
	if err != nil {
		var ve *validate.ValidationError
		if errors.As(err, &ve) {
			metrics.NonCompliantFilesTotal.WithLabelValues(string(ve.Reason), filepath.Ext(req.NewFilePath)).Inc()
			r.Outcome = OutcomeNonCompliant
			r.Error = err
			return r, err
		}
		r.Outcome = OutcomeError
		r.Error = err
		return r, err
	}

	// 2. Classify newly downloaded file.
	t := title.Title{ID: req.TitleID, FilePath: req.NewFilePath, DisplayName: req.Title}
	verdict, err := im.deps.Classify.ClassifyTitle(ctx, t)
	if err != nil {
		r.Outcome = OutcomeError
		r.Error = fmt.Errorf("post-download classify: %w", err)
		return r, r.Error
	}

	// 3. Build facts + evaluate safety.
	facts := safety.Facts{
		ClassifierConfidence:           verdict.Confidence,
		ClassifierCommentaryTrackCount: boolToInt(verdict.HasCommentary),
		Container:                      vr.Container,
		FileMagicMatchesExtension:      true, // validate passed
		FileSizeBytes:                  vr.SizeBytes,
	}
	builtinRes := safety.EvaluateBuiltin(facts, im.deps.SafetyCfg)
	celRes := safety.EvaluateCEL(facts, im.deps.SafetyRules)

	all := append([]safety.Violation{}, builtinRes.Violations...)
	all = append(all, celRes.Violations...)
	r.Violations = all

	for _, v := range all {
		metrics.SafetyViolationsTotal.WithLabelValues(v.Rule).Inc()
	}

	if len(all) > 0 {
		r.Outcome = OutcomeSafetyViolation
		_ = im.deps.Webhook.Dispatch(ctx, webhook.EventSafetyViolation, map[string]interface{}{
			"title_id": req.TitleID, "violations": all,
		})
		metrics.ImportsTotal.WithLabelValues(im.deps.Library, "", "safety_violation").Inc()
		return r, fmt.Errorf("safety violations: %d", len(all))
	}

	// 4. Place.
	placeRes, err := im.deps.Placer.Place(placer.PlaceRequest{
		NewFilePath:      req.NewFilePath,
		OriginalFilePath: req.OriginalFilePath,
		Title:            req.Title,
		Year:             req.Year,
		Edition:          req.Edition,
		Container:        vr.Container,
	})
	if err != nil {
		r.Outcome = OutcomeError
		r.Error = err
		return r, err
	}
	r.FinalPath = placeRes.FinalPath
	r.TrashedPath = placeRes.TrashedPath

	// 5. Record trash.
	if placeRes.TrashedPath != "" {
		_ = im.deps.Trash.Record(ctx, im.deps.Library, req.OriginalFilePath, placeRes.TrashedPath, "replace")
		_ = im.deps.Webhook.Dispatch(ctx, webhook.EventTrash, map[string]interface{}{
			"library": im.deps.Library, "trashed_path": placeRes.TrashedPath,
		})
	}

	// 6. Webhooks for import + replace.
	_ = im.deps.Webhook.Dispatch(ctx, webhook.EventImport, map[string]interface{}{
		"title_id": req.TitleID, "final_path": placeRes.FinalPath, "mode": string(placeRes.Mode),
	})
	if placeRes.Mode == placer.ModeReplace {
		_ = im.deps.Webhook.Dispatch(ctx, webhook.EventReplace, map[string]interface{}{
			"title_id": req.TitleID, "trashed_path": placeRes.TrashedPath,
		})
		metrics.ReplacesTotal.WithLabelValues(im.deps.Library, "success").Inc()
	}

	metrics.ImportsTotal.WithLabelValues(im.deps.Library, string(placeRes.Mode), "success").Inc()
	r.Outcome = OutcomeSuccess
	return r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
