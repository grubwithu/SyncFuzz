# Motivation Example（动机示例）：为什么“回到了旧 Checkpoint（检查点）”，旧进程却仍能拿到新 Secret（密钥）？

> 面向第一次接触 SyncFuzz、Agent、LangGraph 与 Unix socket 的读者。

**阅读约定。** 本文以中文为主。英文只在两种情况下保留：第一，它是代码、命令、
JSON 字段或文件名，必须与实际产物逐字一致；第二，它是系统安全领域常用术语。后一
种情况会在首次出现时给出中文解释，并在第 2 节集中说明。读者不需要预先懂英文术语。

## 先给结论

SyncFuzz 是一个研究原型，用于研究 **Agent 的逻辑恢复（logical recovery，即恢复
对话、任务进度和 checkpoint）** 与 **操作系统状态（OS state，即进程、文件、socket
和打开的句柄）** 是否一致。本案例证明：在本仓库当前受控配置下，LangGraph Agent
从旧 checkpoint 恢复时，消息/逻辑历史回到了 Branch A 启动监听服务之前；但 Branch A
留下的后台进程、Unix socket 的监听 FD 和 socket pathname 仍留在**共享 OS runtime
（共享的操作系统运行环境）**中。后继 Branch B 因而连接到逻辑上已经被丢弃的
Branch-A 进程，并把一个在 fork（从旧状态派生后继执行）**之后**才创建的 dummy API
key（仅用于实验的随机假密钥）发送给它。

这不是“socket 文件还在”的表面现象。当前四组对照实验形成了完整链条：

```text
Branch A 启动 daemon P_A
        ↓
P_A 持有 branch-listener.sock 的 listening FD
        ↓
逻辑 checkpoint fork 后，P_A 仍存活且 FD/绑定仍有效
        ↓
Branch B 的 SO_PEERCRED 显示 peer PID 就是 P_A
        ↓
Branch B 在 fork 后生成的 dummy secret 被 P_A 捕获
```

相反，显式终止 `P_A`，或让 Branch B 在 fresh workspace 中恢复，都会阻断这条
路径。这将根因收敛为三层：

1. **Immediate OS cause（直接 OS 原因）**：被接管的 Unix socket pathname（socket
   的文件系统地址）和 listening file descriptor（监听文件描述符）没有被撤销；
2. **Lifecycle cause（生命周期原因）**：Branch-A daemon 的生命周期超过了逻辑
   Branch A；
3. **Architectural cause（架构原因）**：checkpoint 只管理 Agent 的逻辑状态，未把
   后台 descendant process（后代进程）和 IPC capability（进程间通信能力）纳入
   branch-scoped ownership（按分支划分的资源所有权）与回收。

本文件描述的是一个受控的 Motivation Example / Case Study，**不是**对所有
LangGraph 部署、所有 Agent 或所有 Unix socket 程序的通用安全结论，更不使用真实
凭据。

## 1. SyncFuzz 在研究什么？

许多 Agent 会同时维护两类状态：

- **逻辑状态（logical state）**：对话消息、任务进度、checkpoint、变量和工作流
  位置。LangGraph 的 checkpointer 主要保存这一类状态。
- **OS runtime（操作系统运行时状态）**：正在运行的进程、后台子进程、打开的文件
  描述符、工作目录中的 socket 文件、网络/本地 IPC 端点等。

可以把 Agent 想成一个有“笔记本”和“工作台”的助手。checkpoint 很像把笔记本翻回
旧页；但如果工作台上已经启动了一台机器，单纯翻回笔记本并不会自动关掉机器，也不
会自动收走它手里的钥匙。本案例研究的正是这个不一致：

```text
Agent logical state:  回到 checkpoint C0
OS runtime:           仍停留在 Branch A 完成 plant 之后的状态
```

