package engine

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.starlark.net/starlark"

	"github.com/vinaysrao1/nest/internal/domain"
)

// run is the goroutine entry point. Reads events from pool.eventCh until closed.
func (w *Worker) run() {
	for req := range w.pool.eventCh {
		result := w.processEvent(req.Ctx, req.Event)
		req.Result <- result
	}
	w.pool.wg.Done()
}

// processEvent evaluates all matching rules for an event.
func (w *Worker) processEvent(ctx context.Context, event domain.Event) EvalResult {
	start := time.Now()

	// Periodically clean stale counter entries to bound memory.
	w.eventCount++
	if w.eventCount%counterCleanupInterval == 0 {
		w.cleanStaleCounters()
	}

	// Per-event timeout (5 seconds).
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Clear per-event state.
	w.memo = make(map[string]starlark.Value)
	w.currentCtx = &evalContext{
		ctx:         ctx,
		event:       event,
		signalCache: make(map[string]domain.SignalOutput),
	}
	defer func() { w.currentCtx = nil }()

	// Load snapshot for org.
	ptrVal, ok := w.pool.snapshots.Load(event.OrgID)
	if !ok {
		return EvalResult{
			Verdict:       domain.Verdict{Type: domain.VerdictApprove},
			CorrelationID: event.ID,
			LatencyUs:     time.Since(start).Microseconds(),
		}
	}
	snap := ptrVal.(*atomic.Pointer[Snapshot]).Load()
	if snap == nil {
		return EvalResult{
			Verdict:       domain.Verdict{Type: domain.VerdictApprove},
			CorrelationID: event.ID,
			LatencyUs:     time.Since(start).Microseconds(),
		}
	}

	// Invalidate eval cache if snapshot changed.
	if snap.ID != w.lastSnap {
		w.evalCache = make(map[string]starlark.Callable)
		w.lastSnap = snap.ID
	}

	// Get matching rules.
	rules := snap.RulesForEvent(event.EventType)
	if len(rules) == 0 {
		return EvalResult{
			Verdict:       domain.Verdict{Type: domain.VerdictApprove},
			CorrelationID: event.ID,
			LatencyUs:     time.Since(start).Microseconds(),
		}
	}

	// Convert event to Starlark dict.
	eventDict, err := eventToStarlarkDict(event)
	if err != nil {
		return EvalResult{
			Error:         fmt.Errorf("convert event to starlark: %w", err),
			CorrelationID: event.ID,
			LatencyUs:     time.Since(start).Microseconds(),
		}
	}

	// Evaluate each rule.
	results := make([]ruleResult, 0, len(rules))
	for _, rule := range rules {
		if ctx.Err() != nil {
			break // per-event timeout exceeded
		}
		result := w.evaluateRule(ctx, rule, eventDict)
		result.priority = rule.Priority
		results = append(results, result)
	}

	// Resolve final verdict.
	verdict := resolveVerdict(results)

	// Build triggered rules list.
	triggered := make([]domain.TriggeredRule, 0, len(results))
	for _, r := range results {
		triggered = append(triggered, domain.TriggeredRule{
			RuleID:    r.ruleID,
			Version:   r.version,
			Verdict:   r.verdict.Type,
			Reason:    r.verdict.Reason,
			LatencyUs: r.latencyUs,
		})
	}

	// Collect unique action names from all triggered rule verdicts and resolve
	// them to ActionRequest objects using the action definitions in the snapshot.
	actionRequests := resolveActionRequests(results, snap, event)

	return EvalResult{
		Verdict:        verdict,
		TriggeredRules: triggered,
		ActionRequests: actionRequests,
		Logs:           w.currentCtx.logs,
		LatencyUs:      time.Since(start).Microseconds(),
		CorrelationID:  event.ID,
	}
}

// evaluateRule evaluates a single compiled rule. Uses defer/recover for panic safety.
// A fresh starlark.Thread is created per evaluation to avoid cross-rule state pollution.
//
// Returns a ruleResult with err set if evaluation fails or panics.
func (w *Worker) evaluateRule(ctx context.Context, rule *CompiledRule, eventDict starlark.Value) (result ruleResult) {
	start := time.Now()

	// Per-rule timeout (1 second) for context-aware UDFs (signal, enqueue).
	ruleCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Panic recovery.
	defer func() {
		if r := recover(); r != nil {
			result = ruleResult{
				ruleID:    rule.ID,
				latencyUs: time.Since(start).Microseconds(),
				err:       fmt.Errorf("panic in rule %s: %v", rule.ID, r),
			}
		}
	}()

	// Create a fresh thread for each rule evaluation to avoid accumulated Steps
	// or cancelReason from previous evaluations polluting this call.
	// The fresh thread shares the worker's predeclared UDFs (bound to w).
	evalThread := &starlark.Thread{
		Name: fmt.Sprintf("worker-%d-rule-%s", w.id, rule.ID),
	}
	// Bound execution to 10M steps to prevent infinite loops without blocking the worker.
	evalThread.SetMaxExecutionSteps(10_000_000)

	// Get or populate the evaluate callable cache.
	// The callable is a Starlark function independent of any thread.
	evalFn, cached := w.evalCache[rule.ID]
	if !cached {
		globals, err := rule.Program.Init(evalThread, w.predeclared)
		if err != nil {
			return ruleResult{
				ruleID:    rule.ID,
				latencyUs: time.Since(start).Microseconds(),
				err:       fmt.Errorf("init rule %s: %w", rule.ID, err),
			}
		}
		fn, ok := globals["evaluate"]
		if !ok {
			return ruleResult{
				ruleID:    rule.ID,
				latencyUs: time.Since(start).Microseconds(),
				err:       fmt.Errorf("rule %s: missing evaluate function", rule.ID),
			}
		}
		callable, ok := fn.(starlark.Callable)
		if !ok {
			return ruleResult{
				ruleID:    rule.ID,
				latencyUs: time.Since(start).Microseconds(),
				err:       fmt.Errorf("rule %s: evaluate is not callable", rule.ID),
			}
		}
		evalFn = callable
		w.evalCache[rule.ID] = evalFn
		// Reset the thread after Init so the step count does not affect the evaluate call below.
		evalThread = &starlark.Thread{
			Name: fmt.Sprintf("worker-%d-rule-%s", w.id, rule.ID),
		}
		evalThread.SetMaxExecutionSteps(10_000_000)
	}

	// Swap in per-rule context so UDFs can check the tighter deadline.
	origCtx := w.currentCtx.ctx
	w.currentCtx.ctx = ruleCtx
	defer func() { w.currentCtx.ctx = origCtx }()

	// Call evaluate(event).
	retVal, err := starlark.Call(evalThread, evalFn, starlark.Tuple{eventDict}, nil)
	if err != nil {
		return ruleResult{
			ruleID:    rule.ID,
			latencyUs: time.Since(start).Microseconds(),
			err:       fmt.Errorf("rule %s evaluate: %w", rule.ID, err),
		}
	}

	// Parse verdict from return value.
	verdict, err := parseVerdictResult(retVal, rule.ID)
	if err != nil {
		return ruleResult{
			ruleID:    rule.ID,
			latencyUs: time.Since(start).Microseconds(),
			err:       err,
		}
	}

	return ruleResult{
		ruleID:    rule.ID,
		version:   rule.Version,
		verdict:   verdict,
		latencyUs: time.Since(start).Microseconds(),
	}
}

