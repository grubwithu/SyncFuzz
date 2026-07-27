# SyncFuzz 方法论（统一版）

## 0. 一句话定位

> **把 agent 的 checkpoint/restore 视为一种非确定性来源，将"agent 信念 / OS 状态"一致性问题归约为一个有限、可枚举、可覆盖度量的回退调度空间，并用"理想回滚"反事实基线作为单变量对照 oracle 来判定违规。**

命名建议：**rollback-schedule fuzzing**（方法）/ **belief–state divergence**（bug class）。避免让读者以为你在 fuzz prompt。

---

## 1. 威胁模型与问题定义

### 1.1 系统模型

被测系统 = ⟨LLM 策略, agent 框架（含 checkpointer）, 工具集（含 shell）, OS/容器⟩。

两条时间线：

| 轴 | 状态 | 回退语义 |
|---|---|---|
| **逻辑轴** $S_{\text{agent}}$ | graph state / messages / plan | **可截断**（restore 到 $c$） |
| **物理轴** $S_{\text{os}}$ | 进程、IPC、FS、net、调度、加载器配置 | **单调累积，不可截断** |

框架只对逻辑轴提供回退能力，二者在回退点之后必然分岔。

### 1.2 攻击者能力（分级，逐级增强）

| 级别 | 能力 | 用途 |
|---|---|---|
| **A0** | 无攻击者，仅正常任务 + 用户主动回退 | Gap 测量、正确性/可用性违规（缺失/重复/孤儿） |
| **A1** | 可在工作区放置内容（README、注释、测试输出、依赖名、commit message、`CLAUDE.md`） | 间接 prompt injection，机会主义 |
| **A2** | A1 + 能**可靠诱导回退**（种下副作用后让当前分支必败） | 把回退从"偶发"升级为攻击原语，确定性威胁模型 |

**关键**：主体实验在 A0/A1 下用 harness 强制注入 $\sigma$（合法性由 LangGraph time-travel 是官方 API 保证），A2 作为 realism 补强单独论证。

### 1.3 Bug 的形式定义（agent-specific，非普通 TOCTOU）

一次违规是**跨轴四元组** $\langle C, W, R, U\rangle$，满足 $C \prec W \prec R \prec U$ 且：

$$
\underbrace{U \rightsquigarrow r \;\wedge\; W \text{ writes } r}_{\text{(i) 解析依赖（内核轴）}}
\;\wedge\;
\underbrace{\mathrm{belief}(n)\ \text{est. at } C\ \text{survives } R}_{\text{(ii) 信念存活（agent 轴）}}
\;\wedge\;
\underbrace{\mathrm{obs}(W) \notin \mathrm{ctx}(R^{+})}_{\text{(iii) 证据擦除（跨轴）}}
$$

- $C$：agent 建立"名字 $n$ 可信"这一信念的校验点（stat/read/connect 成功/指纹校验）
- $W$：改变 $n \to \text{object}$ 映射的 bind-site
- $R$：回退（逻辑截断，物理保留）
- $U$：以 $n$ 为索引的 resolve-site（connect/open/execve/send secret）

**条件 (iii) 是 novelty 的锚点**：若 $W$ 的观测在回退后仍在 context 中，agent 原则上可判定 → 降级为 warning，作为对比组。这条件同时是"为什么必须做共轴时间线"的定义级答案——(ii)(iii) 只存在于 agent 轴，纯 eBPF 不可判定。

---

## 2. 输入空间分解

$$x = \langle\, T,\; E,\; I,\; \sigma \,\rangle$$

| 维度 | 结构 | 确定性 | 单次代价 | **在本方法中的角色** |
|---|---|---|---|---|
| $T$ 任务意图 | 无限、无结构 | 极低 | 高 | **固定 corpus（30–50），非 fuzz 主力** |
| $E$ 环境初态 | 文法化 | 完全 | 极低 | **fuzz 面（保证 bind/resolve 必然发生）** |
| $I$ 注入载荷+位置 | 可枚举 | 完全 | 极低 | fuzz 面（A1/A2） |
| $\sigma$ 回退调度 | **离散有限** | 完全 | 低（replay） | **核心 fuzz 面** |

**设计要点：$E$ 承担命中率，$T$ 退化为平凡任务。** 例：工作区是用 Unix socket 做 IPC 的小服务，`make test` 必然启动 listener，`Makefile`/`conftest.py`/`docker-compose.yml` 都引用该路径；任务只是"修一下这个失败的测试"。于是 bind/resolve 必然发生，不依赖运气，也不依赖 injection 成功。

---

## 3. 双轴 Trace 与解析溯源图（技术护城河）

### 3.1 内核轴：name resolution provenance graph

**不要用名字相等判据** $\mathrm{name}(W)\cap\mathrm{name}(U)\neq\emptyset$——它会漏掉全部高间接度案例（写 `.git/config[core.hooksPath]` → `execve(.githooks/pre-commit)` 交集为空）。改用解析依赖：

$$U \rightsquigarrow r \iff r \text{ 的内容/存在性/属性参与了 } U \text{ 对 } n \text{ 的解析}$$
$$\mathcal{H}_{\text{kernel}} = \{(W_i,U_j)\mid i<j,\ \exists r:\ W_i \text{ writes } r \wedge U_j \rightsquigarrow r\}$$

边的三个来源：

1. **内核解析链**：路径每个 dentry、symlink 目标、mount 点、`PATH` 中被跳过的前缀
   - ⚠️ **失败的 `open`（`ENOENT`）也是边**——它意味着"若此处有文件即会被选中"，是 shadowing 面的来源；只看成功路径会系统性漏报
2. **用户态解析链**：$U$ 之前该进程的读集（`openat`+`read`）与 env，给出 config→resource 边
3. **间接度** $d(U)$ = 解析链长度（从图上算出，不是人工标注），作为 depth 覆盖维度

**Bind-site / Resolve-site 分类：**

- $W$：`bind/listen/unlink/rename/link/symlink/mount/open(O_CREAT)/chmod/setxattr`、dotfile 写、`PATH` 变更、常驻进程 fork、cron/timer 写入
- $U$：`connect/openat/execve/stat/getaddrinfo`、配置读取

**规模控制**（务实修正："$n$ 很小"过于乐观，真实是 $10^5$ 量级）：
- 先按 provenance graph 连通分量分桶，仅桶内配对
- 早期剪枝：$U$ 的解析链**不含任何 agent 可写路径** → 直接丢弃（砍掉系统库访问，预期 $>99\%$）
- 剪枝比例本身是值得报的工程数字

### 3.2 Agent 轴：belief span

$$\mathcal{S} = \{(C,U)\mid \exists r:\ \text{check}(r) \prec \text{use}(r),\ \mathrm{rebindable}(r)\}$$

打点方式：每次 tool call 前后注入 marker（uprobe 或特征 write），用 `sched_process_fork` 维护进程树做**因果归属**而非时间窗归属（否则 detached 进程会被误归到后续 turn；daemon 代劳的操作需沿 socket peer 追溯）。

**资源身份必须用 `(dev, ino)` 而非 path**——path 只作为 agent 侧信念的 key，两者不匹配正是要报的告警。

### 3.3 候选集 = 两轴之交

$$\boxed{\mathcal{H}^{*} = \{(C,W,U)\mid (W,U)\in\mathcal{H}_{\text{kernel}} \wedge (C,U)\in\mathcal{S} \wedge \text{证据擦除可判定}\}}$$

---

## 4. Oracle：反事实"理想回滚"基线

这是**否决项**级别的正确性要求。

❌ **错误做法**：baseline = seed run（正向执行）。此时 $\mathcal{T}\ominus\mathcal{B}$ 混入了"分支 B 本来就不同于分支 A"的预期差异，所有回退都被判违规，假阳性爆炸。

✅ **正确做法**：

$$\mathcal{B} = \mathrm{Exec}\big(B \mid \text{logic}=c,\ \text{env}=\mathrm{snapshot}(c)\big),\qquad
\mathcal{T} = \mathrm{Exec}\big(B \mid \text{logic}=c,\ \text{env}=\text{as-is}\big)$$

$$\mathcal{T}\ominus\mathcal{B} = \text{纯粹由"OS 未回滚"引起的差异}\quad\text{（单变量对照）}$$

```
seed run:   ... C ... W ... │k                        ← 录制、建 ℋ*、存 memo + snapshot
                            │
                    ┌───────┴──── restore to c ───────┐
  𝒯 (test) :   logic=c, env=k  ──► B ──► id_𝒯(U)      │  真实系统行为
  ℬ (ideal):   logic=c, env=c  ──► B ──► id_ℬ(U)      │  反事实理想
                    └────────────── diff ⇒ Γ ─────────┘
```

