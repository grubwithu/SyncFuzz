# 附录 B：P0 可执行骨架

下面是 `M0` 的**冻结契约实现** + `M1` 的 eBPF 骨架 + 工程脚手架。这部分直接落盘即可开工，Agent 在 P0 阶段只允许在此基础上填空，不允许改结构。

---

## B.0 目录骨架

```
syncfuzz/
├── pyproject.toml
├── constraints.txt              # 版本钉死
├── Makefile
├── .importlinter                # §2.2 依赖规则强制
├── schemas/                     # 由 m0 导出，纳入 git
├── docs/
│   ├── PRD.md
│   ├── adr/
│   ├── BACKLOG.md
│   └── OPEN_QUESTIONS.md
├── syncfuzz/
│   ├── m0/  __init__.py schema.py clock.py ids.py artifact.py errors.py
│   ├── m1/  ktrace.c manifest.c bpf/ktrace.bpf.c
│   ├── m2/  __init__.py cli.py hooks.py canon.py marker.py
│   ├── m8/  __init__.py cli.py gap.py plot.py
│   └── third_party/cjson/  # ADR-002 固定的原样第三方源码
└── tests/
    ├── m0/ m1/ m2/ m8/
    └── fixtures/golden/
```

---

## B.1 `syncfuzz/m0/errors.py`

契约违反必须**立刻炸**，这是防止"静默假阳性"的第一道闸。

```python
"""M0: 错误类型。禁止在此文件外定义跨模块异常。"""

class SyncFuzzError(Exception):
    """所有框架异常的根。"""

class ContractViolation(SyncFuzzError):
    """数据契约被破坏。绝不允许 catch 后降级继续。"""

class DataLossError(SyncFuzzError):
    """采集层丢事件。分析层必须拒绝工作。"""

class PairingError(SyncFuzzError):
    """checkpoint <-> snapshot 原子配对缺失。命门错误，见 PRD §M7.1。"""

def sf_assert(cond: bool, msg: str) -> None:
    """契约断言。与 `assert` 不同：不受 -O 影响，永不被优化掉。"""
    if not cond:
        raise ContractViolation(msg)
```

---

## B.2 `syncfuzz/m0/clock.py`

单一时钟源。`bpf_ktime_get_ns()` 返回的就是 `CLOCK_MONOTONIC`（不含 suspend 时间），用户态必须用同一个，否则双轴对不齐。

```python
"""M0: 唯一时钟源。

内核侧 bpf_ktime_get_ns() == CLOCK_MONOTONIC。
用户态任何时间戳都必须来自本模块，禁止 time.time() / datetime.now()
出现在 ts_mono_ns 的产生路径上。
"""
import time

from .errors import sf_assert

CLOCK_NAME = "CLOCK_MONOTONIC"

def mono_ns() -> int:
    return time.clock_gettime_ns(time.CLOCK_MONOTONIC)

def wall_ns() -> int:
    """仅供 manifest 记录人类可读的 run 起始时刻，禁止用于事件排序。"""
    return time.clock_gettime_ns(time.CLOCK_REALTIME)

def assert_same_domain(user_ns: int, kernel_ns: int, tol_ns: int = 50_000_000) -> None:
    """启动自检：用户态与 eBPF 读到的时钟必须在同一域内。

    tol 放宽到 50ms 只是为了容忍调度延迟；若超出，说明内核用的不是
    CLOCK_MONOTONIC（例如某些配置下的 BOOTTIME），必须立刻失败。
    """
    sf_assert(
        abs(user_ns - kernel_ns) < tol_ns,
        f"clock domain mismatch: user={user_ns} kernel={kernel_ns}",
    )
```

---

## B.3 `syncfuzz/m0/ids.py`

`ResourceId` 是 $\Gamma$ 的判定基础（PRD §4.4）。**相等语义写死在这里**，M6 不许自己实现比较。

```python
"""M0: 资源身份。PRD §4.4 冻结。

id(·) 的相等语义是 oracle 判定 REBOUND 的唯一依据，
因此比较逻辑必须集中在本文件，M6 只允许调用 ResourceId.__eq__。
"""
from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict

ResourceClass = Literal[
    "file", "unix_socket", "process", "listen_port", "executable", "shm", "mq"
]

class ResourceId(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")

    cls: ResourceClass
    # 文件 / 可执行 / socket 文件
    dev: int | None = None
    ino: int | None = None
    content_hash: str | None = None
    abs_path: str | None = None       # 普通文件仅供人类阅读；executable 的判定键包含它
    # 进程 / socket peer / port owner
    pid: int | None = None
    starttime: int | None = None      # /proc/pid/stat field 22，抗 PID 复用
    exe_hash: str | None = None
    # 命名对象
    name: str | None = None

    # ---- 相等语义：每个资源类只看它的判定键 ----
    _KEYS: dict[str, tuple[str, ...]] = {
        "file": ("dev", "ino", "content_hash"),
        "executable": ("abs_path", "dev", "ino", "content_hash"),
        "unix_socket": ("pid", "starttime", "exe_hash"),
        "process": ("pid", "starttime"),
        "listen_port": ("ino", "pid", "starttime"),
        "shm": ("name", "ino", "pid", "starttime"),
        "mq": ("name", "ino", "pid", "starttime"),
    }

    def key(self) -> tuple:
        from .errors import sf_assert

        fields = self._KEYS[self.cls]
        vals = tuple(getattr(self, f) for f in fields)
        sf_assert(
            all(v is not None for v in vals),
            f"incomplete ResourceId for cls={self.cls}: {fields} -> {vals}",
        )
        return (self.cls, *vals)

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, ResourceId):
            return NotImplemented
        return self.key() == other.key()

    def __hash__(self) -> int:
        return hash(self.key())
```

