package safety

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jeffWelling/commentarr/internal/metrics"
)

// StoredRule is a CEL rule persisted in safety_rules.
type StoredRule struct {
	ID         string
	Name       string
	Expression string
	Action     Action
	Enabled    bool
}

// Profile is a named bundle of enabled rules.
type Profile struct {
	ID      string
	Name    string
	RuleIDs []string
}

// ProfileRepo persists safety rules + profiles + per-library assignments.
type ProfileRepo struct {
	db *sql.DB
}

// NewProfileRepo returns a ProfileRepo.
func NewProfileRepo(d *sql.DB) *ProfileRepo { return &ProfileRepo{db: d} }

// SaveRule upserts a rule.
func (p *ProfileRepo) SaveRule(ctx context.Context, r StoredRule) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO safety_rules (id, name, expression, action, enabled)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name = excluded.name,
		  expression = excluded.expression,
		  action = excluded.action,
		  enabled = excluded.enabled,
		  updated_at = CURRENT_TIMESTAMP`,
		r.ID, r.Name, r.Expression, string(r.Action), r.Enabled)
	if err != nil {
		return fmt.Errorf("save rule %s: %w", r.ID, err)
	}
	return nil
}

// ListRules returns every stored rule, ordered by id. Powers the UI
// listing.
func (p *ProfileRepo) ListRules(ctx context.Context) ([]StoredRule, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, name, expression, action, enabled FROM safety_rules ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	var out []StoredRule
	for rows.Next() {
		var r StoredRule
		var action string
		if err := rows.Scan(&r.ID, &r.Name, &r.Expression, &action, &r.Enabled); err != nil {
			return nil, err
		}
		r.Action = Action(action)
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRule removes a rule by id.
func (p *ProfileRepo) DeleteRule(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM safety_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete rule %s: %w", id, err)
	}
	return nil
}

// GetRule returns a rule by id.
func (p *ProfileRepo) GetRule(ctx context.Context, id string) (StoredRule, error) {
	var r StoredRule
	var action string
	err := p.db.QueryRowContext(ctx, `
		SELECT id, name, expression, action, enabled FROM safety_rules WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.Expression, &action, &r.Enabled)
	if err != nil {
		return StoredRule{}, fmt.Errorf("get rule %s: %w", id, err)
	}
	r.Action = Action(action)
	return r, nil
}

// SaveProfile upserts a profile.
func (p *ProfileRepo) SaveProfile(ctx context.Context, pr Profile) error {
	ids, err := json.Marshal(pr.RuleIDs)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO safety_profiles (id, name, rule_ids_json)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name = excluded.name,
		  rule_ids_json = excluded.rule_ids_json`,
		pr.ID, pr.Name, string(ids))
	if err != nil {
		return fmt.Errorf("save profile %s: %w", pr.ID, err)
	}
	return nil
}

// AssignLibrary ties a library name to a profile id.
func (p *ProfileRepo) AssignLibrary(ctx context.Context, library, profileID string) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO library_safety_profile (library, profile)
		VALUES (?, ?)
		ON CONFLICT(library) DO UPDATE SET profile = excluded.profile`,
		library, profileID)
	if err != nil {
		return fmt.Errorf("assign library %s: %w", library, err)
	}
	return nil
}

// CompiledRulesForLibrary loads the library's profile, its enabled
// rules, compiles every CEL expression, and returns them.
func (p *ProfileRepo) CompiledRulesForLibrary(ctx context.Context, library string) ([]CompiledRule, error) {
	var profileID string
	err := p.db.QueryRowContext(ctx, `
		SELECT profile FROM library_safety_profile WHERE library = ?`, library).Scan(&profileID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup library profile: %w", err)
	}

	var ruleIDsJSON string
	err = p.db.QueryRowContext(ctx, `SELECT rule_ids_json FROM safety_profiles WHERE id = ?`, profileID).Scan(&ruleIDsJSON)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	var ruleIDs []string
	if err := json.Unmarshal([]byte(ruleIDsJSON), &ruleIDs); err != nil {
		return nil, fmt.Errorf("unmarshal rule ids: %w", err)
	}

	out := make([]CompiledRule, 0, len(ruleIDs))
	for _, id := range ruleIDs {
		r, err := p.GetRule(ctx, id)
		if err != nil {
			return nil, err
		}
		if !r.Enabled {
			continue
		}
		compiled, err := CompileRule(r.Expression)
		if err != nil {
			metrics.SafetyCompileErrorsTotal.WithLabelValues(r.Name).Inc()
			return nil, fmt.Errorf("compile rule %s: %w", r.ID, err)
		}
		out = append(out, CompiledRule{Name: r.Name, Compiled: compiled, Action: r.Action})
	}
	return out, nil
}