**要点：**
- fs snapshot（overlayfs upper / btrfs subvolume）的首要用途**不是**保证迭代独立，而是**合成 oracle**
- $\mathcal{B}$ 同样走 memoized replay，否则 LLM 采样噪声污染对照
- $\mathcal{B}$ 按 checkpoint $c$ **缓存复用**（同一 $c$ 只算一次），成本摊薄
- `checkpoint_id` 与 fs snapshot 必须**原子配对**落盘，否则对照错位

### 4.1 判定格 $\Gamma$（纯函数，可完全自动化）

先定义身份函数：

| 资源类 | $\mathrm{id}(\cdot)$ |
|---|---|
| 文件/路径 | `(dev, ino)` + content hash |
| Unix socket | peer 的 `(pid, starttime, exe hash)`（`SO_PEERCRED` + procfs 交叉验证）|
| 进程 | `(pid, starttime)` |
| 监听端口 | listening sock inode + owner `(pid, starttime)` |
| 可执行命令 | 解析后绝对路径 + `(dev, ino)` + hash |
| shm / mq | `(name, ino, creator identity)` |

$$
\Gamma(U)=\begin{cases}
\textsf{consistent} & \mathrm{id}_\mathcal{T}=\mathrm{id}_\mathcal{B}\\
\textbf{REBOUND} & \text{both exist},\ \mathrm{id}_\mathcal{T}\neq\mathrm{id}_\mathcal{B}\quad\leftarrow\textit{motivating example}\\
\textsf{RESIDUE} & \exists\ \text{in}\ \mathcal{T},\ \nexists\ \text{in}\ \mathcal{B}\\
\textsf{MISSING} & \nexists\ \text{in}\ \mathcal{T},\ \exists\ \text{in}\ \mathcal{B}\\
\textsf{DUPLICATE} & \text{multiple id for one name / repeated non-idempotent effect}\\
\textsf{ORPHAN} & \text{live subject in }\mathcal{T}\text{ with no logical provenance}\\
\textsf{ESCAPED} & \text{effect crossed container boundary (irreversible)}
\end{cases}
$$

**严重性分级：** `REBOUND ∧ attacker-controlled(id_𝒯) ∧ dataflow(secret → U)` = **critical**。

### 4.2 早期信号：belief divergence

除了最终副作用，还可在 **divergence frontier** 上直接检测矛盾：agent 的自然语言断言（"listener 仍在运行"）与内核实际状态冲突。这个信号出现**早于**副作用落地，且更直接地论证"逻辑与状态不一致"。

---

## 5. Fuzz Loop（S2）

```python
# ---------- S1: 每个 ⟨T,E,I⟩ 付一次 LLM 成本 ----------
trace   = record(agent, T, E, I)          # 双轴
G       = provenance_graph(trace.kernel)  # 含失败 open
Hstar   = join(hazard_pairs(G), belief_spans(trace.agent))
memo    = trace.llm_memo                  # ctx_hash -> output
snaps   = trace.fs_snapshots              # checkpoint_id -> snapshot

# ---------- S2: 零/低 LLM 成本的 fuzz 循环 ----------
for (C, W, U) in prioritize(Hstar):                 # 能量分配
    c = checkpoint_before(C)
    B = cached_baseline(c) or replay(σ=(k,c), env=snaps[c], memo=memo)
    for k in range(idx(W)+1, idx(U)+1):             # 调度枚举：有限
        for dE, dI in mutate(E, I, grammar):
            T_run = replay(σ=(k,c), env=as_is, memo=memo)
            cls   = Γ(id_T(U), id_B(U))
            if cls != consistent:
                report(C, W, U, k, dE, dI, cls, severity(cls))
            restore(snaps[k])                       # 迭代独立
    feedback.update(realized_Hstar, bind_sites, div_classes, depth)
```

### 5.1 Memoized replay（替代"量化门槛"）

原先"不满足门槛就回炉重生成 query"有两个致命问题：浪费 LLM 调用，且**引入无法刻画的选择偏差**（只保留了 agent 恰好自发触发目标行为的有偏轨迹）。改为：

1. seed run 缓存 $(\text{ctx hash}) \to (\text{LLM output})$
2. replay 时命中缓存即**零成本确定性重放**
3. context 因回退后观测不同而变化 → **divergence frontier**，此后才真调 LLM（fast path / slow path）

⚠️ ctx hash 必须**规范化**：剔除时间戳、PID、绝对路径、随机 ID，否则 hash 永不命中。同时把非 LLM 侧的非确定性也钉住：时钟、随机数、DNS 响应（这些都归入 $E$）。

### 5.2 变异算子

**$\sigma$（主力）**：回退点位移 $k\pm1$ 扫描整个 $(\mathrm{idx}(W), \mathrm{idx}(U)]$ 窗口；变更目标 $c$；**回退次数 $+1$**（嵌套/重复回退：A→rollback→A′→rollback→B，几乎无框架测试过，预期产出率高）；跨 tool-call 边界 vs. 单条命令中途回退。

**$E$**：注入/移除预置资源；**提升间接度**（直接路径 → 经 config/symlink/env）；保名换类型（file→FIFO→socket→symlink→dir）；权限变更；预置 cron/timer。

**$I$**：投放位置枚举；载荷类型 = "种副作用"型 vs. "**诱导失败**"型；**分裂载荷**（A 分支种、B 分支引爆）。

**$T$**（低频）：仅保意图改写，测 robustness。

### 5.3 搜索策略

- **bounded exhaustive 优先**（借 B³/CrashMonkey）：$|\sigma|\le 2$ 时穷举全部 $(k,c)$，给出"此边界内无遗漏"的强 claim
- 再转 **coverage-guided**，AFL 式能量分配：偏向未探索 hazard pair 多、间接度高、资源类稀有的 seed
- **最小回退距离**：二分搜索最靠后的仍触发违规的 $k$，刻画漏洞窗口宽度

### 5.4 覆盖度定义（"更广 / 更深"的量化）

1. **Bind-site coverage**（广）：`(namespace × syscall class × resource class)` 格子命中率
2. **Realized-$\mathcal{H}^*$ coverage**（核心反馈信号）：$\mathcal{H}^*$ 中真正实现 $C\to W\to R\to U$ 且 $U$ 解析到 $W$ 对象的比例；出现新 realized pair → 该 $\langle T,E,I\rangle$ 入 corpus
3. **Divergence-class coverage**：$\Gamma$ 的 7 类 × 资源类
4. **Depth**：回退次数、间接度 $d(U)$、frontier 之后步数

---

## 6. Realism 论证（三层，全做）

| 层 | 内容 | 作用 |
|---|---|---|
| **L-a** in-spec | LangGraph `get_state_history` + 从历史 `checkpoint_id` 重新 invoke 是官方 time-travel feature；注入的是设计上支持的操作 | 最易写，先做 |
| **L-b** 自然触发率 | 统计正常任务中 agent 自发回退/重试的比例 | 说明非人为构造 |
| **L-c** rollback-triggering primitive | $I$ = 种副作用 + 保证当前分支必败（无解错误、误导性报错、假 lint 失败）→ 证明存在可靠触发原语 | 威胁模型从机会主义 **升级为确定性**，决定论文天花板 |

---

## 7. 不依赖攻击的 Motivating Measurement：Gap

$$\mathrm{Gap} = 1 - \frac{|\{\text{OS effects with any trace in checkpoint}\}|}{|\{\text{OS effects}\}|}$$

LangGraph checkpointer 序列化 graph state（messages / channels），tool 产生的 OS 变更**完全在序列化边界之外**，预期 $\mathrm{Gap}\to 1.0$。

**产出**：一个数字 + 一张图（横轴 tool-call 序号，两条带：checkpoint 记录了什么 vs. 内核实际发生了什么）= 论文第 2 节全部内容。

**为什么它是最强 claim**：不依赖任何攻击、injection、LLM 行为——纯测量，不可质疑。且它直接回答"这是否只是模型不够聪明"：$\mathrm{Gap}\to1$ 说明**框架在设计上就没把必要信息交给模型**，是信息论层面的不可判定，与模型能力无关。

**配套 ablation（成本极低、杀伤力极大）**：把分支 A 的操作记录**手工塞回** context，若 agent 随即重新校验 → 干净证明失败根因是框架丢弃信息，而非模型能力不足。

---

## 8. 状态类型分类法（按可逆性组织，作为 $E$ 的文法）

按可逆性而非子系统分类，因为可逆性才是与回退直接相关的维度：

| 类别 | 例子 | 相对回退的性质 |
|---|---|---|
| 幂等可逆 | 普通文件内容 | 低风险，对照组 |
| **名字可再绑定** | unix socket、FIFO、symlink、`PATH` 可执行文件、`~/.ssh/config`、`.git/hooks` | **核心风险面（REBOUND）** |
| 带生命周期的活体 | 后台进程、daemon、持 fd 子进程、detached | ORPHAN 来源 |
| 不可撤销外部效应 | 已发网络请求、已 push commit、已轮换凭据 | ESCAPED，语义上不可能回退 |
| 隐式加载器/配置 | `LD_PRELOAD`、shell rc、`git config`、`pip.conf`、env、alias | 影响后续所有 exec，高间接度 |
| **调度类** | cron / systemd timer / `at` | **跨回退点延迟引爆，PoC 最漂亮** |
| 内核/命名空间 | mount、iptables、`/etc/hosts`、netns | 影响可见性与可达性 |

