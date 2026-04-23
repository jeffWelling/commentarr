// Package classify wraps commentary-classifier pipeline.ClassifyFile for
// Commentarr's needs and persists Verdicts via the title Repo.
package classify

import (
	"context"
	"fmt"
	"time"

	classifier "github.com/jeffWelling/commentary-classifier"
	"github.com/jeffWelling/commentary-classifier/pipeline"
	"github.com/jeffWelling/commentarr/internal/title"
)

// Classifier is the narrow port Service needs. The production adapter is
// a thin wrapper around pipeline.ClassifyFile; tests inject a stub.
type Classifier interface {
	ClassifyFile(path string) ([]classifier.TrackResult, error)
}

// Service orchestrates per-title classification + persistence.
type Service struct {
	repo    title.Repo
	cls     Classifier
	version string // classifier module version string, recorded on each verdict
}

// NewService returns a Service.
func NewService(repo title.Repo, cls Classifier, version string) *Service {
	return &Service{repo: repo, cls: cls, version: version}
}

// ClassifyTitle runs the classifier against the title's file and persists
// the resulting Verdict. Returns the Verdict it just saved.
//
// HasCommentary is true iff any track was recommended "commentary".
// Confidence on a positive verdict is the MAX confidence among
// commentary-recommended tracks. On a negative verdict it is the max
// confidence among non-commentary tracks (i.e. how sure we are that NO
// track is commentary).
func (s *Service) ClassifyTitle(ctx context.Context, t title.Title) (title.Verdict, error) {
	results, err := s.cls.ClassifyFile(t.FilePath)
	if err != nil {
		return title.Verdict{}, fmt.Errorf("classify %s: %w", t.FilePath, err)
	}

	verdict := title.Verdict{
		TitleID:           t.ID,
		ClassifierVersion: s.version,
		ClassifiedAt:      time.Now().UTC().Truncate(time.Second),
	}

	var maxCommentary, maxNotCommentary float64
	for _, r := range results {
		if r.Recommendation == "commentary" {
			if r.CommentaryConfidence > maxCommentary {
				maxCommentary = r.CommentaryConfidence
			}
		} else {
			if r.CommentaryConfidence > maxNotCommentary {
				maxNotCommentary = r.CommentaryConfidence
			}
		}
	}

	if maxCommentary > 0 {
		verdict.HasCommentary = true
		verdict.Confidence = maxCommentary
	} else {
		verdict.Confidence = maxNotCommentary
	}

	if err := s.repo.SaveVerdict(ctx, verdict); err != nil {
		return title.Verdict{}, fmt.Errorf("save verdict %s: %w", t.ID, err)
	}
	return verdict, nil
}

// PipelineClassifier is the production Classifier — a thin adapter around
// commentary-classifier's pipeline.ClassifyFile.
type PipelineClassifier struct {
	Config pipeline.Config
}

// NewPipelineClassifier returns a PipelineClassifier using the classifier
// module's default configuration.
func NewPipelineClassifier() *PipelineClassifier {
	return &PipelineClassifier{Config: pipeline.Default()}
}

// ClassifyFile implements Classifier by delegating to pipeline.ClassifyFile.
func (p *PipelineClassifier) ClassifyFile(path string) ([]classifier.TrackResult, error) {
	result, err := pipeline.ClassifyFile(path, p.Config)
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}