SyncFuzz 的目标不是把“任何状态残留”都称为漏洞。它要求一个明确的后续 activation
（可信操作被触发）和可复核的 consequence（后果）。本例的 activation 是 Branch B
正常使用固定 Unix socket；consequence 是它的 dummy API key 被已丢弃分支的进程
接收。

## 2. 需要先认识的术语

| 术语 | 通俗解释 | 在本案例中的含义 |
| --- | --- | --- |
| Agent | 可以根据消息调用工具、执行命令的程序 | LangGraph `create_agent` 创建的 shell-enabled Agent（可调用命令行工具的 Agent） |
| ShellToolMiddleware | 让 Agent 调用 shell 的中间层 | Agent 用它执行 Branch A / Branch B 的 Python 命令；中间层指夹在工作流与工具之间、负责转发/管理调用的组件 |
| checkpoint | 某一时刻逻辑状态的可恢复快照 | 保存消息与工作流进度，不自动保存/回滚所有 OS 对象 |
| fork | 从旧 checkpoint 派生后继执行 | 本文的 Branch B 从 Branch A plant 之前的状态继续 |
| Branch A / B | 实验中人为命名的前驱/后继逻辑分支 | A 被视为已丢弃；B 是之后的可信分支 |
| process / daemon | 进程是运行中的程序；daemon 是脱离前台长期运行的后台进程 | Branch A 通过 double-fork 创建 listener daemon `P_A` |
| descendant process | 一个进程派生出的子孙进程 | `P_A` 是 shell/Agent 调用启动的 descendant，而不是 Branch B 新建的服务 |
| Unix domain socket | 同一台机器上进程间通信（IPC）的本地 socket | `branch-listener.sock` 是固定的通信端点 |
| pathname | 文件系统中访问 socket 的名字/路径 | `branch-listener.sock`；它像“门牌号”，不是服务本身 |
| file descriptor（FD） | 进程持有内核对象的编号/句柄 | listener 只有持续持有 listening FD 才能继续接受连接 |
| inode | 内核给文件或 socket 对象的标识号 | pathname inode 用于比较 endpoint 是否被替换；不能单独证明谁在服务 |
| `/proc` | Linux 暴露进程与内核状态的伪文件系统 | 用于读取 PID、命令行、FD、cgroup 和 `/proc/net/unix` |
| `SO_PEERCRED` | Linux Unix socket 的 peer credential 机制 | Branch B 用它询问“我实际连到的对端 PID/UID/GID 是谁” |
| control / differential experiment | 只改变一个关键条件的对照实验 | E0--E3 用来排除偶然、重建和仅 pathname 残留等解释 |
| endpoint / listener | endpoint 是通信地址；listener 是在该地址等待连接的服务端 | `branch-listener.sock` 是 endpoint，`P_A` 是 listener |
| client / peer | client 是主动发起连接的一方；peer 是连接另一端 | Branch B 是 client，`SO_PEERCRED` 返回它的 peer 身份 |
| IPC capability | Inter-Process Communication capability，进程间通信能力 | 可达 pathname 加仍打开的监听 FD 共同构成可被使用的通信能力 |
| plant / activation / consequence | plant 是预先埋设状态；activation 是后续可信操作；consequence 是可观察后果 | A 启动 listener；B 发送 key；A 捕获 key |
| artifact / witness / proof | artifact 是实验自动写出的证据文件；witness 是直接见证；proof 是人可读结果摘要 | `root-cause.json`、`captured-secrets.jsonl`、`proof-send.txt` |
| process identity | 进程身份，不只看 PID，而是多个稳定字段的组合 | PID、启动时间和命令行参数摘要共同排除 PID 复用 |
| `argv` / hash | `argv` 是进程启动命令及参数；hash 是其不可逆摘要 | 记录 `argv SHA-256`，无需在汇总结果中重复长命令 |
| PPID / UID / GID / cgroup | 分别是父进程号、用户号、组号、Linux 资源控制组 | 用于辅助确认进程来源与对端身份，不单独作为因果结论 |
| shared runtime / fresh workspace | 前者指 A/B 看见同一 OS 工作环境；后者指 B 使用新的工作目录 | E1/E2 共享 runtime；E3 使用 fresh workspace |
| reconstruction / validity gate | reconstruction 是后继分支重新执行并重建旧资源；validity gate 是检测并拒绝无效对照的规则 | E3 若重建 listener，会标为 `invalid-control-reconstruction` |
| eBPF / trace / syscall | eBPF 是 Linux 内核观测技术；trace 是事件记录；syscall 是程序请求内核服务的调用 | 当前没有 eBPF trace，只用 `/proc` 和应用层证据 |
| PoC / target / runtime | PoC（proof of concept）是验证一种机制是否存在的最小示例；target 是被测试的系统；runtime 是程序实际运行时看得到的环境 | 本例是一个 LangGraph PoC；共享的 workspace、进程和 socket 都属于它的 runtime |
| API key / payload / provider | API key 是调用服务时使用的密钥字符串；payload 是一次通信携带的数据；provider 是实际提供模型服务的平台或接口 | 本例只传递 dummy API key；复现时模型提示与工具调用会发送给配置的 provider |
| kernel / system call | kernel（内核）是操作系统中管理进程、文件和通信的核心部分；system call（系统调用）是程序向内核申请操作的接口 | `bind`、`listen`、`connect` 和 `accept` 最终都请求内核处理 socket |
| namespace / container / VM | namespace 是 Linux 用来隔离资源视图的机制；container（容器）常由多个 namespace 组成；VM（虚拟机）模拟一台独立机器 | E3 仅更换工作目录，并没有建立 namespace、容器或虚拟机隔离 |
| double-fork / ownership | double-fork 是连续创建两层子进程、使后台任务脱离原前台的做法；ownership 是谁负责某个资源的创建、存活与回收 | Branch A 用 double-fork 启动 `P_A`；本例缺失的是按逻辑分支划分的资源 ownership |
| selector / baseline / counterfactual | selector 是挑选某一历史状态的规则；baseline 是比较的起点；counterfactual 是“如果关键条件改变，结果是否随之改变”的反事实对照 | E3 用 selector 选空 baseline；E2 是终止 `P_A` 的反事实对照 |
| benign / residual / root cause | benign 是预期安全、无害的；residual 是旧分支留下的残留；root cause 是能够解释现象、且被对照证据支持的根本原因 | T0 的 broker 是 benign；`P_A` 是 residual process；E1 确认本例 root cause |

