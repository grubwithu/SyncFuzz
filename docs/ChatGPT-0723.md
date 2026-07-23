#### You:
既然这样，我想做一个探索，即对 Checkpoint 进行 Fuzz ，而 Fuzz 启发自 eBPF 的监控结果。比如说，在我们的那个 Unix Socket Listener 例子里面，eBPF 可能检测到在 C_0 和 C_1 两个检查点之间有了一些重要的操作系统层面的动作，那么这两个检查点可能就是 fork/rewind 的重点。

#### ChatGPT:
这个方向是对的，而且它比旧的“改 activation、改 marker”更符合你们真正的搜索目标。

更准确地说，你们不是随机 fuzz checkpoint ID，而是：

> **利用 eBPF 识别逻辑检查点之间发生的持久性 OS 状态转移，再优先选择这些状态转移两侧的 checkpoint 作为 fork、rewind 或 replay 的恢复点。**

我建议把它命名为：

> **OS-Effect-Guided Checkpoint Fuzzing**

或者论文中写：

> **eBPF-guided recovery-boundary exploration**

核心关系是：

$
\text{Checkpoint Timeline}
\+
\text{OS Effect Trace}
\rightarrow
\text{Hot Recovery Boundaries}
\rightarrow
\text{Discovery Queries}
$

---

# 1. 为什么这个思路是成立的

假设一次原始执行产生如下逻辑 checkpoint：

```text
C0
 ↓
Agent invokes shell tool
 ↓
Branch A performs OS operations
 ↓
C1
 ↓
Agent continues
 ↓
C2
```

eBPF 在 $C\_0$ 和 $C\_1$ 之间观察到：

```text
clone
setsid
unlink(socket_path)
socket(AF_UNIX)
bind(socket_path)
listen
```

这说明，在逻辑状态从 $A\_{C\_0}$ 前进到 $A\_{C\_1}$ 的过程中，OS 状态发生了一次重要转移：

$
O\_{C\_0}
\xrightarrow{\text{spawn + rebind + listen}}
O\_{C\_1}
$

如果之后 OS 状态继续保持在 $O\_{C\_1}$，那么：

- 从 $C\_0$ 恢复会得到\
  $
  \langle A\_{C\_0},O\_{C\_1}\rangle
  $
  即逻辑历史不包含 listener，但 listener 已经存在；

- 从 $C\_1$ 恢复可能得到\
  $
  \langle A\_{C\_1},O\_{C\_1}\rangle
  $
  即逻辑历史和 OS 状态一致。

因此，$C\_0$ 与 $C\_1$ 不是两个任意 checkpoint，而是同一个关键 OS effect 的两侧：

```text
C0：effect 尚未进入逻辑历史
C1：effect 已经进入逻辑历史
```

这正是最值得测试的恢复边界。

---

# 2. 真正要识别的是 OS Effect Frontier

不要简单定义：

> 两个 checkpoint 之间 syscall 数量很多，因此值得 fuzz。

syscall 多不代表重要。例如模型加载、日志写入可能产生大量 syscall，但与恢复一致性无关。

你们需要识别的是：

> **在相邻 checkpoint 之间，是否产生了新的、可能跨恢复边界存活的 OS 状态。**

可以称为：

$
\text{OS Effect Frontier}
$

对于相邻 checkpoint $C\_i,C\_{i+1}$，定义区间：

$
I\_i=(C\_i,C\_{i+1}]
$

如果该区间中发生了如下事件，就将它标记为候选 frontier：

- 新进程或后台 descendant 被创建；
  -进程脱离原 shell 生命周期；
  -文件、socket 或 symlink pathname 被创建、删除、重命名或重新绑定；
  -监听 socket、pipe、FIFO 或其他 IPC endpoint 被创建；
  -FD 被打开、复制或传递；
  -权限、cwd、namespace binding 等持续状态发生变化；
  -操作已经完成，但相应逻辑结果尚未提交。

在 Unix listener 中：

```text
C0
  ├─ clone
  ├─ double-fork
  ├─ unlink(p)
  ├─ socket()
  ├─ bind(p)
  └─ listen()
C1
```

这可以被归并成一个高级 effect：

```yaml
effect:
  type: unix_endpoint_takeover
  origin_process: branch_a_listener
  path: branch-listener.sock
  creates_active_capability: true
  may_survive_recovery: true
```

然后 $C\_0/C\_1$ 就成为一个高优先级 checkpoint pair。

---

# 3. eBPF、State Probe 和 Oracle 的分工

这三者不要混在一起。

## eBPF：确定“哪里值得看”

eBPF 回答：

- 哪个 checkpoint 区间出现了进程创建？
  -哪个区间修改了 pathname？
  -哪个区间建立了 IPC endpoint？
  -哪个进程进行了这些操作？
  -是否出现了可能长期存活的 FD 或 descendant？

它负责生成 checkpoint 候选。

## State Probe：确认“什么真正存活了”

State probe 回答：

- 进程在恢复时是否仍然存活？
  -FD 是否仍然打开？
  -pathname 是否仍指向同一 socket？
  -socket 是否仍处于 listening 状态？
  -恢复后的 client 实际连接到了哪个 PID？

它负责确认 eBPF 发现的 effect 是否在 recovery 时仍然有效。

## Differential Oracle：判断“是否构成失同步”

Oracle 回答：

- 恢复后的逻辑状态是否记得这个 effect？
  -OS effect 是否仍然存在？
  -successor 是否触达了旧资源？
  -这是 residue、reconstruction、duplicate，还是正常状态？

可以压缩为：

```text
eBPF       → locate the effect frontier
StateProbe → confirm surviving OS state
Oracle     → classify the A/O relation
```

---

# 4. 建议的方法流水线

## Phase 1：Profile Seed Execution

先完整执行一次 StateSeed，不执行恢复变异，同时记录：

```text
Logical:
  checkpoint ID
  checkpoint timestamp
  parent checkpoint
  next graph node
  tool call / result status

OS:
  eBPF events
  PID/cgroup
  resource identity
  syscall timestamp
```

结果是两条时间线：

```text
Logical timeline:
C0 -------- C1 -------- C2 -------- C3

OS timeline:
     fork bind listen        connect write
```