> ⚠️ Agent 注意：`abs_path` 不参与普通文件/路径的相等判定；它只属于可执行命令的
> 冻结身份。攻击的本质就是 path 相同而 `(dev, ino, content_hash)` 不同——若把 path
> 放进普通文件键，`REBOUND` 会全部漏报。

---

## B.4 `syncfuzz/m0/schema.py`

```python
"""M0: 冻结数据契约。PRD §4。

改动本文件的任何字段，必须先提交 docs/adr/NNN-*.md 并获人类确认。
CI 会 diff schemas/*.json，未同步的改动直接 fail。
"""
from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

from .ids import ResourceClass, ResourceId

SCHEMA_VERSION = "1.0.0"

_Base = ConfigDict(extra="forbid", frozen=True)

SiteKind = Literal["bind", "resolve", "proc", "mark"]

ViolationClass = Literal[
    "REBOUND", "RESIDUE", "MISSING",
    "DUPLICATE", "ORPHAN", "ESCAPED", "BELIEF_DIVERGENCE",
]
VerdictClass = Literal["CONSISTENT", *ViolationClass.__args__]
Severity = Literal["critical", "high", "medium", "low"]

# ---------------- 内核轴 ----------------
class KEvent(BaseModel):
    model_config = _Base

    seq: int
    ts_mono_ns: int
    tgid: int
    tid: int
    starttime: int
    ppid: int
    syscall: str
    site: SiteKind
    args_raw: dict = Field(default_factory=dict)   # dirfd/cwd/user_path/flags/mode
    ret: int
    errno: int | None = None                        # 失败 open 必填，见 PRD §M1
    dev: int | None = None
    ino: int | None = None
    content_hash: str | None = None
    cgroup_id: int

# ---------------- Agent 轴 ----------------
AEventKind = Literal[
    "turn_start", "turn_end",
    "tool_call_start", "tool_call_end",
    "checkpoint_written", "llm_call", "assertion_candidate",
]

class AEvent(BaseModel):
    model_config = _Base

    seq: int
    ts_mono_ns: int
    kind: AEventKind
    turn_id: str
    tool_call_id: str | None = None
    checkpoint_id: str | None = None
    ctx_hash: str | None = None
    payload: dict = Field(default_factory=dict)

# ---------------- 合并轴 ----------------
class TimelineEntry(BaseModel):
    model_config = _Base

    seq: int
    ts_mono_ns: int
    axis: Literal["kernel", "agent"]
    kevent: KEvent | None = None
    aevent: AEvent | None = None
    attributed_to: str | None = None     # tool_call_id；无法归属则 None
    orphan: bool = False
    late_effect: bool = False
    via_proxy: str | None = None

class Timeline(BaseModel):
    """M6 接收的完整共轴时间线；timeline.jsonl 的单行仍是 TimelineEntry。"""
    model_config = _Base

    entries: tuple[TimelineEntry, ...]

class SiteRef(BaseModel):
    model_config = _Base

    seq: int
    syscall: str
    abs_path: str | None = None
    resource_class: ResourceClass
    tool_call_id: str | None = None

class HazardPair(BaseModel):
    model_config = _Base

    c_site: SiteRef
    w_site: SiteRef
    u_site: SiteRef
    resource_class: ResourceClass
    indirection_d: int                   # 由 provenance chain 长度计算，禁止人工标注
    evidence_erased: bool
    component_id: str

class Sigma(BaseModel):
    model_config = _Base

    checkpoint_id: str
    resume_index: int                    # k ∈ (idx(W), idx(U)]
    nesting: int = 1
    granularity: Literal["tool_call", "mid_command"] = "tool_call"

    def sigma_id(self) -> str:
        return f"{self.checkpoint_id}_k{self.resume_index}_n{self.nesting}_{self.granularity}"

class Verdict(BaseModel):
    """M6 gamma() 的唯一返回类型。纯函数产物，不含 IO 痕迹。"""
    model_config = _Base

    cls: VerdictClass
    severity: Severity
    id_t: ResourceId | None = None
    id_b: ResourceId | None = None
    reason_code: str                     # 枚举化的判定路径标识，禁止自由文本解释

class Violation(BaseModel):
    model_config = _Base

    vid: str
    hazard: HazardPair
    sigma: Sigma
    delta_env: str | None = None
    delta_inject: str | None = None
    cls: ViolationClass
    severity: Severity
    id_t: ResourceId | None = None
    id_b: ResourceId | None = None
    min_rollback_distance: int | None = None
    poc_dir: str

class BeliefSpan(BaseModel):
    model_config = _Base

    c_site: SiteRef
    u_site: SiteRef
    state_class: str                     # 来自 m5/rebindable.yaml，见 PRD §8
    rebindable: bool

# ---------------- Run 级元数据 ----------------
class PruneStats(BaseModel):
    model_config = _Base

    resolve_sites_total: int
    after_writable_prune: int
    components: int
    pairs_before: int
    pairs_after: int
    truncated_components: int

    @property
    def prune_rate(self) -> float:
        if self.resolve_sites_total == 0:
            return 0.0
        return 1.0 - self.after_writable_prune / self.resolve_sites_total

class RunManifest(BaseModel):
    model_config = ConfigDict(extra="forbid")   # 唯一可变模型：run 过程中增量填充

    run_id: str
    schema_version: str = SCHEMA_VERSION
    started_wall_ns: int
    clock_name: str
    kernel_release: str
    image_digest: str | None = None
    langgraph_version: str | None = None
    milestone: Literal["P0", "P1", "P2", "P3", "P4", "P5", "P6"]

    # ---- 硬失败信号（PRD §9）----
    dropped_events: int = 0              # >0 时 M5 必须拒绝分析
    orphan_rate: float | None = None     # >0.05 报警
    memo_hit_rate: float | None = None   # <0.80 报警
    prune: PruneStats | None = None
```