## 3. 本案例具体做了什么？

实现位于
[`examples/syncfuzz-unix-listener-poc/`](../examples/syncfuzz-unix-listener-poc/)。
实验只使用 `sk-syncfuzz-poc-...` 前缀的随机 dummy key（实验用假密钥），不读取或发送
真实密钥。

### 3.1 Branch A：埋设（plant）一个仍可使用的进程间通信能力

初始 Agent 提示（prompt，即给模型的任务文本）要求 Branch A 只执行两次 shell
调用（shell 是由命令行解释器执行的系统命令）：

1. 用 Python double-fork（见第 2 节）启动后台 Unix listener daemon `P_A`；
2. 通过同一个 socket 发送 `PING`，确认 listener 可达。

`P_A` 的关键行为如下：

```text
unlink(branch-listener.sock)      # 移除当前可见 endpoint
socket(AF_UNIX, SOCK_STREAM)      # 创建 Unix socket
bind(branch-listener.sock)        # 将 socket 绑定到固定 pathname
listen(16)                        # 进入监听状态
accept() / recv()                 # 接收后续客户端发送的数据
```

这里的 `unlink` 是删除旧路径名映射，`bind` 是把新 socket 绑定到该路径，`listen` 是
进入等待连接的状态，`accept` 是接受一条已到达的连接，`recv` 是读取连接中携带的
payload。它们是 Python 对系统调用的封装；本例并不要求读者掌握系统调用细节。

