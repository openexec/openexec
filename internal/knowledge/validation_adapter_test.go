package knowledge

import "testing"

func TestSuggestedValidationItemsRemainAdvisory(t *testing.T) {
	impact := QueryEnvelope[ImpactResult]{Result: ImpactResult{ValidationRecommendations: []ValidationRecommendation{{Criterion: "Related tests pass", Scope: "related_tests", TestFiles: []string{"service_test.go"}, GraphPaths: []ImpactPath{{FromNodeID: "test", EdgeID: "edge", EdgeType: "tested_by", ToNodeID: "service"}}, Limitations: []string{"structural only"}}}}}
	items := SuggestedValidationItems(impact)
	if len(items) != 1 {
		t.Fatalf("unexpected items: %#v", items)
	}
	item := items[0]
	if item.Source != "graph" || item.Disposition != "suggested" || item.Requirement != "optional" || len(item.CommandArgv) != 0 {
		t.Fatalf("graph recommendation gained validation authority: %#v", item)
	}
	if len(item.GraphPaths) != 1 || len(item.Limitations) != 1 {
		t.Fatalf("recommendation lost explanation: %#v", item)
	}
}
