package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

type ToolRequest struct {
	CallID        string
	Name          string
	Input         json.RawMessage
	WorkingDir    string
	Sandbox       Sandbox
	WritableRoots []string
	// ReadableRoots may be opened but never changed, in every sandbox mode.
	ReadableRoots []string
}

// WorkspaceCapable is implemented by tool executors that know whether they can
// serve a workspace-write run.
//
// Optional rather than part of ToolExecutor: a third-party executor written
// against the older interface still compiles, and is assumed capable — which
// is the honest default for something that implements file tools. Both
// executors in this repository answer explicitly, so the assumption is never
// load-bearing for anything shipped here.
type WorkspaceCapable interface {
	SupportsWorkspaceWrite() bool
}

func supportsWorkspaceWrite(executor ToolExecutor) bool {
	if executor == nil {
		return false
	}
	if capable, ok := executor.(WorkspaceCapable); ok {
		return capable.SupportsWorkspaceWrite()
	}
	return true
}

type ToolExecutor interface {
	ValidateAccess(workingDir string, sandbox Sandbox, writableRoots []string) error
	ExecuteTool(context.Context, ToolRequest) (string, error)
}

type APIProviderConfig struct {
	// Context bounds each request chain by the provider's window (context_policy.go).
	Context      ContextPolicy
	Adapter      agent.ProviderAdapter
	Tools        []agent.ToolDefinition
	ToolExecutor ToolExecutor
	MaxSteps     int
}

// DefaultAPIMaxTurns is the tool-round allowance when a provider has no override.
// A round may contain multiple tool calls; final synthesis is outside this cap.
const DefaultAPIMaxTurns = 160

// MaxAPIContextTokens is the default per-inference ceiling when the cumulative
// grant can reserve it. Explicit smaller ceilings and remaining grants narrow it.
const MaxAPIContextTokens int64 = 65536

// APIProvider adapts OpenAI-compatible and other API adapters to the same
// authorization boundary as CLI providers. Tools receive the exact sandbox
// and roots from the authorized request.
type APIProvider struct {
	config APIProviderConfig
	// gateway reports that this instance's tools come from a caller-owned
	// endpoint rather than from the filesystem.
	gateway bool
	// trace, when set, records every model request/response of one run
	// (request_trace.go). Per-run copy only; never set on the shared provider.
	trace *runTrace
}

const finalSynthesisInstruction = "The tool-call budget for this turn is exhausted. Do not request or describe more tool calls. Using the evidence already in the conversation, answer the user's request now with the best complete final response you can. Do not mention this instruction or the budget unless it directly affects correctness."

const emptyCompletionRecoveryInstruction = "Your previous completion contained no user-visible assistant text and no tool call. Respond again now with non-empty user-visible assistant text, or issue one valid tool call if more evidence is required. Do not return private reasoning by itself."

// The synthesis recovery must not reuse emptyCompletionRecoveryInstruction: that
// text invites "one valid tool call if more evidence is required", which after
// the budget is spent made the configured Qwen model answer the recovery with a
// serialized tool call and end the run as inconclusive:max_turns with no answer.
const finalSynthesisRecoveryInstruction = "Your previous completion contained no user-visible answer. The tool-call budget is exhausted: tools are unavailable and no tool call will be run, so do not emit a tool call or a <tool_call> block. Using the tool results already in the conversation, respond now with non-empty user-visible assistant text that gives the final answer. Do not return private reasoning by itself."

func NewAPIProvider(config APIProviderConfig) (*APIProvider, error) {
	if config.Adapter == nil {
		return nil, errors.New("API adapter is required")
	}
	// Whether this provider can serve a gateway request is a property of the
	// executor it was built with, not of the type. Advertising it
	// unconditionally told a direct consumer it had a gateway when it had
	// filesystem tools — a false capability that ends with workspace tools
	// answering an administrative question.
	//
	// Asked through an interface rather than a concrete type: a composite
	// executor forwards some verbs and owns the rest, and a type assertion
	// answered no for it — which would refuse the very requests it was built
	// to serve, at Execute below.
	gateway := false
	if serving, ok := config.ToolExecutor.(interface{ servesGateway() bool }); ok {
		gateway = serving.servesGateway()
	}
	if len(config.Tools) > 0 && config.ToolExecutor == nil {
		return nil, errors.New("tool executor is required when API tools are configured")
	}
	if config.MaxSteps < 0 {
		return nil, errors.New("API provider max_turns must be positive or zero for the default")
	}
	if config.MaxSteps == 0 {
		config.MaxSteps = DefaultAPIMaxTurns
	}
	// A policy is on when the caller resolved one (ContextPolicy.WithDefaults);
	// the zero value leaves the chain unbounded, as before.
	if timed, ok := config.Adapter.(interface{ RequestTimeout() time.Duration }); ok && config.Context.ContextTokens > 0 && config.Context.RequestTimeout == 0 {
		config.Context.RequestTimeout = timed.RequestTimeout()
	}
	return &APIProvider{config: config, gateway: gateway}, nil
}