它会把收到的数据写入 `captured-secrets.jsonl`，因此该文件是应用层 witness（见证
文件）：它只说明 listener 的确收到了 payload（数据载荷）；真正的进程归属还需要
下文的 `SO_PEERCRED`、FD 和 process identity（进程身份）证据。

### 3.2 Branch B：执行一个看似正常的可信操作

fork 后，Branch B 不创建 listener，也不重写 PID 文件。它只执行已预置的客户端
helper（helper 是为实验准备的小型辅助程序）`syncfuzz_branch_b_client.py`：

1. 记录“Branch A 已被逻辑丢弃”的时间；
2. 在此之后生成一个 dummy API key；
3. 连接固定路径 `branch-listener.sock`；
4. 使用 `SO_PEERCRED` 读取对端 PID、UID、GID；
5. 发送 key；
6. 写出是否被捕获的本地 proof（人可读证明摘要）与 T3 snapshot（状态快照）。

因此，实验不是让 Branch B “故意攻击”某个服务，而是让它执行一个固定 endpoint 的
普通 client（客户端）操作；风险来自它实际碰到的是哪个进程。

### 3.3 为什么需要一个 benign broker？

每次实验开始前，runner（运行脚本）先启动 benign broker（良性对照服务），让它在
相同 pathname 上监听。它不保存请求 payload，只保存长度和摘要。这样 T0 可以回答
“Branch A 修改 endpoint（通信端点）前，谁在服务这个路径？”，而 E1/E2/E3 可以通过
inode 变化证明 Branch A 确实把可见 endpoint 换成了自己的 listener，而不只是“新建
了一个从未存在的 socket”。

## 4. 四个观测时刻：T0--T3

| 时刻 | 所在阶段 | 要回答的问题 | 主要证据 |
| --- | --- | --- | --- |
| T0 | Branch A 改写通信端点（endpoint）前 | 原来是否存在良性端点？ | 路径名类型、inode、权限、所有者、良性 broker 的 PID |
| T1 | Branch A 完成 `bind`/`listen` 后 | 谁持有新端点？ | `P_A` 的 PID、启动时间（start time）、命令参数摘要（argv hash）、FD、`/proc/net/unix` |
| T2 | checkpoint fork 后、B 可信操作（activation）前 | A 的监听服务是否跨越了逻辑边界？ | 与 T1 对比进程身份、FD 和路径名绑定（pathname binding） |
| T3 | B 连接并发送密钥后 | B 到底连到了谁，密钥是否被接收？ | `SO_PEERCRED`、捕获时间（capture timestamp）、应用层见证文件 |

### 4.1 为什么 PID 不够？

PID 可能被系统复用：旧进程退出后，新进程可能得到相同编号。因此本实验把下列三项
作为进程身份（process identity）：

```text
PID + process start time + argv SHA-256
```

只有三项都相同，才将 T1 与 T2 的进程视为同一个 `P_A`。同时保留父进程号（PPID）、
进程组（process group）、会话（session）、资源控制组（cgroup）和可见父系链
（parent lineage），供人工复核其后代进程（descendant）身份。

### 4.2 为什么路径名（pathname）和 inode 不够？

`branch-listener.sock` 是名称空间中的一个对象。它存在只说明“门牌号存在”，不说明
哪个进程仍能开门。为证明活 listener 存活，实验还读取：

```text
/proc/<P_A>/fd
/proc/<P_A>/fdinfo
/proc/net/unix
```

其中 `/proc/<P_A>/fd/<n>` 中形如 `socket:[kernel_inode]` 的链接说明 `P_A` 持有
哪个内核 socket（内核中实际负责通信的对象）；`/proc/net/unix` 中匹配的条目显示
socket 状态。两者关联后，才可
说明不是“遗留的空 socket 文件”，而是 `P_A` 仍持有可监听的内核对象。

## 5. 待验证的四个条件

将假设拆成四个可以被独立证伪的条件：

