#!/usr/bin/env python3
"""Prove bounded workspace search guards with immutable Go source overlays."""
import json,os,pathlib,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parents[1]
env=dict(os.environ,GOWORK='off')
cases=[
 ('advertisement','workspace_tools.go','Name:        "search_files",','Name:        "unavailable_search",','TestWorkspaceSearchFindsDefinitionAndCompletesTriage','read-only triage was not offered'),
 ('dispatch','workspace_tools.go','case "search_files":','case "unavailable_search":','TestWorkspaceSearchFindsDefinitionAndCompletesTriage','search-backed triage failed'),
 ('private-subtree','workspace_search.go','if _, denied := deniedComponent(path); denied {','if _, denied := deniedComponent(path); denied && false {','TestWorkspaceSearchContainsReadsAndSkipsPrivateFiles','search leaked or missed'),
 ('opened-target','workspace_search.go','if err := refuseDeniedOpenedTarget(root, file, path, input.Path); err != nil {','if err := refuseDeniedOpenedTarget(root, file, path, input.Path); err != nil && false {','TestWorkspaceSearchContainsReadsAndSkipsPrivateFiles','search escaped'),
 ('symlink','workspace_search.go','if !entry.Type().IsRegular() {','if false && !entry.Type().IsRegular() {','TestWorkspaceSearchContainsReadsAndSkipsPrivateFiles','search leaked or missed'),
 ('output','workspace_search.go','if out.Len()+len(match)+len(searchIncomplete) > maxReadFileBytes {','if false && out.Len()+len(match)+len(searchIncomplete) > maxReadFileBytes {','TestWorkspaceSearchLimitsAndCancellation/output','missing bounded incomplete result'),
 ('bytes','workspace_search.go','if len(data) > remaining {','if false && len(data) > remaining {','TestWorkspaceSearchLimitsAndCancellation/bytes','missing bounded incomplete result'),
 ('entries','workspace_search.go','if entries > maxSearchEntries || remaining == 0 {','if false && (entries > maxSearchEntries || remaining == 0) {','TestWorkspaceSearchLimitsAndCancellation/entries','unbounded traversal'),
 ('cancel','workspace_search.go','if err := ctx.Err(); err != nil {','if err := ctx.Err(); err != nil && false {','TestWorkspaceSearchLimitsAndCancellation$','cancelled search ran'),
 ('binary','workspace_search.go','if bytes.IndexByte(data, 0) >= 0 {','if false && bytes.IndexByte(data, 0) >= 0 {','TestWorkspaceSearchContainsReadsAndSkipsPrivateFiles','search leaked or missed'),
 ('literal','workspace_search.go','if input.Text == "" || strings.ContainsAny(input.Text, "\\r\\n") {','if false && (input.Text == "" || strings.ContainsAny(input.Text, "\\r\\n")) {','TestWorkspaceSearchLimitsAndCancellation$','invalid literal was accepted'),
]
results=[]
with tempfile.TemporaryDirectory(prefix='workspace-search-controls-')as td:
 for label,name,before,after,test,assertion in cases:
  source=root/'pkg/execution'/name; original=source.read_text();assert original.count(before)==1,(label,before)
  replacement=pathlib.Path(td)/name;replacement.write_text(original.replace(before,after));overlay=pathlib.Path(td)/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(source):str(replacement)}}))
  run=subprocess.run(['go','test','-overlay',str(overlay),'./pkg/execution','-run','^'+test,'-count=1'],cwd=root,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  assert run.returncode!=0 and assertion in run.stdout,(label,run.returncode,run.stdout)
  print(label,'EXPECTED RUNTIME FAILURE',flush=True);results.append({'control':label,'assertion':assertion,'exit':run.returncode})
print(f'{len(results)} search guard removals failed their runtime assertions')