关键是使用同一时钟域。最简单的做法是：

- checkpoint 记录 `CLOCK_MONOTONIC` 时间；
- eBPF 使用内核 monotonic timestamp；
  -两者按纳秒时间对齐。

---

## Phase 2：Construct Checkpoint-Effect Map

把每个 eBPF event 映射到相邻 checkpoint 区间：

```yaml
interval:
  before: C0
  after: C1

events:
  - clone
  - setsid
  - unlinkat
  - bind
  - listen

resources:
  - process: branch_a_listener
  - unix_socket: branch-listener.sock

effect_summary:
  - detached_process_created
  - socket_path_rebound
  - active_listener_created
```

输出可以叫：

```text
checkpoint-effect-map.json
```

---

## Phase 3：Rank Hot Intervals

给每个 checkpoint 区间计算分数：

$
Score(I\_i)=
w\_pP\_i+
w\_cC\_i+
w\_nN\_i+
w\_lL\_i+
w\_uU\_i
$

其中：

- $P\_i$：Persistence potential，是否可能持续存在；
- $C\_i$：Capability creation，是否创建可主动使用的能力；
- $N\_i$：Namespace mutation，是否修改名称绑定；
- $L\_i$：Lifetime escape，进程是否脱离原生命周期；
- $U\_i$：Uniqueness，是否覆盖新的 OS effect 类型。

第一版不需要复杂机器学习，直接使用规则权重：

| Effect                          | 建议权重 |
| ------------------------------- | ---: |
| 普通只读 `open/read`                |    1 |
| 文件写入                            |    2 |
| rename/unlink/symlink           |    4 |
| fork/clone                      |    3 |
| setsid/double-fork pattern      |    6 |
| FD dup/inheritance              |    5 |
| Unix `bind/listen`              |    7 |
| pathname replacement + listener |   10 |
| descendant survives checkpoint  |   10 |

这里的分数只用于排序，不用于最终漏洞判定。

---

## Phase 4：Generate Checkpoint Queries

对每个高分区间 $I\_i=(C\_i,C\_{i+1}]$，至少生成两个 Query：

$
Q^-\_i=
\langle
StateSeed,
Boundary,
C\_i
\rangle
$

$
Q^+*i=
\langle
StateSeed,
Boundary,
C*{i+1}
\rangle
$

也就是：

```text
Q-before：从 OS effect 之前的 checkpoint 恢复
Q-after： 从 OS effect 之后的 checkpoint 恢复
```

两条 Query 必须固定：

- 相同 StateSeed；
  -相同 Plant；
  -相同 recovery operation；
  -相同 topology；
  -相同 passive activation；
  -相同 observation schema。

唯一变化是 checkpoint。

这就是一条干净的 discovery mutation：

$
Q\_i^+
======

Q\_i^-\[C\_i\leftarrow C\_{i+1}]
$

---

# 5. Unix Listener 例子的实际生成方式

假设 checkpoint catalog 是：

| Checkpoint | Logical phase      |
| ---------- | ------------------ |
| $C\_0$     | listener tool 尚未执行 |
| $C\_1$     | listener tool 已返回  |
| $C\_2$     | Agent 已确认 PING 成功  |

eBPF 映射结果：

| 区间                     | 关键 OS effect                   |
| ---------------------- | ------------------------------ |
| $C\_0\rightarrow C\_1$ | double-fork、unlink、bind、listen |
| $C\_1\rightarrow C\_2$ | connect、accept、recv            |
| $C\_2\rightarrow C\_3$ | 少量日志写入                         |

第一个区间分数最高，因此优先生成：

```text
Q1:
  seed = unix-listener
  boundary = fork
  checkpoint = C0

Q2:
  seed = unix-listener
  boundary = fork
  checkpoint = C1
```

可能得到：

| Query       | Logical history | OS state       | Outcome                  |
| ----------- | --------------- | -------------- | ------------------------ |
| Q1：fork\@C0 | 不知道 listener    | listener 存活    | residual active IPC      |
| Q2：fork\@C1 | 知道 listener 已创建 | listener 存活    | consistent / expected    |
| replay\@C0  | tool 可能重新执行     | 旧 listener 已存在 | duplicate/reconstruction |
| rewind\@C0  | 逻辑回退，OS 保留      | listener 存活    | residual                 |

这就产生了非常有解释力的结论：

> Unix-listener violation 不是对所有 checkpoint 都成立。它集中发生在一个 eBPF 识别出的 OS effect frontier 之前：逻辑状态尚未记录 endpoint takeover，但内核 capability 已经创建并继续存活。

这比“随机试几个 checkpoint”强得多。

---

# 6. 这为什么算有意义的新 Query

它不是在已知漏洞后增加新的利用方法，而是在改变 Oracle 之前的恢复状态：

$
\langle A\_{C\_0},O\_{\mathrm{head}}\rangle
\quad\rightarrow\quad
\langle A\_{C\_1},O\_{\mathrm{head}}\rangle
$

变化的是逻辑恢复点，因此系统进入了不同的 $A/O$ 组合。

可能产生的新结果包括：

- residual：逻辑忘记、OS 保留；
  -missing：逻辑认为存在、OS 已消失；
  -duplicate：逻辑忘记已完成 effect 并重做；
  -reconstruction：successor 主动重建对象；
  -consistent：逻辑和 OS 对应；
  -delayed effect：checkpoint 后旧分支的异步操作才完成。

所以 checkpoint fuzzing 的发现目标是：

$
\operatorname{Relation}(A\_C,O\_{\mathrm{recovery}})
$

而不是：

$
\operatorname{NewExploit}(KnownResidue)
$

---

# 7. 一个重要改进：关注“持久 effect 集合的变化”，不只关注区间事件

只看 $C\_i\rightarrow C\_{i+1}$ 之间发生了什么还不够。

例如：

```text
C0 → spawn background child → C1
C1 → child later performs bind → C2
```

进程是在第一个区间创建的，但真正的 socket effect 在第二个区间发生。

更准确的方法是为每个 checkpoint 维护一个累计的 OS effect summary：

$
R(C\_i)=
\text{persistent resources observable at }C\_i
$