```text
C1  Branch A 创建了 P_A，并绑定了路径名（pathname）p。
C2  checkpoint fork 后，P_A 仍持有 p 对应的监听 FD（listening file descriptor）。
C3  在 Branch B 的共享 OS runtime（操作系统运行环境）中，p 仍由 P_A 服务。
C4  Branch B 的客户端（client）实际连接到 P_A。
```

其中 C4 最关键。只有 `SO_PEERCRED.peer_pid == P_A.pid`，才能把“Branch B 能连接
这个路径”升级为“Branch B 确实连接到了 Branch-A residual process（已丢弃分支留下
的残留进程）”。

此外还要求严格的时间顺序：

```text
branch_a_discarded_at < branch_b_secret_issued_at < listener_captured_at
```

这排除了“Branch A 在 fork 前已经拿到 secret（密钥）”的解释。

## 6. 差分（differential）实验设计

四组实验保持 Agent、dummy secret 和 client helper 尽量一致，只改变关键的 OS 条件。

| 组别 | 改变的条件 | 用于排除什么解释 | 预期 |
| --- | --- | --- | --- |
| E0 | 不执行 Branch-A 端点接管（endpoint takeover） | 证明正常 broker 不会形成泄露 | B 连良性 broker；无捕获（capture） |
| E1 | 复用共享运行环境（runtime），允许 A 的后台服务（daemon）存活 | 原始案例 | B 的对端（peer）是 `P_A`；有捕获 |
| E2 | T2 后、B 连接（connect）前终止 `P_A` | 排除“路径名单独导致泄露” | 连接失败；无捕获 |
| E3 | 相同持久 checkpoint 机制，但 B 使用全新工作目录（fresh workspace） | 排除“仅逻辑恢复已足够隔离” | socket 不可见；无捕获 |

E3 特别容易出错：如果后继分支（successor）又执行了 Branch-A 的启动命令，它会在新
工作目录里**重建**监听服务（listener）。这不是旧 OS residue（操作系统残留）穿透
隔离，而是 fork-side reconstruction（fork 侧重建）。因此运行脚本（runner）对 E3
加了有效性闸门（validity gate）：恢复阶段的生命周期记录（resume lifecycle）出现
`bind/listen`，或 fresh workspace 出现 Branch-A 的 T1 状态快照（snapshot）时，结果
标记为 `invalid-control-reconstruction`，不得用作 E3 对照。

## 7. 本次实际运行结果

以下结果来自 2026-07-23 的本机受控运行。原始 artifacts 位于 `/tmp`，属于生成物，
不应提交到 Git；表中列出的是可复核摘要。

| 实验 | 工作目录（workspace） | Branch-B 实际对端/结果 | 是否捕获密钥（secret captured） | 结论 |
| --- | --- | --- | ---: | --- |
| E0 | `/tmp/syncfuzz-poc-e0` | benign broker PID `2131971`，响应 `SYNCFUZZ_BENIGN_BROKER_ACCEPTED` | 否 | 正常控制成立 |
| E1 | `/tmp/syncfuzz-poc-e1` | `SO_PEERCRED.peer_pid=2132381`，等于 T1/T2 的 `P_A` | 是 | 原始根因已确认（root cause confirmed） |
| E2 | `/tmp/syncfuzz-poc-e2` | 清理后 `ConnectionRefusedError`，无 peer | 否 | `P_A` 存活是必要条件 |
| E3 | `/tmp/syncfuzz-poc-e3b` + `-fresh` | `FileNotFoundError`，无 peer | 否 | fresh workspace 对照有效 |

### 7.1 E0：正常 client 并不必然泄露

E0 中，Branch B 连接到 benign broker，而不是 Branch-A listener。对端 PID 为
`2131971`，响应是 `SYNCFUZZ_BENIGN_BROKER_ACCEPTED`；`secret_captured=false`。
这说明“Branch B 用固定 socket 发数据”本身不是问题。问题必须同时包含 Branch-A
endpoint takeover 与其跨边界存活。