---

## 9. 实施路线与优先级

优先级判断的原则：**先消除否决项，再追加分项。**

| 阶段 | 内容 | 周期 | 判据 |
|---|---|---|---|
| **P0** | **Gap 测量**（第 7 节） | ~1 周 | 出图。若 Gap 意外偏低 → 立即调整叙事，而非在 fuzzer 上投三个月才发现前提有问题 |
| **P1** | 双轴 trace + provenance graph + 快照 baseline 正确性 | 3–4 周 | 手工构造的 motivating example 能被 $\Gamma$ 自动判为 REBOUND/critical；假阳性可控 |
| **P2** | memoized replay + bounded exhaustive $\sigma$ 枚举 | 3–4 周 | 单次迭代降至亚秒级；realized-$\mathcal{H}^*$ 覆盖曲线可画出 |
| **P3** | $E$/$I$ 文法变异 + coverage-guided 能量分配 | 3–4 周 | 出现 seed run 中不存在的新 realized pair（证明 fuzz 有增益，而非仅重放） |
| **P4** | L-c rollback-triggering primitive | 2 周 | 存在可靠触发原语 → 威胁模型升级 |
| **P5** | 跨框架扩展 MAF / Claude Code + 披露 | 3–4 周 | 框架对比表 + 厂商确认/CVE |
| **P6** | Defense discussion + artifact 发布 | 2 周 | belief-span benchmark 带 ground truth |

**明确排除（避免稀释核心故事）：**
- **PID 复用**降为 discussion：真 bug，但需精确控制 PID 分配，工程成本高、可复现性差，且与"回退"关联最弱（属独立的身份混淆问题）
- **上下文压缩（auto-compact）**列为 future work 并在 discussion 中点明：它比显式 rewind 更有杀伤力（框架为省 token 自动触发、用户无感、无"用户自己要求的"辩护空间），但当前聚焦显式 checkpoint/restore 以保证 $\sigma$ 可枚举

---

## 10. 评估设计

### 10.1 广度 + 深度组合

| 类型 | 违规类 | 呈现方式 |
|---|---|---|
| **规模化统计** | DUPLICATE / MISSING / ORPHAN | "$N$ 个任务中 $M\%$ 出现非幂等重复执行"——易大量复现，A0 即可 |
| **深度 case study** | REBOUND（critical） | 完整 PoC：secret 流入攻击者对象；配 minimal rollback distance |
| **时序 PoC** | 调度类 | 分支 A 埋下的 cron 在分支 B 引爆 |

### 10.2 无直接 baseline 时的对照构造

1. **理想回滚上界**：$\mathcal{B}$ 本身即"完美防御"的性能/正确性参考点
2. **消融对照**：只用内核轴（退化为普通 TOCTOU 检测）vs. 双轴 → 量化条件 (ii)(iii) 过滤掉多少非 agent-specific 噪声
3. **随机调度对照**：随机采样 $\sigma$ vs. $\mathcal{H}^*$ 制导 → 量化 Stage-1 推断的增益（这是 Razzer 式论证的标准做法）
4. **NL-only 对照**：你原先的 pipeline（纯 fuzz $T$）作为 baseline → 直接展示新架构在单位时间内发现违规数的量级提升。**你走过的弯路正好是最有说服力的 baseline，务必保留旧实现的数据**

### 10.3 关键报告数字

- $\mathrm{Gap}$（接近 1.0）
- provenance graph 剪枝率（预期 $>99\%$）
- 单次 fuzz 迭代耗时（seed run vs. replay，预期 2–3 个数量级差）
- realized-$\mathcal{H}^*$ 覆盖曲线（vs. 随机调度）
- 假阳性率（正确 baseline vs. 错误 baseline，用来论证第 4 节的必要性）
- 每框架 × 每违规类的矩阵

---

## 11. 防御方向（Discussion，让工作闭环）

按彻底性递增：

1. **回退时的世界状态 diff 回注** ⭐ **最该 claim 的一条**：不真回滚 OS，而在 restore 时把"分支 A 造成的、且与当前信念相关的 OS 变更"作为系统消息注入 context。这恰好是你的 provenance graph 已经算出的东西——**检测器与缓解器共用同一条共轴时间线**，SyncFuzz 从 fuzzer 升格为基础设施
2. **Belief invalidation**：为每个 belief span 记录 $\mathrm{id}(r)$，$U$ 之前重新校验，不一致则中断确认
3. **能力/句柄化而非路径化**：check 时即持有 fd/capability 而非路径字符串，机制上消除再绑定（最彻底，最"系统安全"）
4. **不可逆操作的 rollback barrier**：识别 ESCAPED 类别，回退时强制提示或禁止跨越

---

## 12. 论文骨架映射

| 节 | 内容 | 对应本文档 |
|---|---|---|
| §1 Intro | 双时间线分岔；**两个类比**：crash consistency（穷举 crash point ↔ 穷举 rollback point，指导 oracle 构造）+ Razzer/SKI（两阶段：先推断候选对，再 fuzz 调度，指导输入缩减） | §0, §9 |
| §2 Motivation | **Gap 测量 + 图**；motivating example（`/tmp/keyd.sock`）；context 回注 ablation | §7 |
| §3 Threat model | A0/A1/A2 分级；四元组定义 $\langle C,W,R,U\rangle$ 及三谓词 | §1 |
| §4 Design | 输入四元组分解；provenance graph；$\mathcal{H}^*$ 两轴取交 | §2, §3 |
| §5 Oracle & Taxonomy | 反事实理想基线；判定格 $\Gamma$；严重性 | §4 |
| §6 Impl | eBPF hook 清单、因果归属、memoized replay、LangGraph 集成 | §3.2, §5.1 |
| §7 Eval | 覆盖曲线、四组对照、框架矩阵、case study | §10 |
| §8 Discussion | 防御四条；auto-compact 为 future work | §11, §9 |

---

## 13. 逻辑自洽性自查

这套方法论的每个组件都在回答一个特定的质疑，闭环如下：

| 潜在质疑 | 本方法的硬回答 |
|---|---|
| "这只是普通 TOCTOU" | 条件 (iii) 证据擦除 + (ii) 信念存活，纯内核轴不可判定；消融实验量化过滤比例 |
| "这只是模型不够聪明" | $\mathrm{Gap}\to1$（框架未交付信息）+ 回注 ablation（给了信息就会校验） |
| "回退是你强加的，不现实" | L-a 官方 API / L-b 自发率 / L-c 可靠触发原语 |
| "你这不叫 fuzzing" | fuzz 面是 $\sigma$（离散有限可枚举）+ 4 维覆盖度 + 能量分配 + bounded exhaustive 完备性 claim |
| "残留可能是期望行为" | 反事实理想基线，单变量对照，不是"有残留就报" |
| "LLM 不确定性污染 diff" | memoized replay 确定性重放 + $\mathcal{B}$ 同样 replay + ctx hash 规范化 |
| "命中率靠运气" | $E$ 承担命中保证（`make test` 必起 socket），$T$ 平凡化 |
| "输入太大不可行" | 连通分量分桶 + agent-可写路径剪枝（$>99\%$）+ $|\sigma|\le2$ 边界 |
| "没有 baseline 可比" | 四组对照，含你自己的旧 NL-only pipeline |

---

## 立即行动

**本周只做一件事：P0 的 Gap 测量。**

最小步骤：LangGraph + `SqliteSaver` 跑 5–10 个含 shell tool 的普通任务 → eBPF 记录全部 OS effects → 逐个检查是否在任一 checkpoint 的序列化内容中留下痕迹 → 算 Gap，画那张两条带的图。

这一步的价值不对称：结果无论如何都能用。若 Gap ≈ 1（预期），你拿到了整篇论文最不可反驳的一张图；若 Gap 意外偏低（某 checkpointer 记录了 tool 副作用摘要），你在第一周而非第三个月就知道要调整叙事。

需要我展开 P0 的具体实现（eBPF hook 清单 + LangGraph checkpoint 内容提取 + effect 归属算法），或者把 §1.3 的定义和 §4.1 的判定格写成可直接进 paper 的 LaTeX 定义环境，随时说。

---

# User:
我之前做的尝试已经堆成一个屎山了。所以我想重新开始，从零搭一个框架。因此，请你帮我写一个 PRD 文档，文档里面要具体阐述我们的框架要有几个部分，每个部分负责的工作，预期的效果等，防止 Coding Agent 过度发挥导致项目跑偏。

