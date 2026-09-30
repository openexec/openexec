package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	runtime "github.com/openexec/openexec/pkg/runtime"
)

var discoveryFixtures = []string{"incident-array", "empty-array", "multi-array", "single-array", "scalar"}

func discoveryFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/planner-schema/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Derive the actual public fields and tags, but not its coercing UnmarshalJSON
// method. This checks the declared scalar schema, NOT public-runtime enforcement.
// Keep this independent of the planner's historically masked diagnostic.
type discoveryScalarStory runtime.PlanStory

func TestPlannerSchemaDiscoverySchema(t *testing.T) {
	for _, name := range discoveryFixtures {
		t.Run(name, func(t *testing.T) {
			var stories []discoveryScalarStory
			err := json.Unmarshal(discoveryFixture(t, name), &stories)
			if name == "scalar" {
				if err != nil || len(stories) != 4 || stories[1].RequirementID != "REQ-001" {
					t.Fatalf("scalar control: stories=%+v err=%v", stories, err)
				}
				return
			}
			var mismatch *json.UnmarshalTypeError
			if !errors.As(err, &mismatch) || mismatch.Value != "array" ||
				mismatch.Type != reflect.TypeOf("") || mismatch.Field != "requirement_id" {
				t.Fatalf("want array/string requirement_id schema rejection; got %v", err)
			}
			t.Logf("declared scalar schema diagnostic: %v", err)
		})
	}
}

type discoveryProvider struct {
	response string
	calls    int
}

func (p *discoveryProvider) Complete(ctx context.Context, _ string) (string, error) {
	p.calls++
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// No network or inference. Bound a future faulty recovery loop as well.
	if p.calls > 8 {
		return "", errors.New("discovery fixture provider call limit exceeded")
	}
	return p.response, nil
}

func TestPlannerSchemaDiscoveryRuntime(t *testing.T) {
	for _, name := range discoveryFixtures {
		for _, shape := range []string{"array", "object"} {
			t.Run(name+"/"+shape, func(t *testing.T) {
				data := discoveryFixture(t, name)
				var expected []json.RawMessage
				if err := json.Unmarshal(data, &expected); err != nil {
					t.Fatal(err)
				}
				if shape == "object" {
					data = append(append([]byte(`{"stories":`), data...), '}')
				}
				for _, route := range []string{"decode", "generate", "compact", "refine"} {
					t.Run(route, func(t *testing.T) {
						var original runtime.ProjectPlan
						if err := json.Unmarshal(discoveryFixture(t, "scalar"), &original.Stories); err != nil {
							t.Fatal(err)
						}
						original.Goals = []runtime.PlanGoal{{ID: "G-001", Title: "Synthetic goal"}}
						provider := &discoveryProvider{response: string(data)}
						planner := runtime.NewPlanner(provider)
						var plan *runtime.ProjectPlan
						var err error
						switch route {
						case "decode":
							plan = &runtime.ProjectPlan{}
							if shape == "object" {
								err = json.Unmarshal(data, plan)
							} else {
								err = json.Unmarshal(data, &plan.Stories)
							}
						case "generate":
							plan, err = planner.GeneratePlan(context.Background(), "Synthetic fixture discovery")
						case "compact":
							plan, err = planner.GenerateCompactPlan(context.Background(), "Synthetic fixture discovery")
						case "refine":
							plan, err = planner.RefinePlan(context.Background(), "Synthetic fixture discovery", &original,
								&runtime.PlanReview{Approved: false, Assessment: "Clarify the synthetic tasks."})
						}
						if route != "decode" && (provider.calls == 0 || provider.calls > 8) {
							t.Fatalf("fixture provider calls=%d", provider.calls)
						}
						if err != nil {
							if name == "scalar" {
								t.Fatalf("scalar control failed: %v", err)
							}
							if route != "decode" && plan != nil {
								t.Fatal("planner returned a partial plan with an error")
							}
							// Deliberately do not require the legacy 'no stories found'
							// error: discovery must survive the diagnostic/recovery repair.
							t.Logf("public runtime rejected fixture: %s", strings.SplitN(err.Error(), "\n", 2)[0])
							return
						}
						if plan == nil || len(plan.Stories) != len(expected) {
							t.Fatalf("lost fixture stories: %+v", plan)
						}
						if err := plan.Validate(); err != nil {
							t.Fatal(err)
						}
						if name != "scalar" {
							// Discovery reports current coercion; it is not a recovery gate.
							t.Log("public runtime accepted schema-invalid array fixture (scalar enforcement gap)")
							return
						}
						if !reflect.DeepEqual(plan.Stories, original.Stories) {
							t.Fatalf("scalar fields changed: %+v", plan.Stories)
						}
						if route == "refine" && !reflect.DeepEqual(plan.Goals, original.Goals) {
							t.Fatal("refinement lost retained goal")
						}
						// Round-trip the actual public plan bytes, not a shadow type.
						path := t.TempDir() + "/plan.json"
						encoded, err := json.Marshal(plan)
						if err != nil {
							t.Fatal(err)
						}
						if err = os.WriteFile(path, encoded, 0600); err != nil {
							t.Fatal(err)
						}
						stored, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						var reloaded runtime.ProjectPlan
						if err = json.Unmarshal(stored, &reloaded); err != nil || !reflect.DeepEqual(plan, &reloaded) {
							t.Fatalf("public plan round trip: %v", err)
						}
					})
				}
			})
		}
	}
}