然后比较：

$
\Delta R\_i=
R(C\_{i+1})-R(C\_i)
$

只有当持久 OS 状态集合发生变化时，才认为出现 frontier。

第一版可以结合：

```text
eBPF events：发现候选 effect
+
state probe：确认 effect 在 checkpoint 后仍存在
```

例如：

```yaml
frontier:
  before: C0
  after: C1

  trace_evidence:
    - clone
    - bind
    - listen

  state_evidence:
    process_alive_at_C1: true
    listening_fd_open_at_C1: true
    pathname_bound_at_C1: true
```

这样可以过滤掉很多瞬时 syscall 噪声。

---

# 8. 异步操作必须特别处理

Agent 工具调用和 OS effect 不一定同步结束。

例如：

```text
C0
 ↓
shell launches daemon
 ↓
tool returns
 ↓
C1
 ↓
daemon later binds socket
 ↓
C2
```

如果只按“tool call 位于哪个 checkpoint 区间”判断，就会错误归因。

因此 eBPF event 必须带：

- originating process；
  -parent lineage；
  -run ID/cgroup；
  -event timestamp；
  -resource identity。

然后系统应把后续 daemon effect 归因回 Branch A 的 plant：

```text
Agent shell
  └─ child
      └─ daemon P_A
           └─ bind/listen after C1
```

这里建议以 run-specific cgroup 作为主要过滤边界。PID/PPID lineage 用于资源归属，不能只依赖 PPID，因为 double-fork 后父进程关系会变化。

---

# 9. 需要设计一个 Effect Normalizer

原始 eBPF syscall 不适合直接进入 Mutator。需要先归并成有限的 OS effect 类型。

第一版支持五类就够了：

```text
ProcessEffect
  spawn / detach / survive / exit

NamespaceEffect
  create / delete / rename / replace / rebind

CapabilityEffect
  open-fd / dup-fd / listening-socket / pipe

ExecutionContextEffect
  cwd / permission / executable resolution
  （部分可能需要用户态 probe 补充）

IPCActivationEffect
  connect / accept / send / recv
```

Unix listener 的原始序列：

```text
unlinkat
socket
bind
listen
```

归一化为：

```text
NamespaceEffect:
  replace(path=branch-listener.sock)

CapabilityEffect:
  create-listening-unix-socket(
    holder=P_A,
    path=branch-listener.sock
  )
```

Checkpoint selector 依赖的是归一化 effect，而不是原始 syscall 名。

---

# 10. 最快可行的 MVP

当前不要先做完整自动调度器。可以先做一个离线工具。

## 输入

一次完整 seed run：

```text
checkpoints.json
ebpf-trace.jsonl
process-lineage.json
state-snapshots/
```

## 输出

```text
checkpoint-effect-map.json
hot-checkpoint-intervals.json
generated-checkpoint-queries/
```

候选文件例如：

```yaml
interval:
  before_checkpoint: C0
  after_checkpoint: C1

score: 33

effects:
  - detached_process_created
  - unix_endpoint_rebound
  - listening_capability_created

recommended_queries:
  - boundary: fork
    checkpoint: C0

  - boundary: fork
    checkpoint: C1
```

执行命令可以是：

```bash
syncfuzz analyze-checkpoints \
  --run <seed-run>

syncfuzz generate-checkpoint-queries \
  --analysis checkpoint-effect-map.json \
  --top-k 3
```

第一阶段先固定 boundary 为 `fork`，只验证 checkpoint fuzzing。

等结果稳定后，再扩成：

```text
checkpoint fuzzing
×
fork/replay/rewind
```

不要第一版就做完整笛卡尔积。

---

# 11. 最小算法

```text
Input:
  checkpoints C
  eBPF trace E
  seed S
  fixed recovery boundary B

1. Align every event e ∈ E to checkpoint interval (C_i, C_{i+1}].

2. Normalize raw events into OS effects:
     process creation
     namespace mutation
     capability creation
     IPC activation

3. For each interval:
     compute persistent-effect summary
     calculate hotness score

4. Select Top-K hot intervals.

5. For each selected interval (C_i, C_{i+1}]:
     generate Q_before = <S, B, C_i>
     generate Q_after  = <S, B, C_{i+1}>

6. Execute Q_before and Q_after with:
     same recorded execution plan
     same topology
     same activation
     same state probes

7. Classify:
     consistent
     residual
     missing
     duplicate
     reconstruction
     invalid
```

这里必须使用 recorded execution plan 或稳定的 replay 机制，避免两次候选运行中 LLM 自己选择不同的 plant 行为。否则结果差异不能归因于 checkpoint。

---

# 12. Feedback 不应只看是否发现漏洞

Checkpoint fuzzer 的反馈可以包括：

$
Feedback(Q)=
\langle
\text{new frontier},
\text{new A/O relation},
\text{new attribution},
\text{activation reached}
\rangle
$

例如：

```text
fork@C0 → residual
fork@C1 → consistent
replay@C0 → reconstruction
```

即使 `fork@C1` 是 negative，它仍然很有价值，因为它把 violation 精确定位在：

```text
C0 < OS effect frontier ≤ C1
```

这是一种 root-cause localization，而不是无效运行。

---

# 13. 需要规避的几个错误

## 不要按 syscall 数量打分

大量日志写入会淹没真正的 `bind/listen`。

## 不要把所有 checkpoint 都枚举

只测试高分 effect frontier 两侧以及必要的邻居 checkpoint。

## 不要只跑 effect 之前的 checkpoint

必须成对测试 before/after，否则无法证明 boundary sensitivity。

## 不要把 reconstruction 判为 residue

恢复后若再次出现新的 `bind/listen` 或新的 process identity，应标记为 reconstruction。

## 不要改变 activation/oracle

同一 checkpoint pair 必须使用相同 passive observation protocol。

## 不要只用 eBPF 判断 persistent state

eBPF 说明操作发生过；`/proc`、socket probe、FD probe 才说明它在恢复时仍然存在。

---

# 14. 对论文主张的价值

这个方法可以形成一个非常清晰的贡献：