// contextPolicy is the provider's policy narrowed to what this run may send:
// the hard-token path never sends more than MaxAPIContextTokens (or the
// caller's ceiling), whatever the model's window.
func (p *APIProvider) contextPolicy() ContextPolicy {
	policy := p.config.Context
	if policy.ContextTokens <= 0 {
		policy.Disabled = true
	}
	if meter, ok := p.config.Adapter.(*boundedAPIAdapter); ok {
		limit := MaxAPIContextTokens
		if meter.contextLimit > 0 && meter.contextLimit < limit {
			limit = meter.contextLimit
		}
		policy = policy.effective(limit)
	}
	return policy
}

func (p *APIProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		ID: p.config.Adapter.GetName(), Runtime: "api", Models: p.config.Adapter.GetModels(),
		Capabilities: Capability{
			NonInferenceReadiness: true,
			NativeNonThinking:     supportsNativeNonThinking(p.config.Adapter),
			HardTokenBudget:       supportsHardTokens(p.config.Adapter),
			HardContextLimit:      supportsHardTokens(p.config.Adapter),
			// No native session to resume, and no need for one: the caller
			// replays the conversation it already persisted.
			Streaming: true, Resume: false, Replay: true, ToolGateway: p.gateway,
			Cancellation: true, ReadOnly: true,
			// Whether this provider can edit files is the executor's answer,
			// not the fact that it has one. A gateway executor has tools and
			// no filesystem, so "an executor exists" advertised workspace-write
			// to a caller that would then be refused at the turn instead of at
			// the choice. ToolCalling stays presence-based: a gateway does call
			// tools.
			WorkspaceWrite: supportsWorkspaceWrite(p.config.ToolExecutor),
			ToolCalling:    p.config.ToolExecutor != nil,
		},
	}
}

// BoundReadableRootsAvailable is the protocol's fail-closed feature check.
func (p *APIProvider) BoundReadableRootsAvailable() bool {
	_, ok := p.config.ToolExecutor.(interface {
		pinReadableRoots([]string, []ReadableRootIdentity) (ToolExecutor, func(), error)
	})
	return ok && ReadableRootIdentitySupported()
}

func (p *APIProvider) Probe(ctx context.Context, _ string) Readiness {
	models := p.config.Adapter.GetModels()
	if len(models) == 0 {
		return Readiness{State: ReadinessUnhealthy, Problem: "API provider has no configured models"}
	}
	metadata, ok := p.config.Adapter.(agent.MetadataReadiness)
	if !ok {
		return Readiness{State: ReadinessUnknown, Problem: "adapter has no non-inference metadata check; inference not tested"}
	}
	err := metadata.CheckModelMetadata(ctx, models[0])
	if err == nil {
		return Readiness{State: ReadinessReady, Check: "model-metadata"}
	}
	if errors.Is(err, agent.ErrReadinessUnsupported) {
		return Readiness{State: ReadinessUnknown, Check: "model-metadata", Problem: "endpoint metadata unavailable; inference not tested"}
	}
	state := ReadinessUnhealthy
	var providerError *agent.ProviderError
	if errors.As(err, &providerError) && providerError.Code == agent.ErrCodeAuthentication {
		state = ReadinessNeedsLogin
	}
	return Readiness{State: state, Check: "model-metadata", Problem: err.Error()}
}

func (p *APIProvider) Execute(ctx context.Context, request Request, sink EventSink) (Result, error) {
	started := time.Now().UTC()
	// The native workspace executor owns per-run readable-root handles. Keep
	// them alive until this invocation ends, including retries and synthesis.
	copy := *p
	run := &copy
	if len(request.ReadableRootIdentities) > 0 && len(request.ReadableRoots) > 0 {
		if pinner, ok := p.config.ToolExecutor.(interface {
			pinReadableRoots([]string, []ReadableRootIdentity) (ToolExecutor, func(), error)
		}); ok {
			executor, closeRoots, err := pinner.pinReadableRoots(request.ReadableRoots, request.ReadableRootIdentities)
			if err != nil {
				return Result{}, err
			}
			defer closeRoots()
			run.config.ToolExecutor = executor
		} else if len(request.ReadableRootIdentities) > 0 {
			return Result{}, errors.New("tool executor cannot bind readable root identities")
		}
	} else if len(request.ReadableRootIdentities) > 0 {
		return Result{}, errors.New("readable root identities without roots")
	}
	if trace := newRunTrace(); trace != nil {
		traced := *run
		traced.trace = trace
		run = &traced
		sink = trace.wrapSink(sink)
	}
	result, err := run.execute(ctx, request, sink)
	// Cancelled is the caller's context ending the run (its absolute budget
	// included); without it that ending left no witness at all.
	if result.Outcome == OutcomeFailed || result.Outcome == OutcomeInconclusive || result.Outcome == OutcomeCancelled {
		captureRequest(p.config.Adapter, started, result, err)
		run.trace.write(p.config.Adapter, request, result, err)
	}
	return result, err
}

