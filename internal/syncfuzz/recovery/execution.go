package recovery

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
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

// PassiveProbeMetrics holds the post-recovery probe work used to classify one
// observation. It is optional because non-LangGraph adapters do not yet
// expose a comparable passive observer. A pruned sample establishes an exact
// recorded holder identity, but cannot establish holder multiplicity.
type PassiveProbeMetrics struct {
	Mode             LangGraphPassiveProbeMode `json:"mode"`
	DurationNS       uint64                    `json:"duration_ns"`
	ScannedProcesses int                       `json:"scanned_processes"`
	ScannedFDs       int                       `json:"scanned_fds"`
}

func (m PassiveProbeMetrics) Valid() bool {
	return m.Mode.Valid() && m.ScannedProcesses >= 0 && m.ScannedFDs >= 0
}

// ContinuationEvidence records the deterministic evidence collected on both
// sides of executing one frozen continuation query. The evidence vocabulary is
// deliberately adapter-neutral: concrete adapters may record checkpoint
// restore, tool lifecycle, or observation artifact references without making
// them part of recovery's state classifier.
type ContinuationEvidence struct {
	ContinuationQueryID string   `json:"continuation_query_id"`
	PreEvidence         []string `json:"pre_evidence"`
	PostEvidence        []string `json:"post_evidence"`
}

const EnvironmentUseEvidenceSchemaVersion = "syncfuzz.recovery-environment-use.v1"

// EnvironmentUseEvidence is an adapter-produced, payload-free record of a
// typed recovery-time dependency. It is kept separate from Evidence strings so
// higher layers can construct a hazard report without re-parsing prose.
type EnvironmentUseEvidence struct {
	SchemaVersion          string   `json:"schema_version"`
	Family                 string   `json:"family"`
	ProgramID              string   `json:"program_id"`
	LogicalName            string   `json:"logical_name"`
	ResolvedEndpointPath   string   `json:"resolved_endpoint_path"`
	ConnectEventIDs        []string `json:"connect_event_ids"`
	RequestSHA256          string   `json:"request_sha256"`
	CompletedExchange      bool     `json:"completed_exchange"`
	ListenerRole           string   `json:"listener_role"`
	ListenerPID            int      `json:"listener_pid"`
	ListenerFD             int      `json:"listener_fd"`
	ListenerSocketID       string   `json:"listener_socket_id"`
	ListenerEndpointDevice uint64   `json:"listener_endpoint_device"`
	ListenerEndpointInode  uint64   `json:"listener_endpoint_inode"`
	ListenerSocketDevice   uint64   `json:"listener_socket_device"`
	ListenerSocketInode    uint64   `json:"listener_socket_inode"`
}

func (e EnvironmentUseEvidence) Validate() error {
	if e.SchemaVersion != EnvironmentUseEvidenceSchemaVersion || e.Family != "unix-socket" || strings.TrimSpace(e.ProgramID) == "" || strings.TrimSpace(e.LogicalName) == "" || !filepath.IsAbs(e.ResolvedEndpointPath) || len(e.ConnectEventIDs) == 0 || len(e.RequestSHA256) != 64 || !e.CompletedExchange || strings.TrimSpace(e.ListenerRole) == "" || e.ListenerPID <= 0 || e.ListenerFD < 0 || !strings.HasPrefix(e.ListenerSocketID, "socket:") || e.ListenerEndpointInode == 0 || e.ListenerSocketInode == 0 {
		return fmt.Errorf("recovery environment use evidence is incomplete")
	}
	seen := make(map[string]struct{}, len(e.ConnectEventIDs))
	for _, eventID := range e.ConnectEventIDs {
		if strings.TrimSpace(eventID) == "" {
			return fmt.Errorf("recovery environment use evidence has an empty connect event ID")
		}
		if _, found := seen[eventID]; found {
			return fmt.Errorf("recovery environment use evidence repeats connect event %q", eventID)
		}
		seen[eventID] = struct{}{}
	}
	for _, character := range e.RequestSHA256 {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return fmt.Errorf("recovery environment use evidence has an invalid request digest")
		}
	}
	if e.ListenerSocketID != "socket:"+strconv.FormatUint(e.ListenerSocketInode, 10) {
		return fmt.Errorf("recovery environment use evidence listener socket identity is inconsistent")
	}
	return nil
}