# LLM:
# SyncFuzz PRD
**Rollback-Schedule Fuzzing for Agent Belief–State Divergence**

| 项 | 值 |
|---|---|
| 文档版本 | v1.0（冻结基线） |
| 目标读者 | 人类维护者 + Coding Agent |
| 首个交付目标 | P0 Gap 测量出图 |
| 主语言 | Python 3.11+ / C（eBPF CO-RE） |
| 目标平台 | Linux x86_64, kernel ≥ 5.15, cgroup v2 unified hierarchy、BTF 可用 |

---

## 0. 本文档的效力与 Coding Agent 行为守则

> **⚠️ 本节对 Coding Agent 具有最高优先级，高于任何后续章节中的实现建议。**

### 0.1 硬约束

1. **契约先行**：`§4 数据契约` 中的 schema 是**冻结的**。任何字段增删改必须先写一份 `docs/adr/NNN-*.md`（Architecture Decision Record）并等待人类确认，不得直接改代码。
2. **模块边界不可越界**：模块之间**只能通过 §4 定义的 artifact 文件通信**，禁止直接 import 对方内部模块、禁止共享内存对象、禁止跨模块传递 ORM 实体。
3. **禁止顺手扩展**：不在当前里程碑范围内的功能，即使"只需 5 行"，也不许写。发现需要，就在 `docs/BACKLOG.md` 追加一行，然后停止。
4. **禁止自作主张换技术栈**：§7 已钉死依赖。要换需 ADR。
5. **禁止把 LLM 放进判定路径**：oracle（M6）必须是**纯函数**。任何"让模型判断这算不算违规"的实现一律拒绝。
6. **每个模块必须可独立 CLI 运行**，输入是文件、输出是文件，且有 golden fixture 回归测试。
7. **门禁**：未通过上一里程碑的验收标准（§6），不得开始下一里程碑的代码。
8. **不确定就停**：遇到 PRD 未覆盖的设计选择，写进 `docs/OPEN_QUESTIONS.md` 并向人类提问，**不要猜**。

### 0.2 反跑偏检查清单（每次提交前自查）

- [ ] 我改的文件是否都属于当前里程碑的模块？
- [ ] 我是否新增了 §4 之外的数据结构作为**跨模块**接口？
- [ ] 我是否在采集层（M1/M2）写了分析逻辑？（禁止）
- [ ] 我是否在 oracle（M6）里写了阈值、启发式、"大概"、`if 'sock' in path`？（禁止）
- [ ] 我是否引入了新的第三方依赖？（需 ADR）
- [ ] 我是否让某个模块直接调用了另一个模块的 Python 函数？（禁止）

---

## 1. 项目目标与非目标

### 1.1 一句话目标

构建一个可复现的实验框架，**在 LangGraph agent 上系统性枚举回退调度（rollback schedule），用"理想回滚"反事实基线作为单变量对照 oracle，自动判定并分类 agent 逻辑状态与 OS 状态的失同步违规。**

### 1.2 必须达成的四个能力

| # | 能力 | 可验证的产出 |
|---|---|---|
| G1 | **双轴共轴时间线**：内核 syscall 事件与 agent tool-call/checkpoint 事件在同一时钟域对齐，并有因果归属 | 一条时间线 artifact，任一 syscall 可追溯到 turn/tool_call |
| G2 | **Gap 测量**：量化 checkpoint 序列化边界与 OS 副作用边界的错配 | 单一数字 + 双带状图 |
| G3 | **确定性重放**：同一 seed trace 上以亚秒级成本探索数十~上百个 $\sigma$ | replay 相对 seed run 的耗时/成本比 ≥ 100× |
| G4 | **自动违规判定**：纯函数判定格 $\Gamma$ 输出 7 类违规 + 严重性 | 违规报告 JSON + 可复现 PoC 目录 |

### 1.3 明确的非目标（写进代码注释，防止 Agent 发挥）

| 非目标 | 理由 |
|---|---|
| ❌ 通用 agent 安全扫描器 | 只做 rollback 一个 bug class |
| ❌ 支持 Windows / macOS | eBPF 依赖 |
| ❌ 首版支持 Claude Code / MAF | 只做 LangGraph；跨框架在 M9 |
| ❌ fuzz 自然语言 prompt | $T$ 是固定 corpus，不是 fuzz 面 |
| ❌ LLM 参与违规判定 | oracle 必须纯函数 |
| ❌ 在线防御 / runtime patch | 防御只写 discussion |
| ❌ PID 复用攻击 | 降为 discussion，见 §9 排除项 |
| ❌ 上下文压缩（auto-compact） | future work |
| ❌ Web UI / dashboard | 出图用脚本生成 PNG/PDF 即可 |
| ❌ 分布式 / K8s 编排 | 单机足够 |
| ❌ 性能优化（除 §6 明确门槛） | 先对，再快 |

---

## 2. 系统总览

### 2.1 模块图

```
                    ┌──────────────────────────────────────┐
                    │  M0  syncfuzz-core                   │
                    │  schema / 时钟 / ID / artifact IO     │
                    │  (所有模块唯一可共享的依赖)            │
                    └──────────────────────────────────────┘
                                     ▲
        ┌────────────────┬───────────┴────────┬──────────────────┐
        │                │                    │                  │
┌───────┴──────┐ ┌───────┴───────┐  ┌─────────┴────────┐ ┌──────┴───────┐
│ M1 ktrace    │ │ M2 atrace     │  │ M3 envkit        │ │ M4 corpus    │
│ eBPF 内核轴  │ │ agent 轴打点  │  │ 快照/沙箱/去噪   │ │ T,E,I 语料   │
└───────┬──────┘ └───────┬───────┘  └─────────┬────────┘ └──────┬───────┘
        │                │                    │                  │
        └────────┬───────┘                    │                  │
                 ▼                            │                  │
        ┌────────────────────┐                 │                  │
        │ M5 timeline        │◄────────────────┴──────────────────┘
        │ 归并/归属/溯源图    │
        │ → ℋ_kernel, 𝒮, ℋ* │
        └─────────┬──────────┘
                  │
      ┌───────────┼────────────────────────┐
      ▼           ▼                        ▼
┌───────────┐ ┌──────────────┐   ┌──────────────────┐
│ M6 oracle │ │ M7 replay    │   │ M8 gapmeter      │
│ Γ 判定格   │ │ memo + 调度  │   │ Gap 测量(独立!)  │
│ (纯函数)  │ │ 注入 + 基线  │   └──────────────────┘
└─────┬─────┘ └──────┬───────┘
      └───────┬──────┘
              ▼
     ┌──────────────────┐        ┌──────────────────┐
     │ M9 fuzzer        │───────►│ M10 report       │
     │ 覆盖/能量/调度    │        │ 图表/PoC/统计    │
     └──────────────────┘        └──────────────────┘
```

### 2.2 依赖规则（**Agent 必须遵守**）

- 所有模块**只能** import `M0`。
- `M5` 读 M1/M2/M3 的 artifact **文件**，不 import 它们。
- `M9` 通过**子进程或 artifact** 驱动 M7，不 import M7 内部类。
- `M8` **完全独立**，不依赖 M5~M9。这是故意的：Gap 测量必须能在其他模块都没写完时先跑出结果。

违反依赖规则的 PR 直接拒绝。用 `import-linter` 在 CI 强制。

---

## 3. 模块详细规格

### M0 `syncfuzz-core` — 契约与基础设施

**职责**：定义所有跨模块数据结构、统一时钟、统一 ID、artifact 读写、run 目录布局。**不含任何业务逻辑。**

**必须提供**：

| API | 说明 |
|---|---|
| `Event`, `KEvent`, `AEvent`, `Timeline`, `HazardPair`, `BeliefSpan`, `Violation`, `RunManifest` | Pydantic v2 模型，见 §4 |
| `mono_ns() -> int` | 唯一时钟源：`CLOCK_MONOTONIC`（与 eBPF `bpf_ktime_get_ns()` 同域）|
| `ResourceId` | 身份元组封装，见 §4.4 |
| `open_run(path) -> RunDir` / `RunDir.write(name, obj)` / `RunDir.read(name, T)` | artifact IO，原始 UTF-8 JSON/JSONL（不压缩） |
| `sf_assert(cond, msg)` | 违反契约时**立即崩溃**，不降级不兜底 |

**明确禁止**：任何 `parse_*`、`analyze_*`、`detect_*` 函数出现在 M0。

**验收**：`pytest tests/m0` 全绿；schema 有 JSON Schema 导出文件 `schemas/*.json` 并纳入 git（schema 漂移在 CI 可见）。

---

### M1 `ktrace` — 内核轴采集

**职责**：用 eBPF 采集与"名字解析"和"名字绑定"相关的 syscall 事件，输出 `kevents.jsonl`。**只采集，不分析。**