func (p *APIProvider) execute(ctx context.Context, request Request, sink EventSink) (Result, error) {
	if request.NonThinking {
		if request.TokenBudget <= 0 || !supportsNativeNonThinking(p.config.Adapter) {
			return Result{}, fmt.Errorf("native non-thinking mode requires a supported adapter and hard token grant")
		}
		if err := p.config.Adapter.(agent.NativeNonThinkingAdapter).ValidateNativeNonThinking(ctx, request.Model); err != nil {
			return Result{}, err
		}
	}
	if request.ContextTokenLimit != 0 && (request.ContextTokenLimit < 2048 || request.ContextTokenLimit > MaxAPIContextTokens || request.TokenBudget <= 0) {
		return Result{}, fmt.Errorf("context token limit requires a positive hard token grant and a ceiling between 2048 and %d", MaxAPIContextTokens)
	}
	if request.TokenBudget < 0 {
		return Result{}, fmt.Errorf("negative token grant")
	}
	if request.TokenBudget > 0 {
		if !supportsHardTokens(p.config.Adapter) {
			return Result{}, fmt.Errorf("hard total-token admission unavailable")
		}
		// Each Execute owns its limiter, including all empty-response retries and
		// final synthesis. Never store a run budget on the shared provider.
		bounded := *p
		meter := &boundedAPIAdapter{ProviderAdapter: p.config.Adapter, remaining: request.TokenBudget, sink: sink, contextLimit: request.ContextTokenLimit, providerContext: p.config.Context.ContextTokens, nonThinking: request.NonThinking}
		if safe, ok := p.config.ToolExecutor.(interface{ safelyListsDirectories() bool }); ok {
			meter.trustedDirectoryResults = safe.safelyListsDirectories()
		}
		bounded.config.Adapter = meter
		request.TokenBudget = 0
		request.ContextTokenLimit = 0
		request.NonThinking = false // The run-local adapter retains it for every call/retry.
		result, err := bounded.execute(ctx, request, sink)
		var grantContext *contextGrantError
		if errors.Is(err, errHardTokenGrantExhausted) && !errors.As(err, &grantContext) {
			// Empty-response/final-synthesis recovery may wrap this refusal.
			// Preserve the existing typed transport spelling for Console's
			// per-run yield, without normalizing unrelated provider failures.
			err = errHardTokenGrantExhausted
		}
		if sink != nil {
			if e := sink(Event{Type: EventUsage, InputTokens: meter.input, OutputTokens: meter.output, UsageFinal: !meter.unknown}); err == nil {
				err = e
			}
		}
		return result, err
	}
	started := time.Now().UTC()
	result := Result{Executor: p.config.Adapter.GetName(), Model: request.Model, Sandbox: request.Sandbox, StartedAt: started, Outcome: OutcomeFailed}
	finish := func() { result.EndedAt = time.Now().UTC() }
	if sink == nil {
		sink = func(Event) error { return nil }
	}
	if err := validateProviderRequest(request); err != nil {
		finish()
		return result, err
	}
	if err := request.ValidateReplay(); err != nil {
		finish()
		return result, err
	}
	// A request naming a gateway must be served by one. Otherwise the caller
	// believes it is asking console-owned questions while the model is handed
	// the filesystem — which is what happens when the field is ignored rather
	// than refused.
	if request.ToolGateway != "" && !p.gateway {
		finish()
		return result, errors.New("this provider has no tool gateway; the request named one")
	}
	if request.NativeSessionID != "" {
		finish()
		return result, errors.New("API provider does not support native CLI session resume")
	}
	if request.Sandbox.Mode == "workspace-write" && p.config.ToolExecutor == nil {
		finish()
		return result, errors.New("API provider cannot enforce workspace-write without a bounded tool executor")
	}
	if p.config.ToolExecutor != nil {
		if err := p.config.ToolExecutor.ValidateAccess(request.WorkingDir, request.Sandbox, request.WritableRoots); err != nil {
			finish()
			return result, fmt.Errorf("tool executor cannot enforce access: %w", err)
		}
	}
	if err := sink(Event{Type: EventStarted}); err != nil {
		finish()
		return result, err
	}
	messages := replayMessages(request)
	if len(p.config.Tools) == 0 {
		stream, err := p.config.Adapter.Stream(ctx, agent.Request{
			Model: request.Model, Messages: messages, System: request.System})
		if err != nil {
			finish()
			_ = sink(Event{Type: EventFailed, Text: err.Error()})
			return result, err
		}
		for event := range stream {
			if event.Type == agent.StreamEventError {
				finish()
				if event.Error == nil {
					event.Error = errors.New("API stream failed")
				}
				_ = sink(Event{Type: EventFailed, Text: event.Error.Error()})
				return result, event.Error
			}
			if event.Delta != nil && event.Delta.Text != "" {
				result.FinalText += event.Delta.Text
				if err := sink(Event{Type: EventAssistantDelta, Text: event.Delta.Text}); err != nil {
					finish()
					return result, err
				}
			}
		}
		if err := ctx.Err(); err != nil {
			result.Outcome = OutcomeCancelled
			finish()
			_ = sink(Event{Type: EventCancelled})
			return result, err
		}
		if strings.TrimSpace(result.FinalText) == "" {
			finish()
			err := errors.New("API provider returned no assistant text")
			_ = sink(Event{Type: EventFailed, Text: err.Error()})
			return result, err
		}
		result.Outcome = OutcomeSucceeded
		finish()
		if err := sink(Event{Type: EventCompleted}); err != nil {
			return result, err
		}
		return result, nil
	}
	return p.runToolLoop(ctx, request, messages, &result, sink, finish)
}

