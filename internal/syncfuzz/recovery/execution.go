package recovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/objective"
)

const ExecutionSchemaVersion = "syncfuzz.recovery-execution.v1"

// StatePresence is reported independently by an adapter's logical-state and
// OS-state observers. It deliberately does not infer either layer from the
// other.
type StatePresence string

const (
	StatePresencePresent StatePresence = "present"
	StatePresenceAbsent  StatePresence = "absent"
	StatePresenceUnknown StatePresence = "unknown"
)

func (p StatePresence) Valid() bool {
	return p == StatePresencePresent || p == StatePresenceAbsent || p == StatePresenceUnknown
}

// StateOrigin tells whether an observed OS state survived the recovery
// boundary or was formed again by recovery execution.
type StateOrigin string

const (
	StateOriginResidual      StateOrigin = "residual"
	StateOriginReconstructed StateOrigin = "reconstructed"
	StateOriginNone          StateOrigin = "none"
	StateOriginUnknown       StateOrigin = "unknown"
)

func (o StateOrigin) Valid() bool {
	return o == StateOriginResidual || o == StateOriginReconstructed || o == StateOriginNone || o == StateOriginUnknown
}

type EffectMultiplicity string

const (
	EffectMultiplicitySingle    EffectMultiplicity = "single"
	EffectMultiplicityDuplicate EffectMultiplicity = "duplicate"
	EffectMultiplicityUnknown   EffectMultiplicity = "unknown"
)

func (m EffectMultiplicity) Valid() bool {
	return m == EffectMultiplicitySingle || m == EffectMultiplicityDuplicate || m == EffectMultiplicityUnknown
}

// RecoveryObservation is the fixed passive observation for one member of a
// fork pair. An adapter must bind it to the exact query and recorded plan that
// SyncFuzz supplied; an observation cannot be silently reused for a different
// checkpoint, plan, or passive observation.
type RecoveryObservation struct {
	SchemaVersion        string             `json:"schema_version"`
	QueryID              string             `json:"query_id"`
	SeedID               string             `json:"seed_id"`
	Boundary             Boundary           `json:"boundary"`
	CheckpointID         string             `json:"checkpoint_id"`
	RecordedPlanID       string             `json:"recorded_plan_id"`
	PassiveObservationID string             `json:"passive_observation_id"`
	RuntimeInstanceID    string             `json:"runtime_instance_id"`
	AgentState           StatePresence      `json:"agent_state"`
	OSState              StatePresence      `json:"os_state"`
	OSStateOrigin        StateOrigin        `json:"os_state_origin"`
	EffectMultiplicity   EffectMultiplicity `json:"effect_multiplicity"`
	Evidence             []string           `json:"evidence"`
}

func (o RecoveryObservation) ValidateFor(query RecoveryQuery, plan RecordedPlan) error {
	if o.SchemaVersion != "" && o.SchemaVersion != ExecutionSchemaVersion {
		return fmt.Errorf("unsupported recovery observation schema %q", o.SchemaVersion)
	}
	if o.QueryID != query.QueryID || o.SeedID != query.SeedID || o.Boundary != query.Boundary || o.CheckpointID != query.CheckpointID || o.RecordedPlanID != query.RecordedPlanID || o.PassiveObservationID != query.PassiveObservationID {
		return fmt.Errorf("recovery observation does not bind to query %q", query.QueryID)
	}
	if o.RecordedPlanID != plan.RecordedPlanID || strings.TrimSpace(o.RuntimeInstanceID) == "" || !o.AgentState.Valid() || !o.OSState.Valid() || !o.OSStateOrigin.Valid() || !o.EffectMultiplicity.Valid() {
		return fmt.Errorf("recovery observation %q has invalid state evidence", o.QueryID)
	}
	if len(o.Evidence) == 0 {
		return fmt.Errorf("recovery observation %q requires deterministic evidence", o.QueryID)
	}
	return nil
}

// ForkExecutionRequest contains exactly the immutable plan and one checkpoint
// coordinate. An adapter is responsible for starting a fresh isolated runtime
// for every invocation; it must not implement fork by reusing controller
// observation checkpoints from the original profiling container.
type ForkExecutionRequest struct {
	Query RecoveryQuery `json:"query"`
	Plan  RecordedPlan  `json:"plan"`
}

// ForkExecutor is implemented only by adapters that expose an actual durable
// Agent checkpoint/fork operation. The command adapter intentionally does not
// implement it: a SyncFuzz controller observation boundary is not a durable
// Agent checkpoint.
type ForkExecutor interface {
	ExecuteFork(context.Context, ForkExecutionRequest) (RecoveryObservation, error)
}