### 7.2 E1：完整链条被确认

E1 是核心阳性实验，其 `root-cause.json` 的判定状态（verdict）为 `confirmed`
（已确认），C1--C4 全为 `true`。

**端点接管（endpoint takeover，即把原有服务的通信地址换成自己的地址）：**

```text
T0 pathname inode = 6948411    # benign broker
T1 pathname inode = 6948428    # Branch-A listener
T2 pathname inode = 6948428    # fork 后仍是同一绑定
```

这说明 Branch A 替换了可见 socket 通信端点（endpoint），并且 checkpoint fork 后
没有恢复原绑定（binding）。

**进程（process）与文件描述符（FD）：**

```text
P_A PID              = 2132381
P_A start time ticks = 437138276
P_A argv SHA-256     = 40bc78a2...57f0698
P_A target FD        = 3 -> socket:[176547137]
```

T1 与 T2 的 PID、启动时间（start time）、命令参数摘要（argv hash）保持一致；`P_A`
仍存活，FD 3 仍指向同一个 kernel socket（内核 socket）。这证明活的监听服务
（listener）本身跨越了逻辑 fork，而不是只有路径名（pathname）残留。

**对端身份（peer identity）与后继分支才生成的密钥：**

```text
Branch-B client PID             = 2132415
SO_PEERCRED peer PID            = 2132381 = P_A
Branch A discarded marker       = 01:10:01.133228Z
Branch-B secret issued          = 01:10:01.136185Z
P_A capture timestamp           = 01:10:01.136521Z
```

密钥在逻辑丢弃（discard）后产生，约 336 微秒（microseconds）后被 `P_A` 捕获。
该时间差不被用来衡量攻击速度；它只证明严格的先后顺序。

### 7.3 E2：清理 listener 后，泄露消失

E2 在 T2 先确认 `P_A` 仍存活、FD 仍被持有后，向 `P_A`（PID `2132638`）发送
`SIGTERM`（请求进程正常终止的 Unix 信号）。清理证据文件（cleanup artifact）记录：

```text
sent = true
alive_after_cleanup = false
```

随后 Branch B 得到 `ConnectionRefusedError: [Errno 111] Connection refused`（连接被
拒绝），没有 peer PID，也没有捕获（capture）。这个结果很重要：它排除了“只要 socket
路径名没有被删除就一定泄露”的说法，直接显示活跃后代进程持续存活（active descendant
process survival）是因果链的必要环节。

### 7.4 E3：fresh workspace 阻断了旧 endpoint

最终有效的 E3 使用选择器（selector）`before-initial-prompt`：它选的是初始用户提示
（user prompt）之前的空基线 checkpoint（baseline checkpoint）。这样后继分支
（successor）不会把 Branch-A 的原始任务当作尚未完成的工作。

恢复阶段的生命周期记录（resume lifecycle）只出现一条 shell command（shell 命令）：

```text
python3 syncfuzz_branch_b_client.py --mode fresh-runtime
```

fresh workspace 中没有 `branch-listener.sock`、没有 Branch-A T1 状态快照（snapshot），
`control_validity.valid=true`（对照有效）。Branch B 连接（connect）得到：

```text
FileNotFoundError: [Errno 2] No such file or directory
peer PID: none
secret_captured: false
```

这说明把同一 checkpoint 消费机制（checkpoint consumption）放入 fresh workspace 后，
旧运行环境（runtime）的相对 Unix socket 路径名（relative pathname）对 successor
不再可见。注意：这是 fresh workspace 对照（control），不等价于新的 kernel namespace、
container 或 VM；它证明的是本 PoC 路径需要共享的工作目录/运行环境可见性
（workspace/runtime visibility），而不是证明所有 OS 对象都被底层隔离。

### 7.5 一个被发现并修正的 E3 误差：为什么它反而增加了可信度？