const checkpointInstruction = "Context checkpoint: this conversation is getting long. Before you continue, call " + updateHandoverTool + " with done (what is finished), next_steps (ordered), decisions (and why) and open_questions. The harness records files, commands and test results itself. Then carry on with the task."

// sizeFailure says an endpoint refused or abandoned a request because of its
// size: an overflow the guard let through (dispatched), the endpoint's own
// context error, or a timeout of a request that was already large. A timeout of
// a small request is a different failure and is not hidden here.
func sizeFailure(err error, tokens int64, policy ContextPolicy) bool {
	var overflow *agent.ContextOverflowError
	if errors.As(err, &overflow) {
		return true
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "context") && (strings.Contains(text, "exceed") || strings.Contains(text, "too long") || strings.Contains(text, "length")) {
		return true
	}
	if strings.Contains(text, "timeout") || strings.Contains(text, "deadline exceeded") || strings.Contains(text, "awaiting headers") {
		return tokens > policy.softTokens()/2
	}
	return false
}

// runToolLoop is the tool loop of Execute: rounds of model request and tool
// calls inside request chains. A chain ends at the policy's hard threshold and
// the next starts from the handover alone; a checkpoint is progress, never a
// failure, and does not count as a tool round.
func (p *APIProvider) runToolLoop(ctx context.Context, request Request, messages []agent.Message, result *Result, sink EventSink, finish func()) (Result, error) {
	var state *runState
	policy := p.contextPolicy()
	if !policy.Disabled {
		state = newRunState(request, policy)
		if state.load() {
			messages = state.freshMessages(false)
			_ = sink(Event{Type: EventCheckpoint, Text: "resumed from the handover of an earlier pass: " + state.path()})
		}
	}
	askedHandover, askedCheckpoint, fresh := false, false, state != nil && len(messages) == 1 && state.data.Chains > 0
	// MaxChains bounds the restarts of this Execute. The handover carries the
	// run's chain count across passes, so the bound is read from where this
	// pass began: compared with the run's total, every pass after the first
	// got one chain and ended at its first hard threshold (Solar df74566c
	// T-US-CS29AB35C-001-002, 11-12: pass 1 nine chains in 45 min, passes 2
	// to 4 one chain each, about a minute apart, then the task failed).
	passChains := 0
	if state != nil {
		passChains = state.data.Chains
	}
	toolsNow := func() []agent.ToolDefinition {
		tools := p.config.Tools
		if state != nil && askedHandover {
			tools = append(append([]agent.ToolDefinition(nil), tools...), handoverToolDefinition())
		}
		if state != nil && len(state.data.Outputs) > 0 {
			tools = append(append([]agent.ToolDefinition(nil), tools...), readSavedDefinition())
		}
		return tools
	}
	endCheckpoint := func(why string) (Result, error) {
		message := fmt.Sprintf("Context checkpoint (%s). The handover is saved at %s; the next pass continues from it.", why, state.path())
		_ = state.save("checkpointed")
		result.FinalText += message
		result.Outcome, result.Reason = OutcomeInconclusive, ReasonContextCheckpoint
		finish()
		_ = sink(Event{Type: EventInconclusive, Text: message, Reason: ReasonContextCheckpoint})
		return *result, nil
	}
	restart := func(why string) bool {
		if fresh || state.data.Chains-passChains >= policy.MaxChains {
			return false
		}
		messages = state.freshMessages(false)
		askedCheckpoint, fresh = false, true
		_ = sink(Event{Type: EventCheckpoint, Text: "request chain ended (" + why + "); continuing from " + state.path()})
		return true
	}
	rounds, localOnly := 0, 0
	for rounds < p.config.MaxSteps {
		if err := ctx.Err(); err != nil {
			result.Outcome = OutcomeCancelled
			finish()
			_ = sink(Event{Type: EventCancelled})
			return *result, err
		}
		var tokens int64
		if state != nil {
			var size int64
			tokens, size = estimateRequest(agent.Request{Model: request.Model, Messages: messages, System: request.System, Tools: toolsNow()})
			verdict, why := state.policy.admit(tokens, size)
			if verdict == admitHard {
				// Never dispatch a request that cannot finish: restart from the
				// handover, or end the pass cleanly when that is already the request.
				if !restart(why) {
					return endCheckpoint(why)
				}
				continue
			}
			if verdict == admitSoft && !askedCheckpoint {
				askedCheckpoint, askedHandover = true, true
				state.mu.Lock()
				state.data.Checkpoints++
				_ = state.save("")
				state.mu.Unlock()
				messages = append(messages, agent.NewTextMessage(agent.RoleUser, checkpointInstruction))
				_ = sink(Event{Type: EventCheckpoint, Text: "checkpoint (" + why + "); handover at " + state.path()})
			}
		}
		response, err := p.completeWithEmptyRecovery(ctx, agent.Request{
			Model: request.Model, Messages: messages, System: request.System,
			Tools: toolsNow(), ToolChoice: "auto",
		})
		if err != nil {
			if state != nil && ctx.Err() == nil && sizeFailure(err, tokens, state.policy) {
				if !restart("the endpoint refused or abandoned the request for its size") {
					return endCheckpoint("the endpoint refused the request for its size")
				}
				continue
			}
			finish()
			_ = sink(Event{Type: EventFailed, Text: err.Error()})
			return *result, err
		}
		fresh = false
		assistant := agent.Message{Role: agent.RoleAssistant, Content: response.Content, Metadata: response.Metadata}
		messages = append(messages, assistant)
		wroteText := false
		for _, block := range response.Content {
			if block.Type == agent.ContentTypeText && block.Text != "" {
				if strings.TrimSpace(block.Text) != "" {
					wroteText = true
					if state != nil {
						state.noteText(block.Text)
					}
				}
				result.FinalText += block.Text
				if err := sink(Event{Type: EventAssistantDelta, Text: block.Text}); err != nil {
					finish()
					return *result, err
				}
			}
		}
		calls := response.GetToolCalls()
		if len(calls) == 0 {
			if !wroteText {
				finish()
				err := errors.New("API provider returned neither assistant text nor tool calls")
				_ = sink(Event{Type: EventFailed, Text: err.Error()})
				return *result, err
			}
			if meter, ok := p.config.Adapter.(*boundedAPIAdapter); ok {
				if note := meter.omissionLimitation(false); note != "" {
					result.FinalText += note
					if err := sink(Event{Type: EventAssistantDelta, Text: note}); err != nil {
						finish()
						return *result, err
					}
				}
			}
			result.Outcome = OutcomeSucceeded
			finish()
			state.closeOut("complete")
			if err := sink(Event{Type: EventCompleted}); err != nil {
				return *result, err
			}
			return *result, nil
		}
		external := false
		for _, call := range calls {
			event := Event{CallID: call.ToolUseID, ToolName: call.ToolName, Data: call.ToolInput}
			event.Type = EventToolProposed
			if err := sink(event); err != nil {
				finish()
				return *result, err
			}
			event.Type = EventToolStarted
			if err := sink(event); err != nil {
				finish()
				return *result, err
			}
			var output string
			var toolErr error
			scanComplete, recallFailed := false, false
			switch meter, bounded := p.config.Adapter.(*boundedAPIAdapter); {
			case state != nil && call.ToolName == updateHandoverTool:
				output, toolErr = state.update(call.ToolInput)
			case state != nil && call.ToolName == readSavedTool:
				output, toolErr = state.readSaved(call.ToolInput)
			case call.ToolName == recallTool:
				// Recall is the only way omitted evidence is read back; a failed
				// or unavailable one ends the run instead of letting the model
				// carry on as if it had seen that evidence.
				if bounded && meter.prior != nil {
					var page recalledPage
					output, page, toolErr = meter.prior.page(call.ToolInput)
					if toolErr == nil {
						meter.noteRecall(page)
					}
				} else {
					toolErr = errors.New("recall unavailable: no omitted result is retained in this run")
				}
				recallFailed = toolErr != nil
				external = true
			default:
				external = true
				toolRequest := ToolRequest{
					CallID: call.ToolUseID, Name: call.ToolName, Input: call.ToolInput,
					WorkingDir: request.WorkingDir, Sandbox: request.Sandbox,
					WritableRoots: append([]string(nil), request.WritableRoots...),
					ReadableRoots: append([]string(nil), request.ReadableRoots...),
				}
				if scanner, ok := p.config.ToolExecutor.(scanReportingExecutor); ok {
					output, scanComplete, toolErr = scanner.executeToolScan(ctx, toolRequest)
				} else {
					output, toolErr = p.config.ToolExecutor.ExecuteTool(ctx, toolRequest)
				}
			}
			event.Type, event.Text = EventToolCompleted, output
			if toolErr != nil {
				event.Text = toolErr.Error()
			}
			if err := sink(event); err != nil {
				finish()
				return *result, err
			}
			if recallFailed {
				message := "Stopped: recall of omitted tool-result evidence failed (" + toolErr.Error() + "); no further request was dispatched."
				result.FinalText += message
				result.Outcome = OutcomeFailed
				finish()
				_ = sink(Event{Type: EventFailed, Text: message})
				return *result, errors.New(message)
			}
			inRequest := output
			if state != nil {
				state.recordCall(call.ToolName, call.ToolInput, output, toolErr)
				if toolErr == nil {
					inRequest = state.compactResult(call.ToolName, call.ToolInput, output)
				}
			}
			resultMessage := agent.NewToolResultMessage(call.ToolUseID, inRequest, toolErr)
			// The mark travels with this exact result. A handover-compacted
			// result no longer holds the full scan, so it never carries it.
			resultMessage.Content[0].ScanComplete = scanComplete && toolErr == nil && inRequest == output
			messages = append(messages, resultMessage)
		}
		// Updating the handover or reading a saved output is not a tool round;
		// a model that does only that for long is, so the allowance still ends.
		if external {
			rounds, localOnly = rounds+1, 0
		} else if localOnly++; localOnly > 8 {
			rounds, localOnly = rounds+1, 0
		}
	}
	// Reaching the tool budget means research must stop, not that the work the
	// model already did should be discarded. Ask once more with tool choice
	// disabled so the model must synthesize the evidence it has. Definitions
	// stay present while the conversation holds tool calls, because strict
	// compatible endpoints validate historical tool calls against the tools
	// array; a conversation rebuilt from the handover holds none and offers
	// no tools at all.
	if err := ctx.Err(); err != nil {
		result.Outcome = OutcomeCancelled
		finish()
		_ = sink(Event{Type: EventCancelled})
		return *result, err
	}
	system := strings.TrimSpace(request.System)
	if system != "" {
		system += "\n\n"
	}
	system += finalSynthesisInstruction
	synthTools := p.config.Tools
	rebuilt := false
	rebuild := func() {
		messages, synthTools, rebuilt = state.freshMessages(true), nil, true
		_ = sink(Event{Type: EventCheckpoint, Text: "final synthesis from the handover: " + state.path()})
	}
	if state != nil {
		tokens, size := estimateRequest(agent.Request{Messages: messages, System: system, Tools: synthTools})
		if verdict, _ := state.policy.admit(tokens, size); verdict != admitOK {
			rebuild()
		}
	}
	choice := "none"
	response, err := p.completeFinalSynthesis(ctx, agent.Request{
		Model: request.Model, Messages: messages, System: system,
		Tools: synthTools, ToolChoice: choice,
	})
	if err != nil && state != nil && !rebuilt && ctx.Err() == nil && sizeFailure(err, 1<<40, state.policy) {
		rebuild()
		response, err = p.completeFinalSynthesis(ctx, agent.Request{
			Model: request.Model, Messages: messages, System: system, Tools: synthTools, ToolChoice: choice,
		})
	}
	if err != nil {
		var contextOverflow *agent.ContextOverflowError
		if errors.Is(err, errHardTokenGrantExhausted) || errors.As(err, &contextOverflow) {
			// The tool-round limit does not erase a pre-dispatch capacity
			// refusal, including one wrapped by synthesis recovery. Let the
			// outer limiter emit finalized usage and the existing yield signal.
			result.Outcome = OutcomeFailed
			finish()
			_ = sink(Event{Type: EventFailed, Text: err.Error()})
			return *result, err
		}
		message := fmt.Sprintf("API tool loop reached %d rounds and final synthesis failed: %v", p.config.MaxSteps, err)
		result.FinalText += message
		result.Outcome, result.Reason = OutcomeInconclusive, ReasonMaxTurns
		finish()
		state.closeOut("complete")
		_ = sink(Event{Type: EventInconclusive, Text: message, Reason: ReasonMaxTurns})
		return *result, nil
	}
	wroteText := false
	for _, block := range response.Content {
		if block.Type != agent.ContentTypeText || block.Text == "" {
			continue
		}
		wroteText = true
		result.FinalText += block.Text
		if err := sink(Event{Type: EventAssistantDelta, Text: block.Text}); err != nil {
			finish()
			return *result, err
		}
	}
	if meter, ok := p.config.Adapter.(*boundedAPIAdapter); ok && wroteText && rebuilt {
		// Final synthesis is never compacted, so an unrebuilt one holds the
		// whole transcript. One rebuilt from the handover holds neither the
		// omitted results nor any recalled page: every omission is unread there.
		if note := meter.omissionLimitation(true); note != "" {
			result.FinalText += note
			if err := sink(Event{Type: EventAssistantDelta, Text: note}); err != nil {
				finish()
				return *result, err
			}
		}
	}
	state.closeOut("complete")
	if !wroteText {
		message := fmt.Sprintf("API tool loop reached %d rounds and final synthesis returned no text", p.config.MaxSteps)
		result.FinalText += message
		result.Outcome, result.Reason = OutcomeInconclusive, ReasonMaxTurns
		finish()
		_ = sink(Event{Type: EventInconclusive, Text: message, Reason: ReasonMaxTurns})
		return *result, nil
	}
	result.Outcome, result.Reason = OutcomeInconclusive, ReasonMaxTurns
	finish()
	if err := sink(Event{Type: EventInconclusive, Reason: ReasonMaxTurns}); err != nil {
		return *result, err
	}
	return *result, nil
}