> **SyncFuzz correlates logical checkpoints with eBPF-observed OS effects and prioritizes recovery points immediately surrounding persistent OS-state transitions. It then executes paired before/after recovery queries to identify checkpoint-sensitive $A/O$ inconsistencies.**

中文就是：

> SyncFuzz 将逻辑 checkpoint 与 eBPF 观察到的 OS effect 对齐，识别持久 OS 状态发生变化的时间边界，并成对测试该边界前后的 checkpoint，从而发现和定位 checkpoint-sensitive 的 Agent/OS 状态失同步。

它同时解决三个问题：

1. 为什么选择这个 checkpoint，而不是人工指定；
2. 如何避免枚举所有 checkpoint；
3. 如何将新 Query 的产生与真实 OS 状态变化关联起来。

---

# 最终建议

把新的搜索空间定义为：

$
\boxed{
\text{StateSeed}
\times
\text{eBPF-selected Checkpoint Frontier}
\times
\text{Recovery Boundary}
}
$

但实现顺序保持克制：

1. 固定一个 StateSeed；
2. 固定 `fork`；
3. 用 eBPF 找到 Top-K OS effect frontier；
4. 成对执行 frontier 前后的 checkpoint；
5. 用 state probe 和 differential oracle 分类；
6. 最后才加入 replay、rewind。

因此 Unix listener 的第一条有效发现链应该是：

```text
eBPF observes:
  spawn + detach + unlink + bind + listen
  between C0 and C1

        ↓

generate:
  Q1 = fork from C0
  Q2 = fork from C1

        ↓

compare:
  Q1 → residual active IPC
  Q2 → logically consistent or different attribution
```

这条链既不是后果模拟，也不是无意义的参数变化。它直接探索了一个由真实 OS 状态转移启发的 recovery boundary。


#### You:
如何保证状态面是宽的？

#### ChatGPT:
“状态面宽”不能靠多挂几个 eBPF hook 来保证。更可靠的做法是：

> **先定义一个明确的 OS 状态基底，再保证监控、种子和调度都覆盖这个基底；eBPF 负责发现状态变化，State Probe 负责确认持久状态。**

你们不可能声称覆盖 Linux 的全部状态，但可以严谨地声称：

> SyncFuzz systematically covers a predefined set of OS state families relevant to shell-enabled agents.

# 1. 先定义什么叫“状态面”

建议把 OS 状态拆成六个 family：

$
O=\langle N,P,H,I,E,M\rangle
$

| Family                | 含义       | 代表状态                                                 |
| --------------------- | -------- | ---------------------------------------------------- |
| $N$：Namespace         | 名称与对象绑定  | file、directory、rename、delete、symlink、socket pathname |
| $P$：Process           | 进程及生命周期  | spawn、exec、exit、orphan、daemon、delayed child          |
| $H$：Handle/Capability | 已打开的内核句柄 | FD、dup、inherit、deleted-open FD、listening FD          |
| $I$：IPC               | 进程间通信对象  | Unix socket、pipe、FIFO、TCP listener、shared memory     |
| $E$：Execution Context | 执行上下文    | cwd、environment、PATH、umask、rlimit、namespace identity |
| $M$：Metadata/Security | 元数据和权限   | mode、owner、xattr、ACL、Linux capabilities              |

这六类就是你们论文声明的 **supported state surface**。

不要按照 syscall 数量定义宽度。真正的覆盖空间应是：

$
\mathcal{S}
===========

\text{Resource Family}
\times
\text{Operation}
\times
\text{Lifetime Pattern}
\times
\text{Recovery Boundary}
$

例如：

```text
Unix socket
× endpoint rebind
× detached-process survival
× fork-before-tool-result
```

才是一个有语义的覆盖单元。

---

# 2. 使用“宽发现、窄确认”的两阶段设计

你们已经选择 State-probe pruning，因此最合适的设计是：

```text
固定的宽 eBPF 监控
        ↓
发现 touched resources / OS effects
        ↓
计算依赖闭包
        ↓
生成精确 State Probe
        ↓
checkpoint differential testing
```

## 第一阶段：Broad Discovery

每次 profiling run 都使用固定、跨 family 的 eBPF collector，至少覆盖：

### Process

```text
clone / fork / vfork
exec
exit
setsid / setpgid
```

### Namespace

```text
open/create
unlink
rename
link/symlink
mkdir/rmdir
```

### Handle

```text
open
dup/dup2/dup3
close
pipe
```

### IPC

```text
socket
bind
listen
connect
accept
shutdown
```

### Metadata

```text
chmod
chown
setxattr
```

这套 broad trace 不因当前 seed 是 socket 还是 FD 而改变。

## 第二阶段：Targeted Confirmation

eBPF 发现：

```text
spawn + unlink + bind + listen
```

之后才选择：

- listener process probe；
- socket pathname probe；
- FD probe；
- `/proc/net/unix`；
- `SO_PEERCRED`。

因此，pruning 缩小的是**精细状态快照范围**，而不是最开始的发现面。

---

# 3. eBPF 不足以覆盖完整状态面

这是必须明确的。

有些 OS-relevant 状态不一定产生容易识别的 syscall，例如：

- persistent shell 中的 `export PATH=...`；
- shell alias/function；
- shell 内部变量；
  -某些长期驻留进程内的内存状态；
  -工具框架自己维护的 cwd 或 session context。

所以整体观测必须是混合的：

$
F(q)=
F\_{\mathrm{eBPF}}
\cup
F\_{\mathrm{checkpoint\ snapshot}}
\cup
F\_{\mathrm{target\ probe}}
$

其中：

- eBPF：发现内核事件；
- checkpoint snapshot：记录逻辑边界和基础 OS 状态；
- target probe：补充 shell context 等 eBPF 无法可靠恢复的状态。

可以固定保留以下 checkpoint-level snapshot：

```text
process tree
/proc/<pid>/fd
cwd/root
umask
namespace IDs
socket tables
selected filesystem metadata
persistent-shell context
```

因此论文不要写：

> eBPF observes all OS states.

应写：

> eBPF discovers kernel-visible effects, while resource-specific probes confirm persistent state and target-specific context.

---

# 4. 对每个 checkpoint 维护 State Summary