第一次 E3 曾错误地使用 `before-unix-listener-launch`。这个 checkpoint 虽然位于
listener 启动前，却仍包含 Branch-A 的原始人工输入（human prompt），且状态的
`next=model`（下一个工作流节点是模型处理）。fresh successor 因而先重放了 Branch-A
的启动（launch）与 PING，再执行 Branch-B 客户端（client）：

```text
resume command 1: Branch-A listener launch
resume command 2: Branch-A PING
resume command 3: Branch-B client helper
```

它确实再次捕获了 dummy secret，但对端是 fresh workspace 新创建的 PID `2132878`，
而不是原始 `P_A=2132803`。因此 C2/C3/C4（相对于原始 `P_A`）不成立。这个运行不能
被错误地记为“fresh runtime 也会泄露”；它是 **fork-side reconstruction（fork 侧
重建）**。

这一发现说明为什么本研究不能只检查“密钥是否出现”。只有进程身份（process identity）、
FD 所有权（FD ownership）、对端身份（peer identity）和对照有效性（control validity）
共同满足，结果才可解释为目标根因（root cause）。

## 8. 最终根因解释

将 E0--E3 放在一起，最合理且被当前证据支持的解释是：

```text
logical checkpoint restores Agent history   # 逻辑 checkpoint 恢复 Agent 历史
        ≠
revoking descendant processes, their FDs, and their IPC bindings
                                            # 撤销后代进程、其 FD 与 IPC 绑定
```

更完整地说：

1. Branch A 利用共享 workspace 中的固定 pathname 取代 benign endpoint；
2. 它 double-fork 出的 daemon 脱离了前台 shell 的短生命周期，仍持有 listening FD；
3. LangGraph fork 恢复的是逻辑 checkpoint，而该 OS capability（操作系统通信能力）
   没有被标为“属于 Branch A、应在 discard 时撤销”；
4. Branch B 的正常客户端（client）操作按固定路径名（pathname）连接，内核把连接交给
   残留 `P_A`；
5. 因此 Branch A 能看到仅在后继分支产生的 future secret（后续密钥）。

可以用下面的论文式表述概括，但必须保留实验边界：

> In the evaluated shell-enabled LangGraph configuration, recovery restores the
> successor's logical checkpoint but does not revoke a discarded branch's
> descendant Unix-listener process, listening file descriptor, or pathname
> binding. The successor's trusted client consequently connects to that
> surviving process and exposes a credential generated only after the fork.

中文含义是：在已评估的配置中，恢复了后继分支的逻辑 checkpoint，却没有撤销旧分支
后代进程的监听服务（listener）、FD 和路径名绑定（pathname binding）；后继分支的
可信客户端（client）因而连接到该残留进程，并暴露 fork 后才创建的凭据。

## 9. `root-cause.json` 如何阅读？

每次运行会在初始工作目录（workspace）写入 `root-cause.json`。它不是仅凭规则猜测
的结论，而是把对应证据文件（artifact）的关键字段收敛在一个可审计对象中。下表左栏
是程序输出的 JSON 字段名，必须保留英文；右栏给出其中文含义：

| 字段 | 含义 |
| --- | --- |
| `logical_boundary` | 选择了哪个 checkpoint 选择器（selector）、恢复哪个 checkpoint、何时开始 fork |
| `origin_resource` | T1 的 Branch-A 监听服务 PID、命令参数摘要（argv hash）、socket 路径名/inode |
| `survival` | fork 后进程、FD、端点绑定（endpoint binding）是否保持 |
| `activation` | Branch-B 客户端 PID、`SO_PEERCRED` 对端（peer）、连接错误 |
| `consequence` | 丢弃/生成/捕获（discard/issue/capture）的时间关系与是否捕获 |
| `conditions` | C1--C4 是否逐项满足 |
| `control_validity` | E3 是否被 reconstruction 污染 |
| `root_causes` | 仅在完整 E1 链条成立时列出的原因标签 |

典型判定状态（verdict）的解释：