// completeWithEmptyRecovery gives an OpenAI-compatible model one bounded
// chance to turn a reasoning-only completion into an observable result. Some
// local reasoning models return HTTP 200 with hundreds of private reasoning
// tokens but no assistant content or tool call. Exposing those private tokens
// as the answer would be both misleading and a protocol leak; immediately
// failing throws away an otherwise useful turn. A second empty response is a
// real provider/protocol failure and remains visible to the caller.
func (p *APIProvider) completeWithEmptyRecovery(ctx context.Context, request agent.Request) (*agent.Response, error) {
	response, err := p.completeChecked(ctx, request)
	if err != nil || responseHasVisibleOutcome(response) {
		return response, err
	}
	reasoningOnly := responseHasReasoning(response)
	firstObservation := nativeCompletionEvidence(response)
	// Preserve the observed native prompt prefix. Inserting a system message
	// invalidates its token observation and can refuse an otherwise admissible
	// recovery before HTTP. The existing admission charges this appended message
	// in full, within the same context and remaining token grant.
	// A Qwen model that spent the whole output allowance on private reasoning
	// does it again when asked the same way: on 11-12 Solar's T-US-CS29AB35C-001-013
	// spent 48,379 and then 50,221 reasoning tokens (47 min) and kept nothing.
	// The final-synthesis recovery already turns its thinking off for this
	// retry with /no_think; the empty-response recovery does the same.
	recovery := emptyCompletionRecoveryInstruction
	if reasoningOnly && strings.Contains(strings.ToLower(request.Model), "qwen") {
		recovery = "/no_think\n" + recovery
	}
	request.Messages = append(request.Messages, agent.NewTextMessage(agent.RoleUser, recovery))
	response, err = p.completeChecked(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("API provider empty-response recovery failed%s: %w", firstObservation, err)
	}
	if responseHasVisibleOutcome(response) {
		return response, nil
	}
	if reasoningOnly || responseHasReasoning(response) {
		return nil, fmt.Errorf("API provider returned reasoning but neither assistant text nor tool calls after one recovery attempt%s%s", firstObservation, nativeCompletionEvidence(response))
	}
	return nil, fmt.Errorf("API provider returned neither assistant text nor tool calls after one recovery attempt%s%s", firstObservation, nativeCompletionEvidence(response))
}