type ForkExecutorFunc func(context.Context, ForkExecutionRequest) (RecoveryObservation, error)

func (f ForkExecutorFunc) ExecuteFork(ctx context.Context, request ForkExecutionRequest) (RecoveryObservation, error) {
	return f(ctx, request)
}

// ForkExecutorRegistry binds a recorded adapter ID to its durable-checkpoint
// implementation. It starts empty by design: no current target adapter may
// claim fork support merely because SyncFuzz observed controller checkpoints.
type ForkExecutorRegistry struct {
	executors map[string]ForkExecutor
}

func NewForkExecutorRegistry() *ForkExecutorRegistry {
	return &ForkExecutorRegistry{executors: make(map[string]ForkExecutor)}
}

func (r *ForkExecutorRegistry) Register(adapterID string, executor ForkExecutor) error {
	if r == nil || strings.TrimSpace(adapterID) == "" || executor == nil {
		return fmt.Errorf("fork executor registration requires registry, adapter ID, and executor")
	}
	if _, exists := r.executors[adapterID]; exists {
		return fmt.Errorf("fork executor already registered for adapter %q", adapterID)
	}
	r.executors[adapterID] = executor
	return nil
}

func (r *ForkExecutorRegistry) Execute(ctx context.Context, seed objective.StateSeed, pair RecoveryPair, plan RecordedPlan) (*ForkPairExecution, error) {
	if r == nil {
		return nil, fmt.Errorf("fork executor registry is required")
	}
	executor, ok := r.executors[plan.AdapterID]
	if !ok {
		return nil, fmt.Errorf("target adapter %q does not expose a durable checkpoint fork executor", plan.AdapterID)
	}
	return ExecuteForkPair(ctx, seed, pair, plan, executor)
}

// PairClassification keeps both point classifications and the deterministic
// comparison outcome. A non-consistent after result takes precedence because
// it is the branch recovered at the state-forming frontier; if it is clean, a
// non-consistent before result still remains useful boundary-localization
// evidence.
type PairClassification struct {
	BeforeOutcome string `json:"before_outcome"`
	AfterOutcome  string `json:"after_outcome"`
	Outcome       string `json:"outcome"`
}

// ForkPairExecution is the durable artifact produced by the V2.3 executor.
// It records no generated scenario, mutation focus, prompt variant, or query
// genealogy.
type ForkPairExecution struct {
	SchemaVersion    string              `json:"schema_version"`
	ComparisonPairID string              `json:"comparison_pair_id"`
	SeedID           string              `json:"seed_id"`
	FrontierID       string              `json:"frontier_id"`
	RecordedPlanID   string              `json:"recorded_plan_id"`
	Before           RecoveryObservation `json:"before"`
	After            RecoveryObservation `json:"after"`
	Classification   PairClassification  `json:"classification"`
}

// ExecuteForkPair runs before and after as separate invocations of the same
// durable-checkpoint adapter. The immutable recorded plan and passive
// observation come from the recovery pair; checkpoint is the only different
// field in the two requests.
func ExecuteForkPair(ctx context.Context, seed objective.StateSeed, pair RecoveryPair, plan RecordedPlan, executor ForkExecutor) (*ForkPairExecution, error) {
	if executor == nil {
		return nil, fmt.Errorf("fork executor is required")
	}
	if err := seed.Validate(); err != nil {
		return nil, err
	}
	if err := pair.ValidateFor(seed); err != nil {
		return nil, err
	}
	if err := plan.ValidateFor(seed); err != nil {
		return nil, err
	}
	if err := validatePairAgainstPlan(pair, plan); err != nil {
		return nil, err
	}
	before, err := executeForkQuery(ctx, pair.Before, plan, executor)
	if err != nil {
		return nil, fmt.Errorf("execute before query: %w", err)
	}
	after, err := executeForkQuery(ctx, pair.After, plan, executor)
	if err != nil {
		return nil, fmt.Errorf("execute after query: %w", err)
	}
	if before.RuntimeInstanceID == after.RuntimeInstanceID {
		return nil, fmt.Errorf("before and after recovery queries reused runtime instance %q", before.RuntimeInstanceID)
	}
	classification := ClassifyForkPair(before, after)
	return &ForkPairExecution{
		SchemaVersion:    ExecutionSchemaVersion,
		ComparisonPairID: pair.ComparisonPairID,
		SeedID:           pair.SeedID,
		FrontierID:       pair.FrontierID,
		RecordedPlanID:   pair.RecordedPlanID,
		Before:           before,
		After:            after,
		Classification:   classification,
	}, nil
}