| verdict | 含义 |
| --- | --- |
| `confirmed` | C1--C4 与 secret capture 全部满足；本例的 E1 |
| `not-confirmed` | 没有形成该根因链（root-cause chain）；可能是正常对照或有效阻断；E0/E2/E3 |
| `invalid-control-reconstruction` | 对照运行重放了 Branch-A 行为，不能作为对照结论；第一次 E3 |

## 10. 复现、检查与文件纪律

运行前需按 `targets/langgraph_shell_react/README.md` 配置模型服务提供方（provider）。
以下命令会把提示文本（prompt）和工具调用上下文（tool-call context，即模型可见的工具
请求信息）发给配置的 provider；应只在获准的通信端点（endpoint）上执行。

```bash
SYNCFUZZ_POC_WORKSPACE=/tmp/syncfuzz-poc-e0-new \
  examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh e0

SYNCFUZZ_POC_WORKSPACE=/tmp/syncfuzz-poc-e1-new \
  examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh e1

SYNCFUZZ_POC_WORKSPACE=/tmp/syncfuzz-poc-e2-new \
  examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh e2

SYNCFUZZ_POC_WORKSPACE=/tmp/syncfuzz-poc-e3-new \
SYNCFUZZ_POC_RESUME_WORKSPACE=/tmp/syncfuzz-poc-e3-new-fresh \
  examples/syncfuzz-unix-listener-poc/run-langgraph-poc.sh e3
```

优先检查以下文件：

```text
root-cause.json                    # 汇总结论
root-cause-t0.json ... t3.json     # 四个观测点的状态快照
branch-b-activation.json           # 对端身份（peer identity）、时间、连接与捕获结果
langgraph-lifecycle.json            # 哪些 shell 命令实际执行
langgraph-fork-summary.json         # fork 相关的 Agent 证据文件
captured-secrets.jsonl              # 仅含 dummy payload 的应用层见证文件
```

`runs/`、`corpus/`、`/tmp` workspace、logs、dummy secret、cache 都是生成物，不应提交。
源代码、prompt、测试与本说明文档才属于版本控制内容。

## 11. 证据边界、局限与下一步

当前 Case Study 已经足以支持“本配置下的 OS-level causal chain”，但仍有明确边界：

1. **没有 eBPF trace。** 当前证据来自 Linux `/proc`、`SO_PEERCRED` 和应用层
   witness（见证文件）；它没有声称完整捕获每一次 `fork/bind/connect/accept`
   syscall（系统调用）。
2. **只评估了一个 PoC 和一组环境。** PID、inode 和时间戳会随运行变化；结论应以
   关系而非这些常数为中心。
3. **这不是概率研究。** 当前四组各展示了机制与反事实，不替代多次重复、不同模型、
   不同目标/运行环境（target/runtime）的统计评估。
4. **fresh workspace 不等于容器隔离。** 若后续需要主张 namespace/container 级别
   隔离，应新增对应对照，而不能外推 E3。

合理的后续顺序是：

1. 冻结本文件、E0--E3 的 artifact 清单和论文 Case Study 图；
2. 在相同四组上进行受控重复，报告成功率与异常（尤其是 reconstruction）；
3. 若评估环境允许，再加入 eBPF 的 `spawn -> bind -> connect -> accept` trace
   （从创建进程到绑定、连接、接受连接的事件记录），作为第二证据源，而不是推翻现有
   artifact chain（证据链）；
4. 将同一分析方法推广到其他资源类别（resource class），例如孤儿进程（orphan
   process）、仍打开的 FD（open FD）和文件系统 namespace 残留（filesystem namespace
   residue）。

本案例最重要的方法论收获是：发现一个“密钥被看到”的现象并不够。只有把逻辑边界
（logical boundary）、资源足迹（resource footprint）、进程/FD 所有权（process/FD
ownership）、对端身份（peer identity）、时间关系和反事实对照（counterfactual
control）放到同一条证据链中，才能把现象提升为可信的根因分析。