func nativeCompletionEvidence(response *agent.Response) string {
	if response == nil {
		return ""
	}
	observation, ok := response.Metadata["native_completion"].(agent.NativeCompletionObservation)
	if !ok {
		return ""
	}
	// Defense at the presentation boundary as well as the native parser.
	switch observation.Reason {
	case "stop", "length", "load", "unload":
	default:
		observation.Reason = "unknown"
	}
	raw, _ := json.Marshal(observation)
	return " [native_completion=" + string(raw) + "]"
}

// completeFinalSynthesis has a stricter success contract than an ordinary
// agentic completion: after the tool budget is spent, another tool call is not
// an observable answer. Some compatible local endpoints ignore tool_choice
// none and emit a call anyway. Preserve tool definitions on the first attempt
// for strict endpoints, then make one grammar-free direct-answer retry and
// accept only non-empty assistant text.
func (p *APIProvider) completeFinalSynthesis(ctx context.Context, request agent.Request) (*agent.Response, error) {
	response, err := p.completeChecked(ctx, request)
	if err != nil || responseHasFinalAnswerText(response) {
		return response, err
	}
	firstHadReasoning := responseHasReasoning(response)
	firstHadToolCall := response != nil && len(response.GetToolCalls()) > 0
	firstHadSerializedToolCall := responseContainsSerializedToolCall(response)
	request.System = appendSystemInstruction(request.System, finalSynthesisRecoveryInstruction)
	request.Tools = nil
	request.ToolChoice = "none"
	recovery := finalSynthesisRecoveryInstruction
	if strings.Contains(strings.ToLower(request.Model), "qwen") {
		recovery = "/no_think\n" + recovery
	}
	request.Messages = append(request.Messages, agent.NewTextMessage(agent.RoleUser, recovery))
	response, err = p.completeChecked(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("API provider final-synthesis recovery failed: %w", err)
	}
	if responseHasFinalAnswerText(response) {
		return response, nil
	}
	if firstHadToolCall || firstHadSerializedToolCall || (response != nil && len(response.GetToolCalls()) > 0) || responseContainsSerializedToolCall(response) {
		return nil, errors.New("API provider returned a tool call instead of assistant text after the tool budget was exhausted")
	}
	if firstHadReasoning || responseHasReasoning(response) {
		return nil, errors.New("API provider returned reasoning but no assistant text after final-synthesis recovery")
	}
	return nil, errors.New("API provider returned no assistant text after final-synthesis recovery")
}