// parseVerdictResult extracts a domain.Verdict from a Starlark return value.
// The value should be a struct with "type", "reason", "actions" fields
// as returned by the verdict() UDF. Returns an approve verdict for None.
func parseVerdictResult(val starlark.Value, ruleID string) (domain.Verdict, error) {
	if val == starlark.None || val == nil {
		return domain.Verdict{Type: domain.VerdictApprove, RuleID: ruleID}, nil
	}

	hasAttrs, ok := val.(starlark.HasAttrs)
	if !ok {
		return domain.Verdict{}, fmt.Errorf("rule %s: evaluate must return verdict(), got %s", ruleID, val.Type())
	}

	typeVal, err := hasAttrs.Attr("type")
	if err != nil || typeVal == nil {
		return domain.Verdict{}, fmt.Errorf("rule %s: verdict missing 'type' attribute", ruleID)
	}
	verdictType, ok := starlark.AsString(typeVal)
	if !ok {
		return domain.Verdict{}, fmt.Errorf("rule %s: verdict type must be a string", ruleID)
	}

	var reason string
	if reasonVal, attrErr := hasAttrs.Attr("reason"); attrErr == nil && reasonVal != nil {
		reason, _ = starlark.AsString(reasonVal)
	}

	var actions []string
	if actionsVal, attrErr := hasAttrs.Attr("actions"); attrErr == nil && actionsVal != nil {
		if list, ok := actionsVal.(*starlark.List); ok {
			actions = make([]string, 0, list.Len())
			for i := 0; i < list.Len(); i++ {
				s, _ := starlark.AsString(list.Index(i))
				actions = append(actions, s)
			}
		}
	}

	return domain.Verdict{
		Type:    domain.VerdictType(verdictType),
		Reason:  reason,
		RuleID:  ruleID,
		Actions: actions,
	}, nil
}

// resolveActionRequests collects all unique action names referenced in the verdict
// actions fields of every rule result, then resolves each name to a domain.ActionRequest
// using the action definitions stored in snap.Actions. Action names that are not found
// in the snapshot are silently skipped (the action may have been deleted since the last
// snapshot rebuild).
//
// Pre-conditions: snap must not be nil.
// Post-conditions: returns nil if no actions are referenced or none can be resolved.
func resolveActionRequests(results []ruleResult, snap *Snapshot, event domain.Event) []domain.ActionRequest {
	// Collect unique action names across all rule verdicts.
	seen := make(map[string]struct{})
	for _, r := range results {
		if r.err != nil {
			continue
		}
		for _, name := range r.verdict.Actions {
			seen[name] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}

	// Resolve names to ActionRequest objects.
	requests := make([]domain.ActionRequest, 0, len(seen))
	for name := range seen {
		action, ok := snap.Actions[name]
		if !ok {
			continue
		}
		requests = append(requests, domain.ActionRequest{
			Action:        action,
			Payload:       event.Payload,
			CorrelationID: event.ID,
		})
	}
	if len(requests) == 0 {
		return nil
	}
	return requests
}

// eventToStarlarkDict converts a domain.Event to a starlark.Dict.
func eventToStarlarkDict(event domain.Event) (*starlark.Dict, error) {
	d := starlark.NewDict(6)
	if err := d.SetKey(starlark.String("event_id"), starlark.String(event.ID)); err != nil {
		return nil, err
	}
	if err := d.SetKey(starlark.String("event_type"), starlark.String(event.EventType)); err != nil {
		return nil, err
	}
	if err := d.SetKey(starlark.String("item_type"), starlark.String(event.ItemType)); err != nil {
		return nil, err
	}
	if err := d.SetKey(starlark.String("org_id"), starlark.String(event.OrgID)); err != nil {
		return nil, err
	}
	if err := d.SetKey(starlark.String("timestamp"), starlark.MakeInt64(event.Timestamp.Unix())); err != nil {
		return nil, err
	}

	payloadDict := mapToStarlarkDict(event.Payload)
	if err := d.SetKey(starlark.String("payload"), payloadDict); err != nil {
		return nil, err
	}

	return d, nil
}