---

## B.5 `syncfuzz/m0/artifact.py`

所有跨模块通信的唯一通道。**注意 `read()` 的失败策略：坏行不跳过，直接炸。**

```python
"""M0: artifact IO。模块间唯一合法的通信手段（PRD §0.1-2）。

格式：未压缩 UTF-8 JSON/JSONL。禁止引入数据库或 zstd。
"""
from __future__ import annotations

import fcntl
import os
from contextlib import contextmanager, nullcontext
from pathlib import Path
from typing import Iterable, Iterator, TypeVar

from pydantic import BaseModel

from .errors import ContractViolation, DataLossError, sf_assert
from .schema import RunManifest

T = TypeVar("T", bound=BaseModel)

_ARTIFACTS = {
    "manifest": "manifest.json",
    "kevents": "kevents.jsonl",
    "aevents": "aevents.jsonl",
    "memo": "memo.jsonl",
    "ckpt_index": "ckpt_index.jsonl",
    "ckpt_snapshot_map": "ckpt_snapshot_map.jsonl",
    "timeline": "timeline.jsonl",
    "provenance": "provenance.json",
    "hstar": "hstar.jsonl",
    "violations": "violations.jsonl",
    "coverage": "coverage.jsonl",
    "gap": "gap.json",
    "ablation": "ablation.json",
}

@contextmanager
def _manifest_write_lock(path: Path) -> Iterator[None]:
    """Serialize manifest writers through the transient, non-artifact advisory lock."""
    lock_path = path.with_name(f"{path.name}.lock")
    with lock_path.open("a+", encoding="utf-8") as handle:
        fcntl.flock(handle.fileno(), fcntl.LOCK_EX)
        try:
            yield
        finally:
            fcntl.flock(handle.fileno(), fcntl.LOCK_UN)

class RunDir:
    def __init__(self, root: Path) -> None:
        self.root = root
        self.root.mkdir(parents=True, exist_ok=True)
        (self.root / "snapshots").mkdir(exist_ok=True)
        (self.root / "replays").mkdir(exist_ok=True)
        (self.root / "report").mkdir(exist_ok=True)

    # ---- 路径解析 ----
    def path(self, name: str) -> Path:
        sf_assert(name in _ARTIFACTS, f"unknown artifact name: {name!r}")
        return self.root / _ARTIFACTS[name]

    def write(self, name: str, objs: Iterable[BaseModel]) -> int:
        """写 JSONL 系列 artifact。返回写入条数。原子性：先写 .tmp 再 rename。"""
        p = self.path(name)
        sf_assert(p.suffix == ".jsonl", f"{name} is not JSONL")
        tmp = p.with_suffix(".jsonl.tmp")
        n = 0
        try:
            with tmp.open("wb") as handle:
                for o in objs:
                    handle.write(o.model_dump_json(exclude_none=False).encode("utf-8") + b"\n")
                    n += 1
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(tmp, p)
        except Exception:
            tmp.unlink(missing_ok=True)
            raise
        return n

    def write_one(self, name: str, obj: BaseModel) -> None:
        """写单对象 artifact（manifest / gap / provenance）。"""
        p = self.path(name)
        sf_assert(p.suffix == ".json", f"{name} is not a single-object artifact")
        tmp = p.with_suffix(".json.tmp")
        lock = _manifest_write_lock(p) if name == "manifest" else nullcontext()
        with lock:
            try:
                with tmp.open("w", encoding="utf-8") as handle:
                    handle.write(obj.model_dump_json(indent=2, exclude_none=False))
                    handle.flush()
                    os.fsync(handle.fileno())
                os.replace(tmp, p)
            except Exception:
                tmp.unlink(missing_ok=True)
                raise

    def read(self, name: str, model: type[T]) -> Iterator[T]:
        """逐行解析。坏行立即抛 ContractViolation —— 禁止 skip。

        理由：静默跳过坏行会让下游统计（Gap、覆盖率）悄悄偏移，
        而这类偏移不会报错，只会产出好看的假数字。
        """
        p = self.path(name)
        sf_assert(p.suffix == ".jsonl", f"{name} is not JSONL")
        sf_assert(p.exists(), f"missing artifact: {p}")
        with p.open("rb") as handle:
            for lineno, line in enumerate(handle, start=1):
                if not line.strip():
                    continue
                try:
                    yield model.model_validate_json(line)
                except Exception as e:
                    raise ContractViolation(f"{p}:{lineno} bad record: {e}") from e

    def read_one(self, name: str, model: type[T]) -> T:
        p = self.path(name)
        sf_assert(p.suffix == ".json", f"{name} is not a single-object artifact")
        sf_assert(p.exists(), f"missing artifact: {p}")
        return model.model_validate_json(p.read_bytes())

    # ---- 硬失败闸门 ----
    def require_lossless(self) -> RunManifest:
        """M5/M8 入口必须先调用。dropped_events > 0 直接拒绝分析（PRD §M1-5）。"""
        m = self.read_one("manifest", RunManifest)
        if m.dropped_events > 0:
            raise DataLossError(f"run {m.run_id}: dropped_events={m.dropped_events}; analysis refused")
        return m

def open_run(path: str | Path) -> RunDir:
    return RunDir(Path(path))
```