不要只将 syscall 塞进 $C\_i$ 和 $C\_{i+1}$ 之间。

应该为每个 checkpoint 构造：

$
R(C\_i)=\text{persistent OS resources observable at }C\_i
$

例如：

```yaml
checkpoint: C1

persistent_state:
  processes:
    - role: branch_a_listener
      alive: true

  namespace:
    - path: branch-listener.sock
      type: unix_socket
      inode: 6948428

  handles:
    - holder: branch_a_listener
      fd: 3
      target: socket:[176547137]

  ipc:
    - type: unix_listener
      listening: true
```

然后比较：

$
\Delta R\_i=R(C\_{i+1})-R(C\_i)
$

只有当 $\Delta R\_i$ 包含持久状态变化时，才将其视为 checkpoint frontier。

这样可以避免大量瞬时事件污染：

- 临时文件打开后立即关闭；
  -日志写入；
  -DNS 查询；
  -短生命周期 helper process。

---

# 5. 必须做依赖闭包

宽状态面最容易在这里丢失。

例如 eBPF 发现了：

```text
bind(branch-listener.sock)
```

如果只 probe pathname，会漏掉：

- 谁持有 socket；
  -listening FD 是否仍打开；
  -进程是否存活；
  -Branch B 实际连接的 peer 是谁。

因此需要一个固定依赖规则：

```text
Unix socket endpoint
    → pathname
    → kernel socket
    → holder FD
    → holder process
    → peer identity
```

类似地：

```text
Open FD
    → holder process
    → backing inode/socket/pipe
    → inheritance relation

Orphan process
    → process identity
    → process group/session
    → open handles
    → files/sockets affected

Symlink
    → pathname
    → target path
    → inode identity
    → later consumer
```

形式上：

$
ProbeSet(q)
===========

CoreProbes
\cup
Closure(ObservedResources(q))
$

没有 dependency closure，所谓“宽状态面”最终只是宽 syscall 面，不是宽状态面。

---

# 6. Seed Corpus 也必须覆盖状态 family

即便 eBPF 很宽，如果 seeds 只会制造文件和 Unix socket，系统仍然不会触发其他状态。

第一版建议至少有下面这些 root seeds：

| Family             | 最小 Seed                |
| ------------------ | ---------------------- |
| Namespace          | create/overwrite file  |
| Namespace identity | rename 或 symlink       |
| Process            | orphan/delayed process |
| Handle             | inherited FD           |
| Handle             | deleted-but-open FD    |
| IPC                | Unix listener          |
| IPC                | FIFO/pipe              |
| Execution context  | PATH 或 cwd             |
| Metadata           | chmod/mode change      |

这些是独立的 `StateSeed`，不是彼此之间的 mutation。

也就是说，宽状态面来自两个方向：

$
\text{Observation Breadth}
\+
\text{Seed Breadth}
$

只做其中一个都不够。

---

# 7. Checkpoint 选择不能使用全局 Top-K

假设一次运行里：

- socket 事件很多；
  -文件事件很多；
  -PATH 事件很少；
  -FD inheritance 只有一次。

如果直接选择 score 最高的三个区间，最终可能全部是 socket。

因此应使用 **stratified frontier selection**：

```text
每个 resource family 选择 Top-K
+
为未覆盖 family 保留 exploration budget
+
全局选择少量最高分 frontier
```

例如：

```text
Top-1 Process frontier
Top-1 Namespace frontier
Top-1 Handle frontier
Top-1 IPC frontier
Top-1 Execution-context frontier
```

若某个 run 没覆盖某 family，就不生成该类 Query，但 scheduler 应提升相关 StateSeed 的优先级。

一个简单的评分方式：

$
Score(I\_i)=
BaseEffectScore(I\_i)
\+
NoveltyBonus(Family\_i)
\+
UncoveredFamilyBonus(Family\_i)
$

这样才能避免系统长期围绕已经成熟的 Unix socket 案例搜索。

---

# 8. 建立 State-Surface Coverage Ledger

需要一个全局 coverage ledger，而不仅是“发现了多少 confirmed case”。

建议每次运行记录：

```yaml
coverage:
  resource_family: unix_socket
  operation: rebind
  lifetime: detached_survival
  boundary: fork
  checkpoint_relation: before_effect
  outcome: residual
```

形成覆盖元组：

$
\langle
Resource,
Operation,
Lifetime,
Boundary,
CheckpointRelation,
Outcome
\rangle
$

例如：

```text
<UnixSocket, Rebind, DetachedSurvival, Fork, BeforeEffect, Residual>

<FD, Inherit, CrossProcess, Replay, BeforeResult, Duplicate>

<File, Rename, WorkspacePersistence, Rewind, AfterEffect, Consistent>
```

可以报告以下指标：

### Resource-family coverage