func (e ContinuationEvidence) ValidateFor(query RecoveryQuery) error {
	if query.ContinuationQueryID == "" {
		return fmt.Errorf("continuation evidence is not allowed for recovery query %q without a continuation", query.QueryID)
	}
	if e.ContinuationQueryID != query.ContinuationQueryID || len(e.PreEvidence) == 0 || len(e.PostEvidence) == 0 {
		return fmt.Errorf("continuation evidence does not bind complete pre/post evidence to recovery query %q", query.QueryID)
	}
	for _, phase := range [][]string{e.PreEvidence, e.PostEvidence} {
		for _, evidence := range phase {
			if strings.TrimSpace(evidence) == "" {
				return fmt.Errorf("continuation evidence for recovery query %q has an empty entry", query.QueryID)
			}
		}
	}
	return nil
}

// RecoveryObservation is the fixed passive observation for one member of a
// fork pair. An adapter must bind it to the exact query and recorded plan that
// SyncFuzz supplied; an observation cannot be silently reused for a different
// checkpoint, plan, or passive observation.
type RecoveryObservation struct {
	SchemaVersion          string                  `json:"schema_version"`
	QueryID                string                  `json:"query_id"`
	SeedID                 string                  `json:"seed_id"`
	Boundary               Boundary                `json:"boundary"`
	CheckpointID           string                  `json:"checkpoint_id"`
	RecordedPlanID         string                  `json:"recorded_plan_id"`
	PassiveObservationID   string                  `json:"passive_observation_id"`
	MaterializationHeadID  string                  `json:"materialization_head_id,omitempty"`
	RetentionPolicy        RetentionPolicy         `json:"retention_policy,omitempty"`
	RuntimeInstanceID      string                  `json:"runtime_instance_id"`
	AgentState             StatePresence           `json:"agent_state"`
	OSState                StatePresence           `json:"os_state"`
	OSStateOrigin          StateOrigin             `json:"os_state_origin"`
	EffectMultiplicity     EffectMultiplicity      `json:"effect_multiplicity"`
	PassiveProbe           *PassiveProbeMetrics    `json:"passive_probe,omitempty"`
	ContinuationEvidence   *ContinuationEvidence   `json:"continuation_evidence,omitempty"`
	EnvironmentUseEvidence *EnvironmentUseEvidence `json:"environment_use_evidence,omitempty"`
	Evidence               []string                `json:"evidence"`
}