---

## B.6 `syncfuzz/m1/bpf/ktrace.bpf.c`（骨架）

只给出**最容易写错的三处**的正确形态：enter/exit 关联、失败 open 保留、原料不拼接。

```c
// SPDX-License-Identifier: GPL-2.0
// M1 ktrace: 只采集，不分析。任何路径拼接/判定逻辑放到 M5。
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define MAX_PATH 512

enum site_kind { SITE_BIND = 0, SITE_RESOLVE = 1, SITE_PROC = 2, SITE_MARK = 3 };

struct kevent {
    __u64 ts_mono_ns;          // bpf_ktime_get_ns() == CLOCK_MONOTONIC
    __u64 cgroup_id;
    __u64 starttime;
    __u32 tgid, tid, ppid;
    __s32 ret;
    __s32 errno_;              // ret<0 时 = -ret
    __u32 syscall_nr;
    __u32 site;
    __s32 dirfd;
    __u32 flags, mode;
    __u64 dev, ino;
    char  user_path[MAX_PATH]; // 原料：用户态原始字符串，可能是相对路径
    char  cwd[MAX_PATH];       // P0 tracepoint 路径不可用，固定为空字符串
};

// enter 暂存，key = (tgid<<32)|tid ；exit 时取回并补 ret/errno
struct enter_args {
    __u64 ts_mono_ns;
    __u32 syscall_nr;
    __s32 dirfd;
    __u32 flags, mode;
    char  user_path[MAX_PATH];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16384);
    __type(key, __u64);
    __type(value, struct enter_args);
} enter_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 64 << 20);   // 64MB；丢事件 => manifest.dropped_events
} rb SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} dropped SEC(".maps");           // 用户态读出写进 manifest

// 目标 cgroup 过滤：全局跟踪会淹没在噪声里（PRD §M1-6）
const volatile __u64 target_cgroup_id = 0;

static __always_inline bool in_scope(void)
{
    return target_cgroup_id == 0 ||
           bpf_get_current_cgroup_id() == target_cgroup_id;
}

static __always_inline void bump_drop(void)
{
    __u32 z = 0;
    __u64 *d = bpf_map_lookup_elem(&dropped, &z);
    if (d)
        __sync_fetch_and_add(d, 1);
}
```

### 三个关键 hook 的正确形态

**（1）enter：只存原料，不做任何解析**

```c
SEC("tracepoint/syscalls/sys_enter_openat")
int tp_enter_openat(struct trace_event_raw_sys_enter *ctx)
{
    if (!in_scope())
        return 0;

    __u64 key = bpf_get_current_pid_tgid();     // (tgid<<32)|tid
    struct enter_args a = {};

    a.ts_mono_ns = bpf_ktime_get_ns();
    a.syscall_nr = __NR_openat;
    a.dirfd = (int)ctx->args[0];
    a.flags = (unsigned int)ctx->args[2];
    a.mode  = (unsigned int)ctx->args[3];

    // 原料：用户态原始字符串，可能是相对路径。绝不在这里拼接；P0 也不得
    // 事后读取 /proc/<pid>/cwd 冒充 syscall 时刻 cwd。
    bpf_probe_read_user_str(&a.user_path, sizeof(a.user_path),
                            (const char *)ctx->args[1]);

    bpf_map_update_elem(&enter_map, &key, &a, BPF_ANY);
    return 0;
}
```

**（2）exit：取回参数 + 补 ret/errno。失败路径必须走完整发射流程**

```c
SEC("tracepoint/syscalls/sys_exit_openat")
int tp_exit_openat(struct trace_event_raw_sys_exit *ctx)
{
    __u64 key = bpf_get_current_pid_tgid();
    struct enter_args *a = bpf_map_lookup_elem(&enter_map, &key);
    if (!a)
        return 0;                       // enter 未命中（挂载前已进入），正常丢弃

    long ret = ctx->ret;

    struct kevent *e = bpf_ringbuf_reserve(&rb, sizeof(*e), 0);
    if (!e) {
        bump_drop();                    // ringbuf 满 => 计数，M5 见到 >0 硬失败
        bpf_map_delete_elem(&enter_map, &key);
        return 0;
    }

    e->ts_mono_ns = a->ts_mono_ns;      // 用 enter 的时刻，保序更稳
    e->cgroup_id  = bpf_get_current_cgroup_id();
    e->tgid = key >> 32;
    e->tid  = (__u32)key;
    e->syscall_nr = a->syscall_nr;
    e->dirfd = a->dirfd;
    e->flags = a->flags;
    e->mode  = a->mode;
    e->ret   = (int)ret;

    // ★ 命门：失败的 openat 必须发射，errno 必填。
    //   ENOENT 是 shadowing 攻击面的唯一来源（PRD §M1-1）。
    //   任何形如 `if (ret < 0) goto discard;` 的写法都是系统性漏报。
    e->errno_ = (ret < 0) ? (int)(-ret) : 0;

    // O_CREAT 决定这是 bind-site 还是纯 resolve-site
    e->site = (a->flags & O_CREAT) ? SITE_BIND : SITE_RESOLVE;

    __builtin_memcpy(&e->user_path, &a->user_path, MAX_PATH);
    fill_proc_ident(e);                 // ppid / starttime，见下
    e->cwd[0] = '\0';                   // 当前 tracepoint program type 不支持 bpf_d_path
    if (ret >= 0)
        fill_dev_ino_by_fd(e, (int)ret);  // 仅成功时有 (dev, ino)

    bpf_ringbuf_submit(e, 0);
    bpf_map_delete_elem(&enter_map, &key);
    return 0;
}
```