**必须 hook 的事件（v1 冻结清单）**：

| 类别 | 事件 |
|---|---|
| **Bind-site** | `openat/openat2`(含 `O_CREAT`)、`unlinkat`、`renameat2`、`linkat`、`symlinkat`、`mkdirat`、`bind`、`listen`、`fchmodat`、`fsetxattr`、`mount`、`write`(仅命中 watchlist 路径) |
| **Resolve-site** | `openat/openat2`(全部，**含失败)**、`execve/execveat`、`connect`、`newfstatat`、`readlinkat`、`socket` |
| **进程/因果** | `sched_process_fork`、`sched_process_exec`、`sched_process_exit` |
| **标记** | uprobe on `libsyncfuzz_marker.so:sf_mark`（M2 注入用） |

**关键要求（Agent 极易做错，重点强调）**：

1. **失败的 `openat` 必须采集**，`ret < 0` 时记录 `errno`。`ENOENT` 是 shadowing 攻击面的唯一来源，丢了就系统性漏报。
2. 必须在 **sys_exit** 上取返回值，同时在 **sys_enter** 上取参数，用 `(tgid, tid, syscall_nr)` 关联。
3. P0 tracepoint collector 必须记录 user path 与 `dirfd`，且**不得**在 eBPF 中做路径拼接。当前支持矩阵不允许在该 program type 调用 `bpf_d_path`，故 `cwd` 固定记录为不可用（空值）；不得以事后读取 `/proc/<pid>/cwd` 伪造 syscall 时刻的 cwd。P1 的 M5 若需相对路径重建，必须重新决定 feature gate 或 hook 机制。
4. 记录 `(dev, ino)` 于成功的 open/stat 上——这是 `ResourceId` 的基础。
5. 用 **ring buffer**（`BPF_MAP_TYPE_RINGBUF`），丢事件必须计数并写入 manifest 的 `dropped_events`；**丢事件 > 0 时 M5 必须拒绝分析并报错**，不许静默继续。
6. 过滤：只跟踪目标 cgroup，不要全局跟踪。P0 仅支持 cgroup v2 unified hierarchy；`--cgroup-id` 是目标 cgroup v2 目录的 inode ID，非 v2 环境必须 fail-fast。
7. 生产 loader 是直接链接 `libbpf`（≥ 1.7.0）的 C CLI；构建时用 `bpftool gen skeleton` 生成临时 skeleton header，不提交生成文件，也不引入 Python libbpf binding 或 bcc 生产依赖。
8. orchestrator 在启动 M1 前创建完整 `manifest.json`；M1 只在 `manifest.json.lock` 的 advisory `flock` 内无损、原子地更新既有 `dropped_events` number token。该锁文件是瞬时协调状态，不是 artifact，不能作为模块间数据输入或输出。

**接口**：
```
syncfuzz-ktrace --cgroup-id <cgroup-v2-inode> --out <run>/kevents.jsonl --duration <s> \
  --watch-path <host-absolute-path>... \
  --marker-so <host-absolute-path-to-libsyncfuzz_marker.so> \
  --manifest <run>/manifest.json
```

`write` 只对以上 host absolute watch paths 命中的文件采集；marker shared object 必须是 host 上可供 uprobe attach 的绝对路径，不能使用容器内路径。

**验收**：在已有、隔离的 cgroup v2 Linux 容器中运行合成测试程序（无需启动 Agent），做 20 种操作（含 3 个必然失败的 open）；M1 全部捕获、字段完整、`dropped_events == 0`。这只验证 M1 采集边界与事件完整性，不替代 P0 的 LangGraph Agent 集成验收。

---

### M2 `atrace` — Agent 轴采集

**职责**：在 LangGraph 执行过程中记录 agent 侧事件，输出 `aevents.jsonl` + `memo.jsonl` + `ckpt_index.jsonl`。

**必须记录**：

| 事件 | 内容 |
|---|---|
| `turn_start/end` | turn id、时间戳 |
| `tool_call_start/end` | tool 名、参数（规范化后）、返回、**发出的 `sf_mark`** |
| `checkpoint_written` | `thread_id`、`checkpoint_id`、序列化字节大小、**序列化内容的键路径全集**（供 M8 用）|
| `llm_call` | `ctx_hash`（见下）、`output`、token 数、耗时 |
| `assertion_candidate` | agent 输出中形如"X 仍在运行/已存在/未变"的句子（**仅抽取记录，不判定**）|

**`sf_mark` 机制**：`libsyncfuzz_marker.so` 导出 `void sf_mark(const char *json_payload)`。M2 在每个 tool call 前后以 NUL-terminated UTF-8 JSON payload 调用它；M1 只将 payload 原样写入 `KEvent(site="mark").args_raw`，不解析、不推断 turn/tool/phase，也不在读取失败时编造 marker。M5 获授权后才严格解析其语义并作为**边界锚点**使用。

**`ctx_hash` 规范化（Agent 极易做错）**：
计算前必须剔除：绝对路径前缀（工作区根替换为 `$WS`）、时间戳、PID、UUID/随机 ID、耗时数字、内存地址、临时目录名。规范化规则集中在 `m2/canon.py`，**必须有单元测试证明同一逻辑步骤在两次运行中 hash 相同**。这是 M7 能否零成本 replay 的前提。

**明确禁止**：M2 不得判断 assertion 是否为"错误信念"——那是 M6 的事。

**验收**：跑一个 3-tool-call 的 graph，`aevents.jsonl` 完整；同一任务跑两次，`ctx_hash` 序列完全一致（证明规范化有效）。

---

### M3 `envkit` — 环境、快照与去噪

**职责**：提供可复现的执行环境与**checkpoint 对齐的文件系统快照**。

**必须提供**：

1. **容器沙箱**：rootless podman/docker，固定 image digest，独立 netns/pidns，暴露 cgroup id 给 M1。
2. **快照/恢复**：`snapshot(tag) -> SnapshotId`、`restore(SnapshotId)`。首选 **overlayfs upper 层归档**（简单、可移植）；btrfs subvolume 作为可选后端。
3. **原子配对**（关键）：`checkpoint_id` 与 `SnapshotId` 必须写入 `ckpt_snapshot_map.jsonl`，且**在 checkpoint 落盘与快照之间必须暂停 agent 执行**（LangGraph 的 super-step 边界是天然暂停点）。配对错位会让 §5 的 oracle 全盘失效。
4. **非确定性钉死**：固定时区/`TZ`、`PYTHONHASHSEED`、假时钟（`libfaketime` 或注入 tool）、DNS 走本地固定 responder、`/dev/urandom` 种子固定。
5. **环境播种器**：读 M4 的 `E` spec，生成工作区（预置 socket 服务、dotfiles、`PATH` shadowing 机会、symlink farm、cron/timer、`CLAUDE.md`）。

**明确禁止**：M3 不读 trace、不做分析。它是纯粹的"环境提供者 + 时间机器"。

**验收**：同一 $\langle T,E\rangle$ 连续跑 3 次，`kevents` 的**相关子集**（剔除已知噪声白名单）逐事件一致；`restore(snapshot)` 后 `find / -newer` 差异为空。

---

### M4 `corpus` — $T$ / $E$ / $I$ 语料

**职责**：定义并生成三个**非 fuzz-loop** 维度的语料。这是 offline 一次性产出，之后**冻结**。

**$T$（任务集，30–50 个，固定）**：
- YAML 描述：`task_id`、`prompt`、`success_criteria`、`expected_tools`
- **设计原则：任务必须平凡。** 命中率靠 $E$ 保证，不靠 prompt 诱导。典型："修一下这个失败的测试"、"给这个服务加个 health check"、"把日志级别改成 DEBUG 并验证"。
- **禁止**在 prompt 里写"请 bind 一个 socket"这类诱导语句。

**$E$（环境语法，可枚举）**：
```yaml
env_id: e_socket_ipc_indirect_2
resources:
  - kind: unix_socket_service
    path: /run/app/ipc.sock
    referenced_by: [Makefile, tests/conftest.py]
    indirection:                    # 间接度 = 2
      - type: env_var
        name: APP_SOCK
        source: .env
      - type: symlink
        from: /run/app/ipc.sock
        to: /var/lib/app/real.sock
  - kind: dotfile
    path: .git/config
    keys: [core.hooksPath]
  - kind: path_shadow_slot
    dir: ./bin
    before: /usr/bin
```
**可变异维度（M9 用）**：资源种类、间接度 $d$、引用方式、资源类型保名替换（file→FIFO→socket→symlink→dir）、权限、预置 cron。