// responseHasFinalAnswerText is stricter than ordinary visible text. Some
// OpenAI-compatible reasoning endpoints serialize their native tool grammar
// into a text block when tool calling is disabled. That block is visible, but
// it is still another attempted action rather than an answer to the owner.
func responseHasFinalAnswerText(response *agent.Response) bool {
	return responseHasVisibleText(response) && !responseContainsSerializedToolCall(response)
}

func responseContainsSerializedToolCall(response *agent.Response) bool {
	if response == nil {
		return false
	}
	for _, block := range response.Content {
		if block.Type != agent.ContentTypeText {
			continue
		}
		text := strings.ToLower(block.Text)
		if strings.Contains(text, "<tool_call") || strings.Contains(text, "</tool_call>") {
			return true
		}
	}
	return false
}

func responseHasVisibleText(response *agent.Response) bool {
	if response == nil {
		return false
	}
	for _, block := range response.Content {
		if block.Type == agent.ContentTypeText && strings.TrimSpace(block.Text) != "" {
			return true
		}
	}
	return false
}

func responseHasVisibleOutcome(response *agent.Response) bool {
	if response == nil {
		return false
	}
	if len(response.GetToolCalls()) > 0 {
		return true
	}
	return responseHasVisibleText(response)
}

func responseHasReasoning(response *agent.Response) bool {
	if response == nil || response.Metadata == nil {
		return false
	}
	reasoning, _ := response.Metadata["reasoning_content"].(string)
	thinking, _ := response.Metadata["thinking"].(string)
	return strings.TrimSpace(reasoning) != "" || strings.TrimSpace(thinking) != ""
}

