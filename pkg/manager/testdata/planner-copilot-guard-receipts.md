# Planner Copilot credential guard: removal-red / restored-green receipts

Control for `pkg/manager/planner.go` (`cliLLMProvider.Complete`): the
`runner.CopilotEnv(os.Environ())` block that scopes the child environment and
refuses a classic PAT before launch.

Base commit: `742e49e1436346207a16ed12b783172514e14cac`
Go: `go1.26.5 linux/amd64`
Command (both runs): `go test ./pkg/manager -run TestPlannerCopilot -count=1 -v`

Source SHA-256:

| file | sha256 |
| --- | --- |
| `pkg/manager/planner.go` (guard present, committed) | `ad5e19e0625f834ab29d18cccd9d248715c22c2e01125d244407e9e1d3af5919` |
| `pkg/manager/planner.go` (guard block deleted, red run only) | `a82f343743eb5967539434732f962099c846b834008e5e31f9c788be0259c63d` |
| `pkg/manager/planner_copilot_test.go` (unchanged in both runs) | `642041ce62f9c061ca98f0011d6dc79beee9b5651e42d697c27b93af17864812` |

## RED: guard block removed (compiles, assertions fail)

```
=== RUN   TestPlannerCopilotScopesCredentialsAndPassesPrompt
    planner_copilot_test.go:41: vendor keys reached copilot: "sk-o|sk-a|g-key"
--- FAIL: TestPlannerCopilotScopesCredentialsAndPassesPrompt (0.00s)
=== RUN   TestPlannerCopilotRefusesClassicPATBeforeLaunch
    planner_copilot_test.go:53: err = <nil>
--- FAIL: TestPlannerCopilotRefusesClassicPATBeforeLaunch (0.00s)
FAIL
FAIL	github.com/openexec/openexec/pkg/manager	0.007s
FAIL
```

Exit status 1.

## GREEN: guard restored (`git checkout pkg/manager/planner.go`)

```
=== RUN   TestPlannerCopilotScopesCredentialsAndPassesPrompt
--- PASS: TestPlannerCopilotScopesCredentialsAndPassesPrompt (0.00s)
=== RUN   TestPlannerCopilotRefusesClassicPATBeforeLaunch
--- PASS: TestPlannerCopilotRefusesClassicPATBeforeLaunch (0.00s)
PASS
ok  	github.com/openexec/openexec/pkg/manager	0.008s
```

Exit status 0. The mutation was made with the native file-edit tool and
reverted; the committed `planner.go` is byte-identical to the base commit.