**$I$（注入载荷，A1/A2）**：
```yaml
# （接上）$I$ 注入载荷 spec
inject_id: i_split_hookpath
placement: readme            # readme|code_comment|test_output|dep_name|commit_msg|error_msg|claude_md
kind: split                  # seed_effect | induce_failure | split
phase_a: "在分支 A 中诱导 agent 写入 .git/config[core.hooksPath]"
phase_b: "在分支 B 中诱导 agent 执行 git commit（引爆）"
expected_bind_site: ".git/config"
expected_resolve_site: "execve(.githooks/pre-commit)"
```

**三类载荷的作用分工（不可混淆）**：

| kind | 作用 | 支撑论文哪一节 |
|---|---|---|
| `seed_effect` | 在分支 A 种下副作用 | §Design 主体 |
| `induce_failure` | 让当前分支必然失败，从而**可靠触发回退** | L-c realism 论证 |
| `split` | A 分支种、B 分支引爆 | 深度 case study |

**验收**：`syncfuzz-corpus validate` 检查全部 spec 可被 M3 播种成功；每个 $E$ 至少有一条**声明的**预期 bind/resolve 对（供 M5 做 sanity check，**但不作为判定依据**）。

---

### M5 `timeline` — 归并、因果归属、溯源图

**职责**：把 M1/M2/M3 的 artifact 合并成单一共轴时间线，构建 name-resolution provenance graph，输出候选集 $\mathcal{H}^*$。**这是本框架的技术护城河，也是最容易写歪的模块。**

#### M5.1 归并与因果归属

输入 `kevents.jsonl` + `aevents.jsonl` → 输出 `timeline.jsonl`。

**归属规则（严格按此顺序，禁止用时间窗兜底）**：

1. 用 `sched_process_fork` 构建**进程树**，根为 tool call 的直接子进程。
2. 每个 `KEvent` 通过 `(tgid, starttime)` 沿进程树上溯到某个 tool call → `attributed_to = tool_call_id`。
3. `sf_mark` 提供边界校验：若某 kevent 落在 mark 区间外但进程树归属到该 tool call（detached 进程、`nohup`、daemon），**以进程树为准**，并置 `late_effect = true`。
4. 若操作由**已存在的 daemon** 代劳（agent 通过 socket 请求 daemon 做事），沿 socket peer（`SO_PEERCRED` + `unix_diag`）追溯，置 `via_proxy = <daemon identity>`。
5. **无法归属**的事件置 `attributed_to = null`, `orphan = true`。**禁止猜测性归属**；orphan 比例必须写入 manifest，> 5% 时 M5 报警。

> ⚠️ **Agent 注意**：绝对不要写 `if abs(kevent.ts - toolcall.ts) < WINDOW`。时间窗归属会把 detached 进程误归到后续 turn，直接毁掉 §5 的 oracle 正确性。

#### M5.2 Name-resolution provenance graph

对每个 resolve-site $U$，重建其**解析链** $\mathrm{chain}(U)$：

| 来源 | 内容 |
|---|---|
| 路径解析 | 每级 dentry、每个 symlink 目标、mount 点 |
| **失败探测** | `PATH`/搜索序列中被跳过的前缀（来自 `ENOENT` 事件）|
| 用户态配置 | $U$ 所在进程在 $U$ 之前的读集（`openat`+`read` 成功的文件）∩ 可写路径 |
| 环境 | `execve` 的 envp 中被引用的变量及其来源文件 |

定义边：

$$U \rightsquigarrow r \iff r \in \mathrm{chain}(U)$$
$$\mathcal{H}_{\text{kernel}} = \{(W_i,U_j) \mid i<j,\ \exists r:\ \mathrm{writes}(W_i, r) \wedge U_j \rightsquigarrow r\}$$

$d(U) = |\mathrm{chain}(U)|$ 为**计算得出**的间接度，禁止人工标注。

> ⚠️ **禁止用名字相等做判据**。`name(W) ∩ name(U) ≠ ∅` 会漏掉全部高间接度案例（写 `.git/config` → `execve(.githooks/pre-commit)`，名字交集为空）。若发现代码里出现 `if w.path == u.path`，视为严重缺陷。

**规模控制（按此顺序，必须实现全部三级）**：

1. **可写性剪枝**：$\mathrm{chain}(U)$ 不含任何 agent-可写路径 → 丢弃 $U$。（预期砍掉 >99%，主要是 `/usr/lib` 系统库）
2. **连通分量分桶**：只在同一连通分量内配对 $(W,U)$。
3. **上限保护**：单个分量内 pair 数 > `MAX_PAIRS_PER_COMPONENT`（默认 5000）时截断并记录，不许 OOM。

剪枝前后的数量必须写入 manifest（`§10` 要报这个数字）。

#### M5.3 Belief span

$$\mathcal{S} = \{(C,U) \mid \exists r:\ \mathrm{check}(r) \prec \mathrm{use}(r),\ \mathrm{rebindable}(r)\}$$

- $C$ = 建立信念的校验点：成功的 `newfstatat` / `openat` / `connect` / 指纹校验读取
- `rebindable(r)` 由 §8 状态分类法查表决定（`m5/rebindable.yaml`，声明式，非硬编码 if 链）
- $C$ 与 $U$ 必须归属到**不同的 tool call**（否则回退点无处插入）

#### M5.4 输出 $\mathcal{H}^*$

$$\mathcal{H}^{*} = \{(C,W,U) \mid (W,U)\in\mathcal{H}_{\text{kernel}} \wedge (C,U)\in\mathcal{S} \wedge \mathrm{obs}(W)\notin \mathrm{ctx}(R^+)\}$$

第三个条件（证据擦除）由 M5 计算：检查 $W$ 的可观测痕迹（tool 返回文本、stdout/stderr）是否会出现在回退后的 context 中。**不可擦除的 pair 也要输出**，但标记 `evidence_erased = false`，作为对比组（论文要用它论证 novelty）。

**验收**：
- 手工构造的 3 个 golden case（直接路径 $d{=}1$、symlink $d{=}2$、`git config`→hook $d{=}3$）全部出现在 $\mathcal{H}^*$ 中
- 3 个 negative case（只读系统库、不可再绑定资源、$C$ 与 $U$ 同 tool call）全部被正确排除
- 剪枝率 > 95%，orphan 率 < 5%

---

### M6 `oracle` — 判定格 $\Gamma$（**纯函数**）

**职责**：给定 $(\mathcal{T}, \mathcal{B})$ 两条 timeline 与目标 $U$，输出违规分类与严重性。

**签名（冻结）**：
```python
def gamma(u: ResolveSite,
          t: Timeline,      # 真实系统（env 未回滚）
          b: Timeline       # 反事实理想回滚
          ) -> Verdict:     # 纯函数：无 IO、无网络、无 LLM、无随机、无时钟
```

**判定表**：

| 条件 | 分类 |
|---|---|
| $\mathrm{id}_\mathcal{T}(U) = \mathrm{id}_\mathcal{B}(U)$ | `CONSISTENT` |
| 两者皆存在且 $\mathrm{id}$ 不等 | **`REBOUND`** |
| 仅 $\mathcal{T}$ 存在 | `RESIDUE` |
| 仅 $\mathcal{B}$ 存在 | `MISSING` |
| 单一 name 映射到多个 id / 非幂等效应重复 | `DUPLICATE` |
| $\mathcal{T}$ 中存在无逻辑溯源的活体 | `ORPHAN` |
| 效应越过容器边界 | `ESCAPED` |

**严重性**：
```
critical := REBOUND ∧ attacker_controlled(id_T) ∧ dataflow(secret → U)
high     := REBOUND ∨ ESCAPED
medium   := RESIDUE ∨ ORPHAN ∨ DUPLICATE
low      := MISSING
```
`attacker_controlled` 与 `dataflow` 均由 M5 图上的可达性给出，不是启发式。

**附加信号 `BELIEF_DIVERGENCE`**：M2 抽取的 `assertion_candidate` 与 $\mathcal{T}$ 中内核实际状态矛盾时输出。**该信号早于副作用落地**，单独成一类，不并入 $\Gamma$ 主格。

> ⚠️ **Agent 注意**：M6 里出现以下任何一项即为缺陷：阈值常量、`in`/正则匹配路径、"看起来像"、tolerance、try/except 兜底成 CONSISTENT。判定不出结果就抛异常。

**验收**：`tests/m6/` 内 7 类 × 每类 ≥ 3 个 golden fixture 全绿；同一输入调用 1000 次结果完全一致（纯函数性质测试）。

---

### M7 `replay` — Memoized Replay 与反事实基线

**职责**：以低成本执行 $\sigma$，并**正确构造对照组**。这是整个框架的正确性命门。

#### M7.1 两条执行路径（不可混淆）

$$\mathcal{B} = \mathrm{Exec}(B \mid \mathrm{logic}{=}c,\ \mathrm{env}{=}\mathrm{snapshot}(c)) \qquad \mathcal{T} = \mathrm{Exec}(B \mid \mathrm{logic}{=}c,\ \mathrm{env}{=}\text{as-is})$$