func appendSystemInstruction(system, instruction string) string {
	system = strings.TrimSpace(system)
	if system == "" {
		return instruction
	}
	return system + "\n\n" + instruction
}

// replayMessages turns the console's history into the conversation this turn
// starts from, with the current prompt last.
//
// Both paths through Execute build it the same way. They did not have to —
// the streaming path had no history to add — and that is exactly how a
// conversation ends up remembering more with tools than without.
func replayMessages(request Request) []agent.Message {
	messages := make([]agent.Message, 0, len(request.History)+1)
	for _, message := range request.History {
		role := agent.RoleUser
		if message.Role == HistoryRoleAssistant {
			role = agent.RoleAssistant
		}
		messages = append(messages, agent.NewTextMessage(role, message.Content))
	}
	return append(messages, agent.NewTextMessage(agent.RoleUser, request.Prompt))
}

// completeChecked is the single transport gate for the tool loop: no request
// leaves for the endpoint unless its conversation is one a chat template can
// render. Qwen-style templates raise "No user query found in messages" (HTTP
// 500, with the prompt tokens already spent) when no plain user turn remains,
// so refuse locally with a precise reason instead.
func (p *APIProvider) completeChecked(ctx context.Context, request agent.Request) (*agent.Response, error) {
	if err := validateTransportConversation(request.Messages); err != nil {
		return nil, err
	}
	if p.trace == nil {
		return p.config.Adapter.Complete(ctx, request)
	}
	sent := time.Now()
	response, err := p.config.Adapter.Complete(ctx, request)
	p.trace.record(p.config.Adapter, sent, request, response, err)
	return response, err
}

// isPlainUserQuery reports whether a message is a user turn a chat template
// will treat as the owner's query: non-empty text that is not itself a wrapped
// tool response.
func isPlainUserQuery(message agent.Message) bool {
	if message.Role != agent.RoleUser {
		return false
	}
	text := strings.TrimSpace(message.GetText())
	if text == "" {
		return false
	}
	return !(strings.HasPrefix(text, "<tool_response>") && strings.HasSuffix(text, "</tool_response>"))
}

// validateTransportConversation fails closed on conversations that are
// malformed for a compatible endpoint: an empty conversation, a foreign role,
// no plain user query, an orphan tool result, or a tool call left unanswered
// before the next turn.
func validateTransportConversation(messages []agent.Message) error {
	if len(messages) == 0 {
		return errors.New("API conversation is empty")
	}
	hasQuery := false
	pending := map[string]bool{}
	for index, message := range messages {
		switch message.Role {
		case agent.RoleUser, agent.RoleAssistant:
			if len(pending) > 0 {
				return fmt.Errorf("API conversation message %d follows an unanswered tool call", index)
			}
			if message.Role == agent.RoleUser {
				hasQuery = hasQuery || isPlainUserQuery(message)
			}
			for _, call := range message.GetToolCalls() {
				if message.Role == agent.RoleAssistant {
					pending[call.ToolUseID] = true
				}
			}
		case agent.RoleTool:
			for _, block := range message.Content {
				if block.Type != agent.ContentTypeToolResult {
					continue
				}
				if !pending[block.ToolResultID] {
					return fmt.Errorf("API conversation message %d has a tool result with no matching call", index)
				}
				delete(pending, block.ToolResultID)
			}
		default:
			return fmt.Errorf("API conversation message %d has foreign role %q", index, message.Role)
		}
	}
	if len(pending) > 0 {
		return errors.New("API conversation ends with an unanswered tool call")
	}
	if !hasQuery {
		return errors.New("API conversation has no plain user query for the endpoint template")
	}
	return nil
}

var _ Provider = (*APIProvider)(nil)