func (o RecoveryObservation) ValidateFor(query RecoveryQuery, plan RecordedPlan) error {
	if o.SchemaVersion != "" && o.SchemaVersion != ExecutionSchemaVersion {
		return fmt.Errorf("unsupported recovery observation schema %q", o.SchemaVersion)
	}
	if o.QueryID != query.QueryID || o.SeedID != query.SeedID || o.Boundary != query.Boundary || o.CheckpointID != query.CheckpointID || o.RecordedPlanID != query.RecordedPlanID || o.PassiveObservationID != query.PassiveObservationID || o.MaterializationHeadID != query.MaterializationHeadID || o.RetentionPolicy != query.RetentionPolicy {
		return fmt.Errorf("recovery observation does not bind to query %q", query.QueryID)
	}
	if o.RecordedPlanID != plan.RecordedPlanID || strings.TrimSpace(o.RuntimeInstanceID) == "" || !o.AgentState.Valid() || !o.OSState.Valid() || !o.OSStateOrigin.Valid() || !o.EffectMultiplicity.Valid() {
		return fmt.Errorf("recovery observation %q has invalid state evidence", o.QueryID)
	}
	if o.PassiveProbe != nil && !o.PassiveProbe.Valid() {
		return fmt.Errorf("recovery observation %q has invalid passive probe metrics", o.QueryID)
	}
	if query.ContinuationQueryID == "" {
		if o.ContinuationEvidence != nil {
			return fmt.Errorf("recovery observation %q reports continuation evidence for a query without continuation", o.QueryID)
		}
	} else {
		if o.ContinuationEvidence == nil {
			return fmt.Errorf("recovery observation %q requires pre/post continuation evidence", o.QueryID)
		}
		if err := o.ContinuationEvidence.ValidateFor(query); err != nil {
			return err
		}
	}
	if o.EnvironmentUseEvidence != nil {
		if query.ContinuationQueryID == "" {
			return fmt.Errorf("recovery observation %q reports environment use without a continuation", o.QueryID)
		}
		if err := o.EnvironmentUseEvidence.Validate(); err != nil {
			return fmt.Errorf("recovery observation %q environment use: %w", o.QueryID, err)
		}
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
	Query             RecoveryQuery      `json:"query"`
	Plan              RecordedPlan       `json:"plan"`
	ContinuationQuery *ContinuationQuery `json:"continuation_query,omitempty"`
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

// ExecuteRecoverySet runs the complete before/after/head control set through
// one adapter. The adapter must materialize the same recorded head for each
// query and use a fresh runtime instance for every observation.
func (r *ForkExecutorRegistry) ExecuteRecoverySet(ctx context.Context, seed objective.StateSeed, set HistoricalRecoverySet, plan RecordedPlan) (*ForkRecoverySetExecution, error) {
	if r == nil {
		return nil, fmt.Errorf("fork executor registry is required")
	}
	executor, ok := r.executors[plan.AdapterID]
	if !ok {
		return nil, fmt.Errorf("target adapter %q does not expose a durable checkpoint fork executor", plan.AdapterID)
	}
	return ExecuteForkRecoverySet(ctx, seed, set, plan, executor)
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
	SchemaVersion     string              `json:"schema_version"`
	ComparisonPairID  string              `json:"comparison_pair_id"`
	SeedID            string              `json:"seed_id"`
	FrontierID        string              `json:"frontier_id"`
	RecordedPlanID    string              `json:"recorded_plan_id"`
	ContinuationQuery *ContinuationQuery  `json:"continuation_query,omitempty"`
	Before            RecoveryObservation `json:"before"`
	After             RecoveryObservation `json:"after"`
	Classification    PairClassification  `json:"classification"`
}

// RecoverySetClassification retains the head no-rollback control alongside
// the historical before/after comparison. A non-consistent head forces an
// inconclusive result because the observation cannot then be attributed to a
// historical logical rollback.
type RecoverySetClassification struct {
	BeforeOutcome string `json:"before_outcome"`
	AfterOutcome  string `json:"after_outcome"`
	HeadOutcome   string `json:"head_outcome"`
	Outcome       string `json:"outcome"`
}

// ForkRecoverySetExecution is the complete V2.3 recovery artifact. It
// replaces a pair as the result used for discovery claims while keeping pair
// execution available for compatibility fixtures.
type ForkRecoverySetExecution struct {
	SchemaVersion       string                    `json:"schema_version"`
	RecoverySetID       string                    `json:"recovery_set_id"`
	SeedID              string                    `json:"seed_id"`
	FrontierID          string                    `json:"frontier_id"`
	RecordedPlanID      string                    `json:"recorded_plan_id"`
	MaterializationHead MaterializationHead       `json:"materialization_head"`
	RetentionPolicy     RetentionPolicy           `json:"retention_policy"`
	ContinuationQuery   *ContinuationQuery        `json:"continuation_query,omitempty"`
	Before              RecoveryObservation       `json:"before"`
	After               RecoveryObservation       `json:"after"`
	Head                RecoveryObservation       `json:"head"`
	Classification      RecoverySetClassification `json:"classification"`
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
	frozenContinuation, err := freezeContinuationQuery(pair.ContinuationQuery)
	if err != nil {
		return nil, err
	}
	before, err := executeForkQuery(ctx, pair.Before, plan, frozenContinuation, executor)
	if err != nil {
		return nil, fmt.Errorf("execute before query: %w", err)
	}
	after, err := executeForkQuery(ctx, pair.After, plan, frozenContinuation, executor)
	if err != nil {
		return nil, fmt.Errorf("execute after query: %w", err)
	}
	if before.RuntimeInstanceID == after.RuntimeInstanceID {
		return nil, fmt.Errorf("before and after recovery queries reused runtime instance %q", before.RuntimeInstanceID)
	}
	classification := ClassifyForkPair(before, after)
	return &ForkPairExecution{
		SchemaVersion:     ExecutionSchemaVersion,
		ComparisonPairID:  pair.ComparisonPairID,
		SeedID:            pair.SeedID,
		FrontierID:        pair.FrontierID,
		RecordedPlanID:    pair.RecordedPlanID,
		ContinuationQuery: frozenContinuation,
		Before:            before,
		After:             after,
		Classification:    classification,
	}, nil
}

// ExecuteForkRecoverySet invokes all three controls with a fixed recorded
// head and retention policy. It rejects runtime reuse across any controls so
// an observation cannot inherit physical state from another query.
func ExecuteForkRecoverySet(ctx context.Context, seed objective.StateSeed, set HistoricalRecoverySet, plan RecordedPlan, executor ForkExecutor) (*ForkRecoverySetExecution, error) {
	if executor == nil {
		return nil, fmt.Errorf("fork executor is required")
	}
	if err := seed.Validate(); err != nil {
		return nil, err
	}
	if err := set.ValidateFor(seed); err != nil {
		return nil, err
	}
	if err := plan.ValidateFor(seed); err != nil {
		return nil, err
	}
	if err := validateRecoverySetAgainstPlan(set, plan); err != nil {
		return nil, err
	}
	frozenContinuation, err := freezeContinuationQuery(set.ContinuationQuery)
	if err != nil {
		return nil, err
	}
	before, err := executeForkQuery(ctx, set.Before, plan, frozenContinuation, executor)
	if err != nil {
		return nil, fmt.Errorf("execute before query: %w", err)
	}
	after, err := executeForkQuery(ctx, set.After, plan, frozenContinuation, executor)
	if err != nil {
		return nil, fmt.Errorf("execute after query: %w", err)
	}
	head, err := executeForkQuery(ctx, set.Head, plan, frozenContinuation, executor)
	if err != nil {
		return nil, fmt.Errorf("execute head query: %w", err)
	}
	if before.RuntimeInstanceID == after.RuntimeInstanceID || before.RuntimeInstanceID == head.RuntimeInstanceID || after.RuntimeInstanceID == head.RuntimeInstanceID {
		return nil, fmt.Errorf("historical recovery set reused a runtime instance across controls")
	}
	classification := ClassifyForkRecoverySet(before, after, head)
	return &ForkRecoverySetExecution{
		SchemaVersion:       ExecutionSchemaVersion,
		RecoverySetID:       set.RecoverySetID,
		SeedID:              set.SeedID,
		FrontierID:          set.FrontierID,
		RecordedPlanID:      set.RecordedPlanID,
		MaterializationHead: set.MaterializationHead,
		RetentionPolicy:     set.RetentionPolicy,
		ContinuationQuery:   frozenContinuation,
		Before:              before,
		After:               after,
		Head:                head,
		Classification:      classification,
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

func validateRecoverySetAgainstPlan(set HistoricalRecoverySet, plan RecordedPlan) error {
	if plan.MaterializationHeadID != set.MaterializationHead.HeadID || plan.RetentionPolicy != set.RetentionPolicy || set.RetentionPolicy != RetentionPolicyRetainRelevantOSState {
		return fmt.Errorf("historical recovery set does not preserve its recorded head/retention contract")
	}
	if set.RecordedPlanID != plan.RecordedPlanID || set.PassiveObservationID != plan.PassiveObservationID {
		return fmt.Errorf("historical recovery set does not preserve the recorded plan and passive observation")
	}
	for _, query := range []RecoveryQuery{set.Before, set.After, set.Head} {
		if query.RecordedPlanID != plan.RecordedPlanID || query.PassiveObservationID != plan.PassiveObservationID || query.MaterializationHeadID != plan.MaterializationHeadID || query.RetentionPolicy != plan.RetentionPolicy {
			return fmt.Errorf("historical recovery set query changes a recorded recovery condition")
		}
	}
	return nil
}

func executeForkQuery(ctx context.Context, query RecoveryQuery, plan RecordedPlan, continuation *ContinuationQuery, executor ForkExecutor) (RecoveryObservation, error) {
	frozenContinuation, err := freezeContinuationQuery(continuation)
	if err != nil {
		return RecoveryObservation{}, err
	}
	if query.ContinuationQueryID == "" && frozenContinuation != nil {
		return RecoveryObservation{}, fmt.Errorf("recovery query %q does not bind supplied continuation", query.QueryID)
	}
	if query.ContinuationQueryID != "" && (frozenContinuation == nil || frozenContinuation.ContinuationQueryID != query.ContinuationQueryID) {
		return RecoveryObservation{}, fmt.Errorf("recovery query %q does not bind supplied frozen continuation", query.QueryID)
	}
	observation, err := executor.ExecuteFork(ctx, ForkExecutionRequest{Query: query, Plan: plan, ContinuationQuery: frozenContinuation})
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

func ClassifyForkRecoverySet(before RecoveryObservation, after RecoveryObservation, head RecoveryObservation) RecoverySetClassification {
	pair := ClassifyForkPair(before, after)
	headOutcome := classifyObservation(head)
	outcome := pair.Outcome
	if headOutcome != "consistent" {
		outcome = "inconclusive"
	}
	return RecoverySetClassification{
		BeforeOutcome: pair.BeforeOutcome,
		AfterOutcome:  pair.AfterOutcome,
		HeadOutcome:   headOutcome,
		Outcome:       outcome,
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

func WriteForkRecoverySetExecution(path string, execution ForkRecoverySetExecution) error {
	if execution.SchemaVersion != ExecutionSchemaVersion {
		return fmt.Errorf("unsupported fork recovery-set execution schema %q", execution.SchemaVersion)
	}
	return writeRecoveryJSON(path, execution)
}