```
seed run:   ... C ... W ... │k                     ← 录制、建 ℋ*、存 memo + snapshot
                            │
                    ┌───────┴──── restore logic to c ──────┐
  𝒯 (test) :   logic=c, env=as-is  ──► B ──► id_𝒯(U)       │
  ℬ (ideal):   logic=c, env=snap(c) ──► B ──► id_ℬ(U)      │
                    └────────────── Γ(id_𝒯, id_ℬ) ────────┘
```

> ⚠️ **绝对禁止**把 seed run 当作 baseline。那样 $\mathcal{T}$ 与 $\mathcal{B}$ 之间有两个变量在动（逻辑路径 + 环境），diff 无归因能力，全部结论作废。

**$\mathcal{B}$ 的三条硬性要求**：
1. 必须走**同样的 memoized replay**，否则 LLM 采样噪声会污染对照。
2. 按 checkpoint $c$ **缓存复用**：同一 $c$ 的 $\mathcal{B}$ 只算一次，供该 $c$ 下所有 $(k, \delta E, \delta I)$ 共享。
3. `snapshot(c)` 必须来自 M3 的 `ckpt_snapshot_map.jsonl` **原子配对**记录；查不到配对则**抛异常终止**，不许用最近邻快照近似。

#### M7.2 Memoized LLM 与 divergence frontier

```
replay 第 i 步:
  ctx_hash = canon(context_i)                    # M2 的规范化函数，共用
  if ctx_hash in memo:  → fast path（零成本、确定性）
  else:                 → divergence frontier，此后转 slow path（真实调用 LLM）
```

- `frontier_step` 必须记录进 `replay_result.json`（M9 用它做深度覆盖）
- fast/slow 步数比要报（§10 的耗时数量级差就靠它）
- **禁止**在 miss 时"找最相似的 memo 条目"——那会引入不可控偏差

#### M7.3 $\sigma$ 执行器

```python
@dataclass(frozen=True)
class Sigma:
    checkpoint_id: str      # 回退目标 c
    resume_index: int       # 回退点 k ∈ (idx(W), idx(U)]
    nesting: int = 1        # 回退次数（嵌套/重复回退）
    granularity: Literal["tool_call", "mid_command"] = "tool_call"
```
执行后必须 `restore(snapshot[k])` 保证**迭代独立**——注意这与 §M7.1 中 $\mathcal{B}$ 用的快照是**两个不同用途**，不要合并实现。

**验收**：
- 同一 $\sigma$ 跑 10 次，`id_T(U)` 完全一致（确定性）
- replay 平均耗时 / seed run 耗时 ≤ 1/100
- 故意破坏 `ckpt_snapshot_map` 后，M7 抛异常而非静默产出错误 baseline（**负向测试必须有**）

---

### M8 `gapmeter` — Gap 测量（**独立、最优先**）

**职责**：不依赖 M5~M7，独立计算

$$\mathrm{Gap} = 1 - \frac{|\{\text{OS effects with any trace in checkpoint}\}|}{|\{\text{OS effects}\}|}$$

**实现要点**：
- 输入：`kevents.jsonl`（M1）+ `ckpt_index.jsonl`（M2，含每个 checkpoint 序列化内容的**全部键路径与值**）
- "有痕迹"的判定必须**宽松**（对自己不利的方向）：只要 effect 涉及的路径/资源名以任意形式（含子串、含被截断的 stdout 片段）出现在任一 checkpoint 序列化内容中，即计为"有痕迹"。**宁可高估分母命中，也不许高估 Gap。**
- 输出：`gap.json`（数字 + 分资源类拆分）+ `gap_band.pdf`（双带状图：横轴 tool-call 序号，上带 = checkpoint 记录了什么，下带 = 内核实际发生了什么）

**配套 ablation（同属 M8，成本低、杀伤力大）**：
把分支 A 的 OS 操作摘要**手工塞回** context，观察 agent 是否重新校验。输出 `ablation.json`：`revalidation_rate_with_context` vs. `without`。这条直接回答"是不是模型不够聪明"。

**验收**：5–10 个含 shell tool 的普通任务跑通，出图。**这是 P0 的唯一交付物。**

---

### M9 `fuzzer` — 覆盖引导与能量分配

**职责**：调度 $\mathcal{H}^*$ 优先级、枚举变异、维护覆盖度、管理 corpus。

**主循环（与 §5 伪码一致，不得改变结构）**：
```python
for (C, W, U) in prioritize(Hstar):
    c = checkpoint_before(C)
    B = cached_baseline(c) or replay(sigma=(k,c), env=snaps[c], memo=memo)
    for k in range(idx(W)+1, idx(U)+1):
        for dE, dI in mutate(E, I, grammar):
            T_run = replay(sigma=(k,c), env="as-is", memo=memo)
            verdict = gamma(U, T_run, B)
            if verdict.cls != CONSISTENT:
                report(C, W, U, k, dE, dI, verdict)
            restore(snaps[k])
    feedback.update(realized_Hstar, bind_sites, div_classes, depth)
```

**搜索策略（两阶段，顺序不可颠倒）**：
1. **Bounded exhaustive**：$|\sigma| \le 2$ 时穷举全部 $(k, c)$ 组合 → 支撑"此边界内无遗漏"的完备性 claim
2. **Coverage-guided**：AFL 式能量分配，偏向 (未探索 hazard pair 多 / 间接度 $d$ 高 / 资源类稀有) 的 seed

**四维覆盖度（冻结定义）**：

| 维度 | 定义 |
|---|---|
| Bind-site coverage | `(namespace × syscall class × resource class)` 格子命中率 |
| **Realized-$\mathcal{H}^*$** | $\mathcal{H}^*$ 中真正实现 $C\to W\to R\to U$ 且 $U$ 解析到 $W$ 对象的比例 ← **主反馈信号** |
| Divergence-class | $\Gamma$ 的 7 类 × 资源类 |
| Depth | 回退次数、$d(U)$、frontier 之后步数 |

**最小回退距离**：对每个违规，二分搜索最靠后的仍触发违规的 $k$，写入报告（刻画漏洞窗口宽度）。

**验收**：出现 seed run 中不存在的 **新 realized pair**（证明 fuzz 有增益而非仅重放）；覆盖曲线 vs. 随机调度基线有显著分离。

---

### M10 `report` — 图表、PoC、统计

**职责**：把 artifact 转成论文素材。**只读，不产生新结论。**

**必须产出**：

| 产物 | 用途 |
|---|---|
| `gap_band.pdf` | §2 Motivation 主图 |
| `coverage_curve.pdf` | realized-$\mathcal{H}^*$ vs. 时间，四条线（$\mathcal{H}^*$-guided / random σ / kernel-only / NL-only）|
| `violation_matrix.pdf` | 框架 × 违规类 |
| `poc/<vid>/` | 每个 critical 违规一个可复现目录：`repro.sh`、`sigma.json`、`env/`、`timeline_T.jsonl`、`timeline_B.jsonl`、`verdict.json`、`asciinema.cast` |
| `stats.json` | §10.3 全部关键数字 |

**PoC 目录必须 `bash repro.sh` 一键复现**，且在干净机器上验证过。

---

## 4. 数据契约（**冻结**）

### 4.1 Run 目录布局

所有 `.json` 与 `.jsonl` artifact 均为未压缩 UTF-8 文件；P0 不使用或接受
`.zst` 变体。`manifest.json.lock` 仅用于 manifest writer 的 advisory `flock`，不属于
artifact，任何模块不得消费它。

```
runs/<run_id>/
  manifest.json            # RunManifest：版本、image digest、内核版本、dropped_events、剪枝统计
  kevents.jsonl            # M1
  aevents.jsonl            # M2
  memo.jsonl               # M2
  ckpt_index.jsonl         # M2
  ckpt_snapshot_map.jsonl  # M3  ← 原子配对，命门
  snapshots/               # M3
  timeline.jsonl           # M5
  provenance.json          # M5
  hstar.jsonl              # M5
  replays/<sigma_id>/      # M7
  violations.jsonl         # M6+M9
  coverage.jsonl           # M9
  report/                  # M10
```

### 4.2 `KEvent`
```python
class KEvent(BaseModel):
    seq: int                     # 全局单调序号
    ts_mono_ns: int              # CLOCK_MONOTONIC
    tgid: int; tid: int; starttime: int
    ppid: int
    syscall: str
    site: Literal["bind", "resolve", "proc", "mark"]
    args_raw: dict               # 原料：dirfd/cwd/user_path/flags/mode
    ret: int
    errno: int | None            # ← 失败 open 必填
    dev: int | None; ino: int | None
    content_hash: str | None
    cgroup_id: int
```

