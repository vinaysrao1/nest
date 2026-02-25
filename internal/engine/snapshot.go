package engine

import (
	"fmt"
	"sort"
	"time"

	"github.com/vinaysrao1/nest/internal/domain"
)

// Snapshot is an immutable, point-in-time view of all compiled rules and action
// definitions for one org. It is safe for concurrent read access by multiple goroutines.
type Snapshot struct {
	ID       string
	OrgID    string
	Rules    []*CompiledRule
	ByEvent  map[string][]*CompiledRule
	Actions  map[string]domain.Action // action name -> action definition
	LoadedAt time.Time
}

// NewSnapshot constructs a Snapshot from a slice of CompiledRules and an action map.
//
// Rules are indexed by each of their event types. Wildcard rules (event_types=["*"])
// are stored under the "*" key. Within each event-type bucket, rules are sorted by
// priority descending so that the highest-priority rule appears first.
//
// actions is keyed by action name and may be nil (treated as empty).
func NewSnapshot(orgID string, rules []*CompiledRule, actions map[string]domain.Action) *Snapshot {
	id := fmt.Sprintf("%s-%d", orgID, time.Now().UnixNano())
	byEvent := buildEventIndex(rules)
	if actions == nil {
		actions = make(map[string]domain.Action)
	}

	return &Snapshot{
		ID:       id,
		OrgID:    orgID,
		Rules:    rules,
		ByEvent:  byEvent,
		Actions:  actions,
		LoadedAt: time.Now(),
	}
}

// buildEventIndex groups rules by event type and sorts each group by priority descending.
func buildEventIndex(rules []*CompiledRule) map[string][]*CompiledRule {
	byEvent := make(map[string][]*CompiledRule)
	for _, rule := range rules {
		for _, et := range rule.EventTypes {
			byEvent[et] = append(byEvent[et], rule)
		}
	}
	for _, ruleList := range byEvent {
		sortByPriorityDesc(ruleList)
	}
	return byEvent
}

// RulesForEvent returns all rules that apply to the given event type,
// merging event-specific rules with wildcard ("*") rules and sorting
// the combined set by priority descending.
//
// Returns an empty (non-nil) slice if no rules match.
func (s *Snapshot) RulesForEvent(eventType string) []*CompiledRule {
	specific := s.ByEvent[eventType]
	wildcard := s.ByEvent["*"]

	if len(specific) == 0 && len(wildcard) == 0 {
		return []*CompiledRule{}
	}

	merged := make([]*CompiledRule, 0, len(specific)+len(wildcard))
	merged = append(merged, specific...)
	merged = append(merged, wildcard...)
	sortByPriorityDesc(merged)
	return merged
}

// sortByPriorityDesc sorts a slice of CompiledRule pointers by priority descending (in-place).
func sortByPriorityDesc(rules []*CompiledRule) {
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})
}