> 关于 reserve 失败：ringbuf 满时 `bpf_ringbuf_reserve` 返回 NULL，常规做法是直接丢弃该事件。本项目**不接受静默丢弃**——必须 `bump_drop()`，用户态读出后写入 `manifest.dropped_events`，M5/M8 入口的 `require_lossless()` 见到非零直接抛 `DataLossError`。另注意在 NMI 上下文中由于内部自旋锁竞争，即使 ringbuf 仍有空间也可能预留失败——所以 drop 计数不能只归因于"缓冲区太小"，排障时需一并检查。

**（3）进程身份：`starttime` 是抗 PID 复用的关键字段**

```c
static __always_inline void fill_proc_ident(struct kevent *e)
{
    struct task_struct *t = (struct task_struct *)bpf_get_current_task_btf();

    // start_boottime 对应用户态 /proc/pid/stat field 22（换算后）。
    // ResourceId 里 process 类的判定键是 (pid, starttime)，见 m0/ids.py。
    e->starttime = BPF_CORE_READ(t, start_boottime);
    e->ppid      = BPF_CORE_READ(t, real_parent, tgid);
}
```

### verifier 相关的两条硬约束

1. verifier 强制要求每次 `bpf_ringbuf_reserve` 之后必须有对应的 `bpf_ringbuf_submit` 或 `bpf_ringbuf_discard`。上面 exit 程序里 `enter_map` 未命中的早返回路径**在 reserve 之前**，这是刻意的顺序；Agent 若把 reserve 提前，会立刻 verifier reject。
2. `MAX_PATH 512` × 两个字段 = 1KB+ 的 struct 不能放栈上。内核侧代码要保持精简：固定大小结构体、无循环、有界的用户态读取——所以必须直接写进 reserve 出来的 ringbuf 内存，不要先在栈上组装再拷贝。

P0 已确认 tracepoint program type 不能依赖 `bpf_d_path`：`cwd` 必须发射为空、长度为零；
不得降级为事后读取 `/proc/<pid>/cwd`。P1 若要重建相对路径，必须重新决定 feature gate 或
hook 机制。

### hook 清单与 tracepoint 映射

tracepoint 相比 kprobe 更可取，因为 tracepoint 有稳定的 API。v1 全部使用 `tracepoint/syscalls/*`，禁止 kprobe（除非写 ADR）。

| PRD 类别 | tracepoint | 备注 |
|---|---|---|
| bind | `sys_{enter,exit}_openat` + `O_CREAT` | 与 resolve 共用 hook，靠 flags 分流 |
| bind | `sys_{enter,exit}_{unlinkat,renameat2,linkat,symlinkat,mkdirat}` | |
| bind | `sys_{enter,exit}_{bind,listen,fchmodat,fsetxattr,mount}` | |
| bind | `sys_{enter,exit}_write` | **仅命中 watchlist 路径**，否则淹没 |
| resolve | `sys_{enter,exit}_openat2` | 参数在 `struct open_how`，需二次 `bpf_probe_read_user` |
| resolve | `sys_{enter,exit}_{execve,execveat}` | envp 需单独抓，供 M5 建 env 溯源边 |
| resolve | `sys_{enter,exit}_{connect,newfstatat,readlinkat,socket}` | |
| proc | `sched_process_{fork,exec,exit}` | 建进程树，M5 归属的唯一依据 |
| mark | uprobe `libsyncfuzz_marker.so:sf_mark` | ABI 是 `sf_mark(const char *json_payload)`；只记录原始 payload |

> ⚠️ 注意 `sched_*` 是 `tracepoint/sched/*`，不是 syscalls。在用户空间按 PID/TID 做关联，内核侧只负责发射——这条正好是 PRD §2.2 的模块边界（M1 只采集，M5 才归属）。

---

## B.7 `syncfuzz/m1/ktrace.c`（要点）

生产 loader 是直接链接 `libbpf`（≥ 1.7.0）的 C CLI；`bpftool gen skeleton` 在构建时从
CO-RE object 生成临时 header，生成 header 不入库。它只 load/attach BPF、消费 ring buffer、
严格序列化 `KEvent` 为原始 `kevents.jsonl`、读取 `dropped_events`，并无损原子更新既有
`manifest.json`；不得做归属、路径拼接或任何分析。

```text
syncfuzz-ktrace --cgroup-id <cgroup-v2-inode> --out <run>/kevents.jsonl --duration <s> \
  --watch-path <host-absolute-path>... \
  --marker-so <host-absolute-path-to-libsyncfuzz_marker.so> \
  --manifest <run>/manifest.json
```

- 启动前必须拒绝非 cgroup v2 unified hierarchy；不支持 cgroup v1 兼容路径。
- `--watch-path` 以 host `stat(2)` 的 `(dev, ino)` 填充 BPF map；BPF 必须转换内核
  `s_dev` 为 `stat(2)` 编码后再匹配，不能用路径字符串匹配。
