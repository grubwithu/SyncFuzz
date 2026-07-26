# Legacy V3 Prototype Status

## Decision

The current V3 recovery-hazard prototype is frozen as engineering evidence,
not as the implementation of the research method. Do not use
`synthesis-langgraph-v3-five-control` for new experiments. The Make target
fails closed accordingly.

The prototype established that a LangGraph container can retain a real
Unix-domain listener across an exact durable-checkpoint restore, and that
eBPF can link target `bind`, `listen`, and recovery-time `connect` events to
run-local socket identities. Those are reusable implementation lessons, not
method-level findings.

## Why It Is Not a Valid Fuzzer Closure

1. The original target materialized an `EnvironmentProgram` outside the
   Agent's normal task. A checkpoint after that time was incorrectly treated
   as evidence that the Agent knew the active environment binding.
2. The old “retention ablation” used a separate clean environment runtime.
   It changed both E and the retained runtime, rather than holding E and the
   historical logical state fixed while removing only retained OS state.
3. The generated task and generic continuation were not sufficient to prove
   one stable normal health-client workload, dynamic configuration resolution,
   or a durable observation of the service identity.

## Worktree Inventory

### Potentially reusable infrastructure

- eBPF resource collector and container cgroup scope resolution;
- LangGraph durable checkpoint manifest and exact structural restore;
- source-runtime lease verification and workspace cloning;
- recovery-time resource tracing;
- generic artifact I/O and reproducibility checks.

### Prototype-specific code to keep out of a new core

- `internal/syncfuzz/environment/target_unix_socket_materialization.go`;
- `internal/syncfuzz/environment/unix_socket_materializer.go`;
- `internal/syncfuzz/hazard/`;
- `synthesis-langgraph-v3-five-control-legacy` and its report model;
- Unix-socket listener role, PID, FD, inode, and acknowledgement details.

These belong, if retained at all, in a LangGraph Unix-socket calibration
adapter. They must not define the general StateFuzzer model.

## Partial Correction Present in This Worktree

The latest uncommitted work changes target execution to materialize E between
the initial task and a normal profiling follow-up. It records a target-owned
post-materialization follow-up artifact and requires profile-time eBPF
`connect` evidence before recovery planning. This is useful scaffolding for a
future adapter, but it still does not supply a real retention ablation and
does not make the V3 report valid.

## New-Project Starting Point

The new implementation should begin from a small independent model:

```text
Workload × EnvironmentProgram × RecoveryProgram
  -> profiling trace
  -> logical-state and materialization evidence
  -> historical-cut selection
  -> recovery-time use evidence
  -> controlled comparison
```

The generic core must not name LangGraph, eBPF, Unix sockets, listeners, or
specific resource identities. Those are adapter concerns.
