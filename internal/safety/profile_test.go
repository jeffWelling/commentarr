package safety

import (
	"context"
	"testing"

	"github.com/jeffWelling/commentarr/internal/db"
)

func newProfileRepo(t *testing.T) *ProfileRepo {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return NewProfileRepo(d)
}

func TestProfileRepo_SaveAndGetRule(t *testing.T) {
	r := newProfileRepo(t)
	ctx := context.Background()
	rule := StoredRule{
		ID: "r1", Name: "high-confidence", Expression: "classifier_confidence >= 0.85",
		Action: ActionBlockReplace, Enabled: true,
	}
	if err := r.SaveRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetRule(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Expression != rule.Expression || got.Action != rule.Action {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestProfileRepo_SaveProfile_AssignLibrary(t *testing.T) {
	r := newProfileRepo(t)
	ctx := context.Background()

	rule := StoredRule{ID: "r1", Name: "c", Expression: "classifier_confidence >= 0.5", Action: ActionBlockReplace, Enabled: true}
	if err := r.SaveRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	profile := Profile{ID: "strict", Name: "Strict", RuleIDs: []string{"r1"}}
	if err := r.SaveProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if err := r.AssignLibrary(ctx, "home-movies", "strict"); err != nil {
		t.Fatal(err)
	}

	rules, err := r.CompiledRulesForLibrary(ctx, "home-movies")
	if err != nil {
		t.Fatalf("CompiledRulesForLibrary: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "c" {
		t.Fatalf("unexpected: %+v", rules)
	}

	ok, err := rules[0].Compiled.Evaluate(Facts{ClassifierConfidence: 0.8})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected pass")
	}
}

func TestProfileRepo_CompiledRulesForLibrary_Empty(t *testing.T) {
	r := newProfileRepo(t)
	rules, err := r.CompiledRulesForLibrary(context.Background(), "none")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected empty, got %+v", rules)
	}
}

func TestProfileRepo_DisabledRulesSkipped(t *testing.T) {
	r := newProfileRepo(t)
	ctx := context.Background()
	_ = r.SaveRule(ctx, StoredRule{ID: "r-on", Name: "on", Expression: "classifier_confidence > 0.0", Action: ActionBlockReplace, Enabled: true})
	_ = r.SaveRule(ctx, StoredRule{ID: "r-off", Name: "off", Expression: "classifier_confidence > 1.0", Action: ActionBlockReplace, Enabled: false})
	_ = r.SaveProfile(ctx, Profile{ID: "p", Name: "p", RuleIDs: []string{"r-on", "r-off"}})
	_ = r.AssignLibrary(ctx, "lib", "p")

	rules, err := r.CompiledRulesForLibrary(ctx, "lib")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Name != "on" {
		t.Fatalf("disabled rule should be skipped, got %+v", rules)
	}
}