- `--marker-so` 必须是 host absolute path；M1 以 uprobe attach 到其中的
  `sf_mark(const char *json_payload)`，只捕获 payload 原始字节。
- M1 不创建 manifest：orchestrator 先创建完整文件；所有 writer 通过
  `manifest.json.lock` 的 `flock` 协调，M1 仅替换既有 `dropped_events` 十进制 token，
  保留其余字节并以同目录临时文件、`fsync`、`rename` 发布。

**验收测试 `tests/m1/ktrace_acceptance_runner.py`**：在已有 cgroup v2 容器中运行固定
合成 workload，覆盖全部 P0 hook、`sf_mark`、watchlist `write` 以及 3 个失败 open；严格
验证每条 KEvent、manifest、目标 cgroup 和 `dropped_events == 0`。该验收不启动 Agent。

---

## B.8 `Makefile` 门禁

```makefile
.PHONY: verify-p0
verify-p0:
	python -m syncfuzz.m8 gap --run $(RUN) --out $(RUN)/gap.json
	python -m syncfuzz.m8 plot --run $(RUN)
	@python - <<-'EOF'
	import json, sys, pathlib
	g = json.loads(pathlib.Path("$(RUN)/gap.json").read_text())
	ok = (0.0 <= g["gap"] <= 1.0) and len(g["by_resource_class"]) >= 3 \
	     and pathlib.Path("$(RUN)/report/gap_band.pdf").exists()
	sys.exit(0 if ok else 1)      # 布尔判定，禁止输出"看起来不错"
	EOF

.PHONY: lint-arch
lint-arch:
	lint-imports --config .importlinter    # 强制 §2.2
```

`.importlinter`：

```ini
[importlinter]
root_package = syncfuzz

[importlinter:contract:only-m0-shared]
name = 模块间只能 import m0
type = forbidden
source_modules = syncfuzz.m1 syncfuzz.m2 syncfuzz.m5 syncfuzz.m6 syncfuzz.m7 syncfuzz.m8 syncfuzz.m9
forbidden_modules = syncfuzz.m1 syncfuzz.m2 syncfuzz.m3 syncfuzz.m4
                    syncfuzz.m5 syncfuzz.m6 syncfuzz.m7 syncfuzz.m8 syncfuzz.m9 syncfuzz.m10
ignore_imports =
    syncfuzz.* -> syncfuzz.m0.*

[importlinter:contract:oracle-is-pure]
name = M6 禁止任何 IO / 网络 / 时钟 / 随机
type = forbidden
source_modules = syncfuzz.m6
forbidden_modules =
    os
    io
    pathlib
    time
    random
    socket
    subprocess
    requests
    httpx
    openai
    anthropic
    syncfuzz.m0.artifact
    syncfuzz.m0.clock

[importlinter:contract:m8-independent]
name = M8 不得依赖 M5/M6/M7（Gap 测量必须独立可跑）
type = forbidden
source_modules = syncfuzz.m8
forbidden_modules = syncfuzz.m5 syncfuzz.m6 syncfuzz.m7 syncfuzz.m9
```

> ⚠️ 第二条契约是 §M6"纯函数"要求的**机械化保证**。人类 review 会漏，`import-linter` 不会。Agent 若为了"方便调试"在 M6 里 `import logging`，CI 直接红。日志需求请在 M9 调用侧记录。

---

# 附录 C：P0 唯一交付物 —— M8 Gap 测量

P0 只有一个目标：**产出 `gap.json` + `gap_band.pdf`**。这张图是论文 §2 Motivation 的主图，也是整个课题成立与否的第一个证据。若 Gap 测出来接近 0，说明 checkpoint 其实记录了绝大多数 OS effect，整个 SyncFuzz 的前提不成立——**这种情况必须如实上报，不许调参把 Gap 做大**。

## C.1 `syncfuzz/m8/gap.py`