### 4.3 `AEvent`
```python
class AEvent(BaseModel):
    seq: int; ts_mono_ns: int
    kind: Literal["turn_start","turn_end","tool_call_start","tool_call_end",
                  "checkpoint_written","llm_call","assertion_candidate"]
    turn_id: str
    tool_call_id: str | None
    checkpoint_id: str | None
    ctx_hash: str | None
    payload: dict
```

### 4.4 `ResourceId`（身份函数，冻结）

| 资源类 | $\mathrm{id}(\cdot)$ |
|---|---|
| 文件/路径 | `(dev, ino)` + content hash |
| Unix socket | peer 的 `(pid, starttime, exe hash)`（`SO_PEERCRED` + procfs 交叉验证）|
| 进程 | `(pid, starttime)` |
| 监听端口 | listening sock inode + owner `(pid, starttime)` |
| 可执行命令 | 解析后绝对路径 + `(dev, ino)` + hash |
| shm / mq | `(name, ino, creator identity)` |

文件/路径的完整判定键是 `(dev, ino, content_hash)`；可执行命令的完整判定键包含解析后
的 `abs_path`。`abs_path` 只用于可执行命令身份，不加入普通文件/路径的判定键。

### 4.5 `Timeline` / `HazardPair` / `Verdict` / `Violation`
```python
class TimelineEntry(BaseModel):
    # timeline.jsonl 的单条 record
    seq: int; ts_mono_ns: int
    axis: Literal["kernel", "agent"]
    kevent: KEvent | None; aevent: AEvent | None
    attributed_to: str | None
    orphan: bool; late_effect: bool; via_proxy: str | None

class Timeline(BaseModel):
    # M6 接收的完整共轴时间线容器，不是 JSONL 的单行别名
    entries: tuple[TimelineEntry, ...]

class HazardPair(BaseModel):
    c_site: SiteRef; w_site: SiteRef; u_site: SiteRef
    resource_class: str
    indirection_d: int           # 计算得出，非标注
    evidence_erased: bool
    component_id: str

class Verdict(BaseModel):
    # M6 的纯函数结果；只有 Verdict 可取 CONSISTENT
    cls: Literal["CONSISTENT","REBOUND","RESIDUE","MISSING","DUPLICATE","ORPHAN","ESCAPED","BELIEF_DIVERGENCE"]
    severity: Literal["critical","high","medium","low"]
    id_t: ResourceId | None; id_b: ResourceId | None
    reason_code: str

class Violation(BaseModel):
    vid: str
    hazard: HazardPair
    sigma: Sigma
    delta_env: str | None; delta_inject: str | None
    cls: Literal["REBOUND","RESIDUE","MISSING","DUPLICATE","ORPHAN","ESCAPED","BELIEF_DIVERGENCE"]
    severity: Literal["critical","high","medium","low"]
    id_t: ResourceId | None; id_b: ResourceId | None
    min_rollback_distance: int | None
    poc_dir: str
```

---

## 5. 里程碑与门禁

| ID | 里程碑 | 模块 | 周期 | 门禁（不达标不得进入下一阶段） |
|---|---|---|---|---|
| **P0** | Gap 测量出图 | M0, M1(最小), M2(最小), M8 | 1 周 | `gap.json` + `gap_band.pdf` 产出；Gap 值有分资源类拆分 |
| **P1** | 双轴 + 溯源图 + 基线正确性 | M1 全, M2 全, M3, M5 | 3–4 周 | 手工 `/tmp/keyd.sock` case 被 $\Gamma$ 自动判为 `REBOUND`；**破坏 `ckpt_snapshot_map` 的负向测试必须抛异常** |
| **P2** | 确定性 replay | M7, M6 | 2–3 周 | replay/seed 耗时比 ≤ 1/100；同 $\sigma$ 十次结果一致；$\mathcal{B}$ 按 $c$ 复用生效 |
| **P3** | 覆盖引导 fuzz | M4, M9 | 3–4 周 | 出现 seed run 中不存在的**新 realized pair**；覆盖曲线与随机 $\sigma$ 显著分离 |
| **P4** | Realism：回退触发原语 | M4($I$ 扩展) | 2 周 | 存在可靠 `induce_failure` 原语；统计 L-b 自发回退率 |
| **P5** | 跨框架 | M2 适配层 | 3–4 周 | MAF / Claude Code 各跑通 ≥ 10 任务，出框架矩阵 |
| **P6** | 防御 + artifact | M10, docs | 2 周 | belief-span benchmark 带 ground truth；`repro.sh` 干净机验证 |

**跨模块并行规则**：P1 阶段 M3 与 M5 可并行（M5 先用 M1/M2 的 golden fixture 开发）。其余阶段**串行**，禁止 Agent 提前动后续模块。

---

## 6. 验收标准汇总（可执行）

```bash
make verify-p0   # gap.json 存在 且 0 <= gap <= 1 且 图已生成
make verify-p1   # golden cases 全绿 + 剪枝率>95% + orphan<5% + 负向测试通过
make verify-p2   # 确定性 + 性能比 + baseline 复用命中率
make verify-p3   # 新 realized pair 数 > 0 + 覆盖曲线分离显著性检验
```

每条 `verify-*` 必须是 CI 可跑的**布尔判定**，不许输出"看起来不错"。

---

## 7. 技术栈（冻结，改需 ADR）

| 层 | 选型 | 备注 |
|---|---|---|
| eBPF | C CLI 直接链接 `libbpf` ≥ 1.7.0 + CO-RE；Python 侧 `bcc` 仅用于开发调试 | skeleton 由 `bpftool gen skeleton` 临时生成；生产路径不用 bcc |
| Agent 框架 | LangGraph + `SqliteSaver` | 版本钉死在 `constraints.txt` |
| 数据模型 | Pydantic v2 | schema 导出到 `schemas/` |
| 沙箱 | rootless podman + overlayfs | btrfs 为可选后端 |
| 存储 | 未压缩 UTF-8 JSON/JSONL | 禁止引入数据库与 zstd |
| C JSON | vendor 的 cJSON（ADR-002 固定快照） | C 程序的 JSON 序列化/反序列化只使用该源码；不是跨模块运行时接口 |
| 图计算 | `networkx` | 规模不够时再谈替换 |
| CLI | `typer` | 每模块一个入口 |
| 测试 | `pytest` + golden fixtures | 快照测试用 `syrupy` |
| 架构守护 | `import-linter` | CI 强制 §2.2 依赖规则 |
| 出图 | `matplotlib`（无 seaborn） | 论文风格统一在 `m10/style.py` |

---

## 8. 资源状态分类法（M5 查表用，声明式）

`m5/rebindable.yaml`：

| 状态类 | 例子 | 可再绑定 | 回退可逆 |
|---|---|---|---|
| `NAME_BINDING` | 路径→inode、symlink、`PATH` 项、socket 文件 | ✅ | ❌ |
| `LIVE_ENDPOINT` | 监听端口、运行中进程、fd | ✅ | ❌ |
| `CONFIG_STATE` | dotfile、`.git/config`、env 文件 | ✅ | 部分 |
| `SCHEDULED` | cron、systemd timer、at | ✅ | ❌ |
| `ESCAPED` | 远程 push、外部 API 写、发信 | — | ❌ 永久 |
| `PURE_DATA` | 普通文件内容 | ❌ | ✅ |

> 只有前四类进入 $\mathcal{S}$。`PURE_DATA` 被排除是 novelty 的一部分——普通 TOCTOU 检测器覆盖不了前四类。

---

## 9. 风险与已排除项

| 风险 | 缓解 |
|---|---|
| eBPF 丢事件导致漏报 | ringbuf + `dropped_events > 0` 时 M5 硬失败 |
| `ctx_hash` 规范化不足 → memo 命中率低 | `canon.py` 单测覆盖；命中率写进 manifest，< 80% 时报警 |
| 快照/checkpoint 配对错位 | 原子配对 + 负向测试 + 查不到即抛异常 |
| orphan 归属率高 | 进程树优先 + socket peer 追溯；> 5% 报警而非静默 |
| LangGraph API 变动 | 版本钉死 + M2 内做 adapter 层隔离 |
| $\mathcal{H}^*$ 爆炸 | 三级剪枝 + `MAX_PAIRS_PER_COMPONENT` |

**已排除（写进代码注释，防 Agent 手痒）**：PID 复用攻击、auto-compact、跨机分布式、在线防御、Web UI、非 Linux 平台、fuzz 自然语言 prompt。

---

## 10. 交付物清单

- [ ] `syncfuzz/` 十个模块，各自 CLI 可独立运行
- [ ] `schemas/*.json` 冻结 schema
- [ ] `tests/` golden fixtures，含**负向测试**
- [ ] `docs/adr/`、`docs/BACKLOG.md`、`docs/OPEN_QUESTIONS.md`
- [ ] `Makefile` 含 `verify-p0..p3`
- [ ] `report/` 三张主图 + `stats.json`
- [ ] `poc/` 每个 critical 违规一目录，`repro.sh` 干净机验证通过