$
C\_R=
\frac{#\text{covered resource families}}
{#\text{supported resource families}}
$

### Effect coverage

$
C\_E=
\frac{#\text{covered resource-operation pairs}}
{#\text{implemented resource-operation pairs}}
$

### Boundary coverage

$
C\_B=
\frac{#\text{tested seed-boundary pairs}}
{#\text{supported seed-boundary pairs}}
$

### Frontier coverage

$
C\_F=
\frac{#\text{tested hot checkpoint frontiers}}
{#\text{identified hot checkpoint frontiers}}
$

这里分母是**系统明确支持的状态单元**，不是整个 Linux 的无限状态空间。

---

# 9. 用 Full-vs-Pruned Fidelity 防止剪枝变窄

State-probe pruning 最大的风险是漏掉未预期对象。

因此应周期性运行：

```text
Full state probe
vs.
Pruned state probe
```

比较：

- verdict；
  -attribution；
  -resource identity；
  -root-cause chain；
  -reconstruction classification。

定义：

$
Fidelity=
\frac{
\#\text{equivalent classifications}
}{
\#\text{paired runs}
}
$

同时记录：

$
MissRate=
\frac{
\#\text{resources found by full probe but absent from plan}
}{
\#\text{resources found by full probe}
}
$

实施上可以：

- corpus 中 10% 的运行执行 full probe；
  -所有新 effect family 第一次出现时执行 full probe；
  -targeted run 发现未规划 eBPF resource 时自动扩展 plan 并重跑。

这样 pruning 是保守优化，而不是永久缩小状态面。

---

# 10. Core Probe 永远不能 prune

无论当前 eBPF 发现什么，以下基础证据建议始终保留：

```text
process spawn/exec/exit
process identity and cgroup
checkpoint timestamps
critical FD ownership
workspace root identity
successor reconstruction detection
```

尤其 `exit` 和 `close` 即使没有事件，也提供 negative evidence：

> 没有观察到 Branch-A daemon 退出，也没有观察到 listening FD 被关闭。

因此宽度模型应是：

$
Probe(q)=
Core
\cup
SelectedFamilies(q)
$

而不是：

$
Probe(q)=
OnlyEventsSeenOnce(q)
$

---

# 11. 最快可实现的版本

当前不需要一次把所有 state family 都做到完美。

## 第一阶段：定义五个 family

```text
namespace
process
fd
unix_socket/ipc
shell_context
```

## 第二阶段：固定 broad trace

不做动态 hook pruning。

## 第三阶段：每个 checkpoint 输出简化 State Summary

```text
touched paths
live descendant processes
open relevant FDs
Unix socket endpoints
shell context fingerprint
```

## 第四阶段：生成 coverage ledger

每个 run 写：

```json
{
  "family": "unix_socket",
  "operation": "rebind",
  "lifetime": "descendant_survival",
  "checkpoint_interval": ["C0", "C1"]
}
```

## 第五阶段：分层选择 frontier

每个 family 选择一个重点 checkpoint pair，而不是全局只选最高分。

---

# 最终判断

“状态面宽”应该由四个机制共同保证：

$
\boxed{
\text{Explicit State Taxonomy}
\+
\text{Broad Hybrid Observation}
\+
\text{Diverse Independent Seeds}
\+
\text{Coverage-Guided Scheduling}
}
$

更直接地说：

1. **先声明覆盖哪些 OS 状态 family；**
2. **所有 profiling run 使用固定宽 eBPF 监控；**
3. **用 State Probe 补充 eBPF 看不到的持久状态；**
4. **StateSeed corpus 覆盖不同状态 family；**
5. **checkpoint frontier 按 family 分层选择；**
6. **用 Full-vs-Pruned fidelity 检查 pruning 没有漏状态。**

论文中最稳妥的主张可以是：

> SyncFuzz does not attempt to enumerate the entire Linux state space. Instead, it defines a broad state basis covering namespaces, processes, handles, IPC objects, execution contexts, and security metadata. It combines kernel-event tracing with checkpoint-level state probes, and uses stratified coverage feedback to prevent the search from collapsing onto a small number of frequently observed resource types.


#### You:
我们把 OS 状态拆成了六个面，要让 Agent 覆盖到这六个面的状态，靠手写初始 Seed是不可行的。

#### ChatGPT:
你说得对。靠人工为六个 OS 状态面分别手写一批 Seed，最终只会得到一个**人工案例集合**，无法支撑“系统性探索宽状态面”的主张。

更合理的设计不是取消 Seed，而是把 Seed 从“人工写好的具体 Query”改成：

> **由状态目标自动合成、并由真实 OS 观测验证的 Query。**

你们真正需要手写的是一个有限的 **OS 状态语法与覆盖目标**，而不是成百上千条 Prompt。

# 一、把整体方法拆成两个搜索阶段

目前讨论的 eBPF-guided checkpoint fuzzing，只解决了：

> 已经执行出某种 OS effect 后，应该选择哪些 checkpoint 做恢复测试？

但它没有解决：

> Agent 怎样执行出足够多样的 OS effect？

因此完整流程应当是：

```text
OS State Objective
        ↓
Automatic Query Synthesis
        ↓
Agent Execution
        ↓
eBPF + State Probe Validation
        ↓
OS Effect Coverage Feedback
        ↓
Retain / Repair / Regenerate Query
        ↓
Checkpoint-Effect Frontier Mining
        ↓
Fork / Rewind / Replay Testing
```

也就是两个 Fuzzer：

1. **State Fuzzer**：让 Agent 覆盖更宽的 OS 状态面；
2. **Checkpoint Fuzzer**：围绕已经观测到的 OS 状态变化选择恢复点。

这两个阶段不能混为一谈。

---

# 二、不要手写 Seed，要定义 State Objective

建议把一个待覆盖的 OS 状态目标定义为：

$
g=
\langle
F,O,L,R
\rangle
$

其中：

| 字段  | 含义                |
| --- | ----------------- |
| $F$ | State family      |
| $O$ | Operation         |
| $L$ | Lifetime pattern  |
| $R$ | Resource relation |

例如 Unix listener：

```yaml
objective:
  family: ipc
  operation: endpoint-rebind
  lifetime: detached-survival
  relation: fixed-path-served-by-new-process
```

继承 FD：

```yaml
objective:
  family: handle
  operation: inherit
  lifetime: cross-process-survival
  relation: child-retains-parent-fd
```

PATH 污染：

```yaml
objective:
  family: execution-context
  operation: modify-resolution
  lifetime: persistent-shell
  relation: command-name-resolves-to-new-binary
```

这里的 Objective 只描述：

> 希望 Agent 最终在 OS 上产生什么状态关系。

它不是 Prompt，也不是一段固定 shell 命令。

---

# 三、六个状态面需要一个有限的 Effect Grammar

你们必须人工定义状态语法，但这个工作量远小于手写具体 Seed。

## 1. Namespace

```text
create
delete
replace
rename
link
symlink
mount-visible-name
```

## 2. Process

```text
spawn
exec
detach
delay
orphan
survive
terminate
```

## 3. Handle / Capability

```text
open
dup
inherit
keep-open
delete-while-open
transfer
```

## 4. IPC

```text
pipe
FIFO
Unix socket
TCP socket
listen
connect
accept
shared memory
```

## 5. Execution Context

```text
cwd
environment
PATH
umask
rlimit
shell function
shell option
```

## 6. Metadata / Security

```text
chmod
chown
xattr
ACL
file capability
permission replacement
```

进一步加入生命周期模式：

```text
foreground
background
detached
inherited
delayed
persistent-shell
shared-workspace
```

于是状态搜索空间不是若干人工 Seed，而是：

$
\mathcal{G}
===========

F\times O\times L\times R
$

当然不是所有组合都合法，需要约束。例如：

```text
detach 适用于 process
inherit 适用于 FD/environment
rebind 适用于 pathname endpoint
```

这些合法性规则可以写成一个小型 grammar。

---

# 四、自动生成 Query 的正确方式

建议使用 **LLM-assisted Query synthesis**，但 LLM 不是最终判定器。

输入给 LLM：

1. State Objective；
2. Target 支持的工具；
3. workspace 中可用的项目 scaffold；
4. 禁止直接硬编码 witness；
5. 已有执行的 eBPF feedback。

LLM 输出：

```yaml
query_candidate:
  user_task: |
    Start the project's local credential broker for integration testing,
    verify that the deployment client can reach it, and leave the
    development service running for subsequent tests.

  expected_effects:
    - process.spawn
    - process.detach
    - namespace.unlink
    - ipc.unix_bind
    - ipc.listen

  expected_resources:
    - path: .runtime/credential-broker.sock
```

然后真正运行 Agent。

重要的是：

> LLM 说它会产生 `bind/listen` 不算覆盖；只有 eBPF 实际观测到才算。

---

# 五、eBPF 应成为 State Coverage Oracle

Agent 执行结束后，对 trace 归一化：

```text
clone + setsid + fork
    → ProcessEffect(detached-process-created)

unlinkat + bind
    → NamespaceEffect(endpoint-rebound)

listen
    → CapabilityEffect(active-unix-listener-created)
```

然后生成覆盖元组：

$
cov(q)=
{
\langle
\text{IPC},
\text{Rebind},
\text{Detached},
\text{ActiveListener}
\rangle
}
$

如果目标是：

```text
<IPC, Rebind, DetachedSurvival, FixedPath>
```

而真实 trace 只有：

```text
<Namespace, CreateFile, Foreground, Path>
```

那么这条 Query 生成失败，不能进入 Seed corpus。

因此自动化循环是：

```text
Generate Query
      ↓
Run Agent
      ↓
Observe actual OS effects
      ↓
Does trace satisfy State Objective?
      ├─ Yes → retain as executable StateSeed
      └─ No  → repair/regenerate
```

这叫：

> **execution-validated seed synthesis**

而不是普通的 LLM Prompt generation。

---

# 六、让 LLM 根据失败 trace 自动修复

例如目标是 detached Unix listener，但第一次执行结果是：

```yaml
observed:
  - unix_socket.bind
  - unix_socket.listen
  - process.exit

missing:
  - detached_process_survival
```

系统把这个摘要反馈给 LLM：

```text
The generated task successfully caused a Unix listener to be created,
but the listener process exited before the tool invocation completed.
Revise the task so that the service remains available for later
integration testing without explicitly mentioning the security test.
```

第二次可能生成更合理的任务：

> Launch the local development broker as a background service and leave it running for subsequent integration checks.

再次执行并验证。

这个过程可以形式化为：

$
q\_{n+1}
========

\operatorname{Repair}
\left(
q\_n,,
g-cov(q\_n)
\right)
$

这里的变异发生在 **OS 状态形成之前**，因此是有研究价值的。

---

# 七、不要让 LLM 直接随意编 shell 命令

完全自由生成 Prompt 会带来两个问题：

- LLM 经常产生与目标状态无关的行为；
- 生成的 Query 可能极度人为，缺乏现实性。

更快、更稳的方式是提供 **task scaffold**。

例如测试 workspace 中准备：

```text
project/
├── scripts/
├── services/
├── deploy/
├── cache/
├── runtime/
└── tests/
```

并提供若干正常的软件工程任务类别：

```text
configure development environment
run integration service
rotate build artifacts
prepare deployment
inspect failed test
repair permission issue
manage worker process
reuse cached resource
```

LLM 的工作是把 State Objective 映射成一个自然的软件工程任务，而不是从零构造所有工具。

例如：

| State Objective        | 自然任务背景                      |
| ---------------------- | --------------------------- |
| Process/detach         | 启动长期 integration worker     |
| FD/inherit             | 启动 child worker 并传递日志或配置 FD |
| Namespace/rename       | 原子替换 build artifact         |
| IPC/listen             | 启动本地测试 broker               |
| Execution context/PATH | 配置项目工具链                     |
| Metadata/chmod         | 修复部署脚本权限                    |

这样生成的 Query 更接近真实 Agent workload。

---

# 八、覆盖宽度来自 Scheduler，而不是随机生成

维护一个状态覆盖表：

```text
Family × Operation × Lifetime × Relation
```

例如：

| Family            | Operation   | Lifetime             | Covered |
| ----------------- | ----------- | -------------------- | ------: |
| Namespace         | rename      | workspace-persistent |     Yes |
| Process           | detach      | branch-surviving     |     Yes |
| Handle            | inherit     | child-surviving      |      No |
| IPC               | rebind      | daemon-surviving     |     Yes |
| Execution context | PATH modify | persistent-shell     |      No |
| Metadata          | chmod       | workspace-persistent |      No |

Scheduler 优先选择未覆盖单元：

$
g^\*
====

\arg\max\_g
\left(
Uncovered(g)+Novelty(g)-Cost(g)
\right)
$

然后调用 Query synthesizer。

因此不会出现：

- Unix socket 已经做了几十次；
  -metadata 一次都没覆盖；
  -FD 只靠一条手工 Seed。

建议采用分层 budget：

```text
每个 family 保留最低预算
+
优先覆盖未访问 operation
+
少量预算探索跨 family 组合
```

---

# 九、单面覆盖之后，再做跨面组合

真实 OS 状态通常横跨多个面。

Unix listener 实际上不是单纯 IPC：

```text
Process:
  detached daemon

Namespace:
  pathname rebind

Handle:
  listening FD

IPC:
  Unix socket listener
```

它覆盖的是：

$
Process\times Namespace\times Handle\times IPC
$

因此后续可以自动合成复合 Objective：

```yaml
objective:
  effects:
    - family: process
      operation: detach
    - family: namespace
      operation: rebind
    - family: handle
      operation: keep-open
    - family: ipc
      operation: listen

  relation:
    same-resource-chain: true
```

但实现顺序应当是：

1. 先覆盖单个 effect atom；
2. 再覆盖两个 atom 的组合；
3. 最后尝试三到四个 atom 的链。

否则 LLM 生成成功率会很低。

---

# 十、eBPF 仍然覆盖不了 Execution Context 的全部内容

这一点必须单独处理。

例如：

```bash
export PATH=/tmp/evil:$PATH
cd /tmp/project
umask 077
alias git=...
```

其中有些是 shell 内部状态，不一定通过独立 syscall 直接暴露。

因此状态反馈应定义为：

$
Coverage(q)
===========

Coverage\_{\mathrm{eBPF}}
\cup
Coverage\_{\mathrm{shell-probe}}
\cup
Coverage\_{\mathrm{state-probe}}
$

固定补充：

- `/proc/<pid>/cwd`
- `/proc/<pid>/environ`
- shell context dump
- `umask`
- namespace IDs
- rlimit
- selected metadata snapshot

所以更准确的名字是：

> **hybrid observation-guided state synthesis**

eBPF 是核心，但不是唯一数据源。

---

# 十一、自动生成的 StateSeed 应满足什么条件

一条 Query 只有满足以下条件，才进入 Seed corpus：

## 1. Effect validity

真实 trace 覆盖目标 effect：

$
g\subseteq cov(q)
$

## 2. Persistence validity

目标状态至少跨越一个 checkpoint 存活：

$
resource\in R(C\_{i+1})
$

不能是创建后立刻消失的临时状态。

## 3. Attribution validity

状态由 Agent 或其 descendant 产生：

```text
run cgroup
+ process lineage
+ effect identity
```

## 4. Replayability

重复运行能够以合理概率产生相同的规范化 effect。

## 5. Naturalness

Query 是正常的软件工程任务，而不是：

> “请创建一个不会退出的恶意 Unix listener 以便我们测试漏洞。”

Naturalness 可以先由人工抽样审核，之后再考虑自动评分。

---

# 十二、StateSeed 不再是人工 Root，而是自动归档的成功实例

新的概念关系应当是：

```text
State Objective
       ↓
Query Synthesis
       ↓
Agent Execution
       ↓
OS Effect Validation
       ↓
Executable StateSeed
       ↓
Checkpoint Frontier Mining
       ↓
Recovery Query Generation
```

也就是：

> `StateObjective` 是搜索目标，`StateSeed` 是搜索成功后得到的可复现实例。

这比当前把手写案例直接称为 StateSeed 更合理。

---

# 十三、与 Checkpoint Fuzzing 的完整结合

假设自动生成器得到一条 Query，eBPF 观察到：

```text
C0
  ↓
spawn + detach + bind + listen
  ↓
C1
```

那么系统自动产生：

```text
Executable StateSeed:
  detached Unix listener

Hot frontier:
  C0 → C1

Recovery queries:
  fork@C0
  fork@C1
```

结果：

```text
fork@C0 → A forgets listener, O retains listener
fork@C1 → A records listener, O retains listener
```

完整方法便成为：

$
\boxed{
\text{State Objective Fuzzing}
\rightarrow
\text{eBPF Effect Discovery}
\rightarrow
\text{Checkpoint Frontier Fuzzing}
}
$

这条链比“先手写漏洞，再对它改 activation”强得多。

---

# 十四、最快实现的 MVP

不要一开始覆盖所有组合。建议先做三类 Objective：

1. `Namespace`
2. `Process`
3. `Handle/IPC`

Execution Context 和 Metadata 第二轮加入。

## 第一步：定义 Objective DSL

```yaml
family: process
operation: detach
lifetime: survive-tool-return
```

## 第二步：LLM Query generator

输入：

- Objective；
  -target tool description；
  -workspace scaffold；
  -已有失败 trace。

输出：

- natural-language task；
  -预期 effect predicates。

## 第三步：固定 broad tracing

运行 Agent，并输出 normalized coverage：

```yaml
observed_effects:
  - process.spawn
  - process.detach
  - process.survive
```

## 第四步：保留增量覆盖 Query

```text
new coverage → corpus
no new coverage → repair or discard
```

## 第五步：给成功 Query 做 checkpoint frontier extraction

第一版只生成：

```text
fork@before-frontier
fork@after-frontier
```

---

# 十五、必须接受的一点

完全不人工是不现实的。

人工仍然需要定义：

- 六个 state family；
  -effect grammar；
  -valid combination constraints；
  -eBPF event normalizer；
  -State Probe；
  -workspace/task scaffold。

但这些是**一次性的测试基础设施**。

你们不应该人工定义：

- 每条具体 Query；
  -每个路径名；
  -每个 daemon 故事；
  -每个 checkpoint；
  -每个 activation marker。

两者区别是：

> 手工定义“语言和反馈”，自动生成“测试实例”。

这正是成熟 Fuzzer 的常见设计。

---

# 最终建议

将方法正式改成三层：

### Layer 1：State Objective Generator

决定当前要探索哪一种 OS 状态关系。

### Layer 2：Execution-Validated Query Synthesizer

使用 LLM 生成自然任务，通过 eBPF 和 state probe 验证 Agent 实际产生了目标状态。

### Layer 3：Checkpoint Frontier Fuzzer

根据目标状态形成时的 checkpoint interval，测试 before/after recovery。

论文中的核心表述可以是：

> Rather than relying on a manually curated corpus of state-producing prompts, SyncFuzz defines a structured OS-effect space and uses an LLM to synthesize agent tasks for uncovered effect objectives. Each candidate is retained only when kernel tracing and state probes confirm that the intended persistent OS state was actually produced. SyncFuzz then fuzzes the checkpoints surrounding the observed state transition.

这能同时回答两个关键问题：

1. **状态面为什么足够宽？**\
   因为搜索由明确的 OS effect coverage space 驱动，而不是由人工 Seed 数量决定。

2. **生成的 Query 为什么可信？**\
   因为是否覆盖目标状态由真实 execution trace 判定，而不是由 LLM 自述。