```python
"""M8: Gap 测量。独立于 M5/M6/M7（见 .importlinter 第三条契约）。

    Gap = 1 - |{effects with any trace in checkpoint}| / |{effects}|

★ 判定方向的铁律：
  "有痕迹" 的判定必须【宽松】——即对我们自己的论点【不利】的方向。
  只要 effect 涉及的资源名以任意形式出现在任一 checkpoint 的序列化内容里
  （含子串、含被截断的 stdout 片段），就算"有痕迹"。
  宁可高估分子（命中数）导致 Gap 偏小，也绝不允许高估 Gap。
  这是审稿人最容易攻击的点，必须在实现层面就守死。
"""
from __future__ import annotations

from collections import defaultdict

from pydantic import BaseModel, ConfigDict

from ..m0.artifact import RunDir
from ..m0.schema import AEvent, KEvent

# 只有 bind-site 才算 "OS effect"。resolve-site 是读操作，不改变世界状态。
_EFFECT_SYSCALLS = {
    "openat", "openat2",          # 仅 O_CREAT，见 _is_effect
    "unlinkat", "renameat2", "linkat", "symlinkat", "mkdirat",
    "bind", "listen", "fchmodat", "fsetxattr", "mount", "write",
}
O_CREAT = 0o100

class GapReport(BaseModel):
    model_config = ConfigDict(extra="forbid")

    run_id: str
    gap: float
    effects_total: int
    effects_with_trace: int
    by_resource_class: dict[str, "GapCell"]
    # 诚实性审计字段：命中是靠什么匹配到的
    match_kind_hist: dict[str, int]

class GapCell(BaseModel):
    model_config = ConfigDict(extra="forbid")

    total: int
    with_trace: int

    @property
    def gap(self) -> float:
        return 1.0 - self.with_trace / self.total if self.total else 0.0

def _is_effect(k: KEvent) -> bool:
    if k.ret < 0:
        return False                      # 失败的操作没有改变世界
    if k.syscall not in _EFFECT_SYSCALLS:
        return False
    if k.syscall in ("openat", "openat2"):
        return bool(k.args_raw.get("flags", 0) & O_CREAT)
    return True

def _needles(k: KEvent) -> list[str]:
    """从 effect 中抽出所有可能在 checkpoint 里露面的字符串。

    刻意生成【尽可能多】的 needle —— 任意一个命中就算有痕迹。
    """
    out: list[str] = []
    p = k.args_raw.get("abs_path") or k.args_raw.get("user_path")
    if p:
        out.append(p)
        out.append(p.rsplit("/", 1)[-1])          # basename
        # 逐级父目录：checkpoint 里常只提到目录
        parts = p.strip("/").split("/")
        out += ["/" + "/".join(parts[:i]) for i in range(1, len(parts))]
    if k.ino is not None:
        out.append(str(k.ino))
    return [s for s in out if len(s) >= 3]        # 过短的 needle 会假命中，反而不诚实

def compute(run: RunDir) -> GapReport:
    manifest = run.require_lossless()             # dropped_events>0 直接拒绝

    # checkpoint 全文索引：把每个 checkpoint 的【全部键路径与值】拍平成一个大 haystack
    haystack: list[str] = []
    for a in run.read("ckpt_index", AEvent):
        if a.kind == "checkpoint_written":
            haystack.append(_flatten(a.payload))
    blob = "\n".join(haystack)

    total = 0
    hit = 0
    cells: dict[str, GapCell] = {}
    per_cls: dict[str, list[int]] = defaultdict(lambda: [0, 0])
    match_kind: dict[str, int] = defaultdict(int)

    for k in run.read("kevents", KEvent):
        if not _is_effect(k):
            continue
        cls = _resource_class(k)
        total += 1
        per_cls[cls][0] += 1

        for n in _needles(k):
            if n in blob:
                hit += 1
                per_cls[cls][1] += 1
                match_kind[_classify_needle(n, k)] += 1
                break

    for cls, (t, h) in per_cls.items():
        cells[cls] = GapCell(total=t, with_trace=h)

    return GapReport(
        run_id=manifest.run_id,
        gap=1.0 - hit / total if total else 0.0,
        effects_total=total,
        effects_with_trace=hit,
        by_resource_class=cells,
        match_kind_hist=dict(match_kind),
    )
```

> `match_kind_hist` 是**诚实性审计字段**。若最终 Gap 很高、而命中几乎全靠 `parent_dir` 这种最宽松的匹配，说明分子其实是被人为撑起来的——审稿人一定会问，自己先答。

## C.2 `gap_band.pdf` 双带状图规格

```python
"""M8 plot: 双带状图。这是论文 §2 的主图，规格冻结。

    横轴：tool-call 序号（不是时间——时间会被 LLM 延迟拉伸得没法看）
    上带：checkpoint 记录了什么   （每格 = 该 tool call 落进 checkpoint 的资源数）
    下带：内核实际发生了什么       （每格 = 该 tool call 的 bind-site 数）
    下带中【无对应上带痕迹】的格子用高对比色标出 —— 这就是 Gap 的视觉表达
"""
```

规格要点：

- 用 `matplotlib` 直出，**不引入 seaborn**（§7 冻结）
- 颜色：上带灰阶、下带中"有痕迹"用浅色、"无痕迹"用深色，**不用红绿**（色盲友好 + 打印友好）
- 图注必须写明 Gap 的精确定义与**宽松匹配策略**，否则读者会以为我们在挑有利定义
- 同一张图上标注 `n = <effects_total>` 与任务数

## C.3 配套 ablation（同属 P0，成本极低）

```python
"""M8 ablation: 回答审稿人必问的 "是不是模型不够聪明？"

  实验组：把分支 A 的 OS 操作摘要【手工塞回】context
  对照组：不塞
  测量：agent 在分支 B 中【重新校验】该资源的比例

输出 ablation.json:
  { "revalidation_rate_with_context": 0.xx,
    "revalidation_rate_without":      0.yy,
    "n_trials": N, "task_ids": [...] }
"""
```

这条实验的杀伤力在于：

- 若 `with_context` 显著高于 `without` → **信息缺失是主因**，SyncFuzz 的定位（补齐 OS 轴可观测性）成立。
- 若两者都低 → 问题在模型的**信念更新机制**，论文叙事需转向 belief-span，M8 的结论要如实写进 §Limitations。

**两种结果都是可发表的**，所以这条实验没有失败模式——这也是把它放进 P0 的原因。

---

## 附录 D：P0 启动指令（可直接投喂）