func validatePairAgainstPlan(pair RecoveryPair, plan RecordedPlan) error {
	if pair.SchemaVersion != "" && pair.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported recovery pair schema %q", pair.SchemaVersion)
	}
	if pair.Boundary != BoundaryFork || pair.Before.Boundary != BoundaryFork || pair.After.Boundary != BoundaryFork {
		return fmt.Errorf("V2.3 executor requires a fork-only recovery pair")
	}
	if strings.TrimSpace(pair.ComparisonPairID) == "" || strings.TrimSpace(pair.SeedID) == "" || strings.TrimSpace(pair.FrontierID) == "" || pair.Before.CheckpointID == pair.After.CheckpointID {
		return fmt.Errorf("recovery pair lacks a distinct before/after frontier")
	}
	if pair.RecordedPlanID != plan.RecordedPlanID || pair.Before.RecordedPlanID != plan.RecordedPlanID || pair.After.RecordedPlanID != plan.RecordedPlanID {
		return fmt.Errorf("recovery pair does not preserve the recorded plan")
	}
	if pair.PassiveObservationID == "" || pair.Before.PassiveObservationID != pair.PassiveObservationID || pair.After.PassiveObservationID != pair.PassiveObservationID || plan.PassiveObservationID != pair.PassiveObservationID {
		return fmt.Errorf("recovery pair does not preserve one passive observation")
	}
	if pair.Before.SeedID != pair.SeedID || pair.After.SeedID != pair.SeedID || pair.Before.QueryID == pair.After.QueryID {
		return fmt.Errorf("recovery pair does not preserve one seed with distinct queries")
	}
	return nil
}

func executeForkQuery(ctx context.Context, query RecoveryQuery, plan RecordedPlan, executor ForkExecutor) (RecoveryObservation, error) {
	observation, err := executor.ExecuteFork(ctx, ForkExecutionRequest{Query: query, Plan: plan})
	if err != nil {
		return RecoveryObservation{}, err
	}
	if err := observation.ValidateFor(query, plan); err != nil {
		return RecoveryObservation{}, err
	}
	return observation, nil
}

// ClassifyForkPair is a deterministic evidence classifier. It intentionally
// returns inconclusive instead of guessing when either observer cannot state
// a layer, origin, or effect multiplicity.
func ClassifyForkPair(before RecoveryObservation, after RecoveryObservation) PairClassification {
	beforeOutcome := classifyObservation(before)
	afterOutcome := classifyObservation(after)
	return PairClassification{
		BeforeOutcome: beforeOutcome,
		AfterOutcome:  afterOutcome,
		Outcome:       selectComparisonOutcome(beforeOutcome, afterOutcome),
	}
}

func classifyObservation(observation RecoveryObservation) string {
	if !observation.AgentState.Valid() || !observation.OSState.Valid() || !observation.OSStateOrigin.Valid() || !observation.EffectMultiplicity.Valid() || observation.AgentState == StatePresenceUnknown || observation.OSState == StatePresenceUnknown || observation.OSStateOrigin == StateOriginUnknown || observation.EffectMultiplicity == EffectMultiplicityUnknown || len(observation.Evidence) == 0 {
		return "inconclusive"
	}
	if observation.EffectMultiplicity == EffectMultiplicityDuplicate {
		return "duplicate"
	}
	if observation.OSState == StatePresencePresent && observation.OSStateOrigin == StateOriginReconstructed {
		return "reconstruction"
	}
	if observation.AgentState == StatePresenceAbsent && observation.OSState == StatePresencePresent && observation.OSStateOrigin == StateOriginResidual {
		return "residual"
	}
	if observation.AgentState == StatePresencePresent && observation.OSState == StatePresenceAbsent {
		return "missing"
	}
	if observation.AgentState == observation.OSState && (observation.OSState == StatePresenceAbsent || observation.OSStateOrigin == StateOriginResidual || observation.OSStateOrigin == StateOriginNone) {
		return "consistent"
	}
	return "inconclusive"
}

func selectComparisonOutcome(before string, after string) string {
	priority := map[string]int{
		"consistent":     0,
		"inconclusive":   1,
		"missing":        2,
		"residual":       3,
		"reconstruction": 4,
		"duplicate":      5,
	}
	if priority[after] >= priority[before] {
		return after
	}
	return before
}

func WriteForkPairExecution(path string, execution ForkPairExecution) error {
	if execution.SchemaVersion != ExecutionSchemaVersion {
		return fmt.Errorf("unsupported fork pair execution schema %q", execution.SchemaVersion)
	}
	return writeRecoveryJSON(path, execution)
}