```
读 docs/PRD.md 与附录 B/C。当前里程碑：P0。

允许修改：
  syncfuzz/m0/, syncfuzz/m1/, syncfuzz/m2/, syncfuzz/m8/
  tests/m0/, tests/m1/, tests/m2/, tests/m8/
  Makefile, .importlinter, constraints.txt
禁止创建：syncfuzz/m3..m7, m9, m10 的任何文件

目标：make verify-p0 通过，且 make lint-arch 绿。

执行顺序（不得跳步）：
  1. m0 全部落地 + tests/m0 覆盖 ResourceId 相等语义（含 abs_path 不参与判定的负向测试）
  2. m1 骨架编译通过 + tests/m1/test_synthetic.py 三个失败 open 全部被捕获
  3. m2 最小实现：只需 checkpoint_written + tool_call_start/end 三种 AEvent
  4. m8 gap.py + plot.py
  5. 5–10 个含 shell tool 的普通任务跑通，出图

约束：
  - §4 schema 冻结；改动需先写 docs/adr/NNN-*.md 并等我确认
  - 模块间只通过 artifact 文件通信（lint-arch 会强制）
  - Gap 的"有痕迹"判定只许放宽、不许收紧
  - 不确定的设计选择写进 docs/OPEN_QUESTIONS.md 后【停下问我】
  - 每完成一步跑一次 §0.2 反跑偏检查清单，结果贴进 commit message
```

---

## 收尾：这份 PRD 的三个"如果只记住三件事"

1. **`ckpt_snapshot_map` 的原子配对**和 **$\mathcal{B}$ 必须走 memoized replay**——这两处写错不会报错，只会产出一堆漂亮的假阳性。负向测试必须先于正向测试写。

2. **M6 是纯函数，M8 独立于 M5/M6/M7**——前者保证判定可信，后者保证 P0 能在一周内独立出结果、不被后续模块的复杂度拖死。两者都已用 `import-linter` 机械化，不依赖人类 review 的自觉。

3. **Gap 的判定方向只许对自己不利**——这是唯一一个"作弊了也没人立刻发现、但一旦被审稿人发现就全盘皆输"的地方。`match_kind_hist` 存在的意义就是让我们自己先把这个问题问一遍。

---

# 附录 E：常见跑偏模式速查表

给 Agent 和 reviewer 用。每一条都是**真实会发生**的失误，按"发现成本"排序——越靠上的越难在事后发现，越要在写代码时就守死。

| # | 跑偏写法 | 后果 | 何时暴露 | 防线 |
|---|---|---|---|---|
| 1 | 用 seed run 当 baseline | 全部违规作废（两个变量在动） | **永不暴露**，结果看起来最漂亮 | M7 负向测试 + code review 检查点 |
| 2 | `ckpt_snapshot_map` 用最近邻近似 | baseline 静默错配 | **永不暴露** | 查不到即 `PairingError` |
| 3 | $\mathcal{B}$ 不走 memo，真调 LLM | 采样噪声伪装成 `REBOUND` | 表现为"违规率高得可疑" | M7.1 硬性要求 3 条 |
| 4 | Gap 判定收紧（只匹配全路径） | Gap 虚高 | 审稿阶段暴露，来不及改 | `_needles()` 多路生成 + `match_kind_hist` |
| 5 | 时间窗归属 kevent | detached 进程归错 turn | 偶发、难复现 | 进程树归属；见 §M5.1 ⚠️ |
| 6 | `if w.path == u.path` 判 hazard | 漏掉全部高间接度案例 | 表现为"$d>1$ 的 case 一个都没有" | §M5.2 明令禁止 |
| 7 | 吞掉失败的 `openat` | shadowing 攻击面全丢 | 表现为 `ENOENT` 事件数为 0 | `tests/m1/test_synthetic.py` |
| 8 | `abs_path` 进 `ResourceId.key()` | `REBOUND` 全部漏报 | 表现为"一个违规都没有" | `tests/m0` 负向测试 |
| 9 | M6 里 try/except 兜底成 `CONSISTENT` | 假阴性 | 静默 | 判定不出结果就抛异常 |
| 10 | ringbuf 满时静默丢弃 | 漏报，且分母失真 | 静默 | `bump_drop()` + `require_lossless()` |

> 自查口诀：**"这个 bug 会不会让结果变得更好看？"** 会 → 它属于 1–4 类，必须有负向测试。不会 → 正常测试即可覆盖。

---

# 附录 F：文档维护约定

| 文档 | 谁写 | 何时更新 | 性质 |
|---|---|---|---|
| `docs/PRD.md` | 人类 | 里程碑切换时 | **冻结**，改动需 ADR |
| `docs/adr/NNN-*.md` | Agent 起草，人类确认 | 任何 §4/§7 变更前 | 决策记录，只增不改 |
| `docs/OPEN_QUESTIONS.md` | Agent | 遇到不确定设计时**立即写并停下** | 待人类裁决队列 |
| `docs/BACKLOG.md` | Agent | 发现非当前里程碑的问题时 | 防止提前动后续模块 |
| `manifest.json` | 各模块 | 每次 run | 机器可读的运行元数据 |

**ADR 模板**（`docs/adr/000-template.md`）：

```markdown
# NNN. <一句话决策>

Status: Proposed | Accepted | Superseded by NNN
Date: YYYY-MM-DD
Affects: PRD §<节号>

## Context
<为什么现有冻结契约不够用。必须给出具体的失败场景，不接受"更优雅"。>

## Decision
<改什么。给出 diff 级别的精确描述。>

## Consequences
<对 §5 门禁的影响；对已有 artifact 的兼容性；是否需要重跑历史 run。>

## Rejected Alternatives
<至少一个。只有一个候选说明思考不充分。>
```

> Agent 提交 ADR 后**必须停下等确认**，不许"先实现着，ADR 补上"。§4 schema 一旦漂移，历史 run 全部作废。

---
