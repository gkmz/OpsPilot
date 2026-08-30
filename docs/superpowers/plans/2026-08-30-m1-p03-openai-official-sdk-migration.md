# M1-P03 OpenAI 官方 Go SDK 迁移与能力补齐实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在开始 P03 新功能前，将 OpsPilot 从自写 OpenAI 兼容 HTTP/SSE 客户端迁移到 OpenAI 官方 Go SDK，并在新调用边界上完成 usage、会话安全保存、错误分类和 v0.1 验收。

**Architecture:** 先验证并锁定官方 SDK、API 表面和版本，再把 `internal/llm` 改造成面向 OpsPilot 的薄适配层；应用层只依赖本项目自己的请求、响应、usage 和错误接口，不直接依赖 SDK 类型。默认评估 `github.com/openai/openai-go` 的 Responses API；如果当前模型、兼容 Base URL 或流式 usage 无法满足 P03 验收，则记录决策并使用官方 SDK 的 Chat Completions API 作为过渡，禁止继续扩展自写 HTTP 协议代码。会话与本地存储保持独立，迁移失败可回滚到迁移前提交。

**Tech Stack:** Go 1.27、OpenAI 官方 Go SDK（版本在 Task 0 锁定）、`context`、`errors`、`encoding/json/v2`、`os`、`filepath`、`httptest`。

## Global Constraints

- 后端继续使用 Go，关键代码使用中文注释，公开方法提供中文文档注释。
- `internal/app`、`internal/conversation` 不得依赖 OpenAI SDK 的公开类型；SDK 依赖只允许出现在 `internal/llm` 及其测试替身边界。
- API Key、Authorization Header、环境变量全集和 SDK 原始请求不得写入 session、证据或普通日志。
- Provider 未返回 usage 时必须显式 `Known=false`，不得把零值当成真实统计；取消、断流和协议失败不得提交半截 assistant 消息。
- P03 不做自动重试、限流、熔断、Provider 路由、RAG、Agent、MCP、自动修复或 React 工作台。
- SDK 迁移必须保留可配置 Base URL、模型名、超时和 context 取消能力；不兼容能力必须在文档中明确记录。
- 计划执行期间不在功能点之间提交；全部功能和整体回归通过后再做一个 Conventional Commit 聚合提交。

## 文件职责地图

- `go.mod`、`go.sum`：锁定官方 SDK 依赖和版本。
- `internal/llm/client.go`：官方 SDK 的薄适配器，暴露稳定的 `Chat`/`Stream`、usage 和错误映射。
- `internal/llm/client_test.go`：用 `httptest` 验证 SDK 发出的请求、流式事件、usage、取消和协议失败。
- `internal/llm/fake_client.go`、`internal/llm/fake_client_test.go`（必要时）：为 app 测试提供与 SDK 无关的可控替身。
- `internal/errors/errors.go`：定义配置、网络、HTTP、协议、取消等类别及底层 cause 包装。
- `internal/app/run.go`、`internal/app/run_test.go`：编排 SDK 适配器、每轮 usage、会话保存和 CLI 展示。
- `internal/conversation/conversation.go`、对应测试：维护消息和只读快照，保证失败轮次不提交 assistant。
- `internal/session/record.go`、`sanitize.go`、`store.go`、对应测试：定义脱敏记录、未知 usage 表达和安全本地保存。
- `cmd/opspilot/main.go`：保留信号取消入口；如需要，增加关闭保存和保存路径参数。
- `README.md`、`docs/milestones/v0.1-m1-p03.md`、`docs/milestones/v0.1.md`：同步 SDK、配置、边界和验收说明。
- `docs/evidence/M1-P03-openai-sdk-migration.md`：记录迁移决策、测试命令、请求验证和残余风险。
- `docs/notes/`：记录官方 SDK 迁移、Responses/Chat Completions 选择、usage 与取消语义。

### Task 0: 建立基线和迁移决策输入

**目标：** 在任何代码迁移前确认工作区、当前行为和官方 SDK 能力，避免把未验证的 API 设计写进业务层。

- [ ] 记录 `git status --short`、当前分支和现有 P02 测试结果。
- [ ] 阅读 `README.md`、`docs/milestones/v0.1-m1-p03.md` 和现有 `internal/llm`，列出必须保持的行为：多轮历史、流式输出、Ctrl+C 取消、失败不提交半截 assistant。
- [ ] 查阅官方 SDK 的当前文档和包 API，确认 Go 模块路径、可用版本、Responses 流式事件、usage 字段、Base URL、超时和错误类型。
- [ ] 写一份迁移决策记录：默认选择 Responses API；若现有兼容服务或目标模型不支持，明确选择官方 SDK Chat Completions，并写出不做双协议长期并存的理由。
- [ ] 运行 `go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./cmd/opspilot`，保存基线输出。

### Task 1: 引入 SDK 并建立 `internal/llm` 适配器契约

**目标：** 先固定业务接口，再替换底层实现。

**Interfaces:**

- `type Usage struct { PromptTokens, CompletionTokens, TotalTokens int; Known bool }`
- `type Response struct { Content string; Usage Usage }`
- `type Client interface { Chat(context.Context, []Message) (Response, error); Stream(context.Context, []Message, func(string) error) (Usage, error) }`
- `NewClient(config.Config) Client`；SDK 具体实现类型命名为 `SDKClient`，SDK 类型不得泄漏。

- [ ] 为业务接口补写失败测试：已知/未知 usage、空响应、底层 SDK 错误可被上层识别。
- [ ] 执行 `go get github.com/openai/openai-go@latest`，记录命令解析出的具体版本并检查 `go.mod`/`go.sum` 只引入必要依赖；后续构建严格使用该已写入 `go.mod` 的版本。
- [ ] 将 SDK client 初始化集中到 `internal/llm`，把 Base URL、API Key、Model、Timeout 从 `config.Config` 映射进去。
- [ ] 删除或替换 `client.go` 中手写 `http.NewRequest`、JSON 协议结构体和 SSE scanner；保留 OpsPilot 的 `Message`、`Response`、`Usage` 数据模型。
- [ ] 添加中文文档注释，运行 `go test ./internal/llm ./internal/config` 和 `go vet ./...`。

### Task 2: 迁移非流式和流式调用语义

**目标：** 在官方 SDK 上恢复 P02 的完整调用行为，并验证请求路径、消息映射和取消语义。

- [ ] 写 `httptest` 失败测试，验证 SDK 请求包含正确模型、system/user 历史、鉴权和 Responses（或决策记录中的 Chat Completions）路径。
- [ ] 写流式失败测试，覆盖正常增量、服务端提前结束、非法事件、回调失败和 context cancellation。
- [ ] 实现 SDK 消息到 `[]Message` 的转换，以及流事件到 `onChunk` 的转换；只有 SDK 明确报告完成事件后才返回成功。
- [ ] 将 SDK 错误统一映射为项目错误类别，同时保留 `errors.Is`/`errors.As` 可用的底层 cause。
- [ ] 确认 SDK 默认超时与 `context` 取消不会叠加出不可诊断的错误；补充超时测试。
- [ ] 运行 `go test ./internal/llm -v`、`go test -race ./internal/llm`。

### Task 3: 接回 app 和可控测试替身

**目标：** 让应用编排依赖项目接口，避免后续测试绑定真实 SDK。

- [ ] 先修改 `RunInteractive` 和 `runTurn` 的参数类型为 `llm.Client`，使现有交互测试编译失败并暴露耦合点。
- [ ] 创建最小 fake client，能按轮次返回 chunks、usage、错误和取消结果。
- [ ] 恢复并扩展 app 测试：首轮参数、后续历史、正常流式输出、取消、断流和错误后继续交互。
- [ ] 验证失败轮次只保留 user，不提交半截 assistant；成功轮次提交完整 assistant。
- [ ] 运行 `go test ./internal/app ./internal/conversation -v` 和 `go test -race ./...`。

### Task 4: usage 模型和多轮聚合

**目标：** 在 SDK 新响应模型上建立可复查的单轮和累计 usage。

- [ ] 为 `TurnUsage`/`UsageSummary` 写测试：两轮已知值正确累计，任一轮未知时累计标记不完整，取消/断流不伪造 completion。
- [ ] 适配 Responses/Chat Completions 的 usage 字段，统一映射到 `Usage`；未知状态必须序列化为显式字段而非隐含零值。
- [ ] 在 app 层记录每轮 usage 和会话累计 usage，保持流式和非流式语义一致。
- [ ] 为 CLI 增加简洁 usage 展示或明确的静默策略，并在 README 说明统计不等于成本结算。
- [ ] 运行 usage 相关测试和 `go vet ./...`。

### Task 5: 会话记录、脱敏和本地保存

**目标：** 保存可复查但不泄露凭据的会话。

- [ ] 写测试覆盖记录字段、消息快照、每轮/累计 usage、失败轮次边界和 API Key/Authorization/Base URL 凭据不落盘。
- [ ] 创建 `internal/session` 的 record、sanitize、store；使用临时目录、受限文件权限、原子写入和明确覆盖策略。
- [ ] 提供默认保存位置与关闭保存选项；保存失败返回可理解错误但不影响内存结果。
- [ ] 在 `Run`/`runTurn` 接入保存时机：成功轮次保存完整 assistant，失败/取消只保存允许的状态和已提交消息。
- [ ] 运行 session/app 测试、竞态检测并检查生成文件内容。

### Task 6: CLI、配置和错误展示

**目标：** 用户可以配置官方 SDK 所需参数，理解错误类别和保存状态。

- [ ] 写测试覆盖缺失/非法 API Key、Base URL、Model、Timeout，以及保存路径和关闭保存配置。
- [ ] 更新 CLI 参数或环境变量说明，明确官方 SDK 的 endpoint 规则；若使用 Responses API，明确不再手工追加 `/chat/completions`。
- [ ] 将配置、网络、HTTP、协议、取消错误映射为稳定类别；展示信息不得包含 API Key 或完整请求头。
- [ ] 验证取消后进程可退出、错误后可继续下一轮、保存失败不会吞掉诊断结果。
- [ ] 运行 `go test ./cmd/opspilot ./internal/config ./internal/app -v`。

### Task 7: 文档、证据和 v0.1 整体回归

**目标：** 让迁移和 P03 功能有可复现证据，再进行版本收口。

- [ ] 更新 `README.md`、P03 里程碑和 v0.1 复盘：官方 SDK/API 选择、配置、usage 未知语义、会话目录、安全边界和明确不做项。
- [ ] 创建 `docs/evidence/M1-P03-openai-sdk-migration.md`，记录 SDK 版本、假服务请求断言、正常/取消/HTTP/协议/usage/保存失败证据。
- [ ] 运行完整检查：`go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./cmd/opspilot`。
- [ ] 检查 `git diff`、`git status --short`、session 文件权限和证据中的敏感信息；确认无无关生成物。
- [ ] 全部通过后创建聚合提交：`feat: migrate P03 to official OpenAI Go SDK`；再按项目发布流程创建 `v0.1.0` tag。

## 验收清单

- [ ] 运行时已使用锁定版本的 OpenAI 官方 Go SDK，业务层无手写 OpenAI HTTP/SSE 协议。
- [ ] Base URL、API Key、Model、Timeout 和 context cancellation 行为保持可配置且可测试。
- [ ] Responses（或决策记录确认的 Chat Completions）非流式/流式调用均可运行，失败不提交半截 assistant。
- [ ] usage 已知/未知、多轮单轮/累计和取消/断流边界均有测试。
- [ ] 会话成功保存、关闭保存、保存失败、文件权限和敏感字段脱敏均有测试。
- [ ] 配置、网络、HTTP、协议和取消错误可区分且保留底层 cause。
- [ ] `go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./cmd/opspilot` 全部通过。
- [ ] 文档、证据、里程碑状态一致，未包含密钥或完整请求头。

## 自检结果

- **需求覆盖：** SDK 迁移在 Task 0-2；app 解耦在 Task 3；usage 在 Task 4；会话安全保存 Task 5；错误和 CLI Task 6；验收证据 Task 7。
- **无占位符：** SDK 版本和 API 选择是 Task 0 的明确决策输入，不在后续任务中假装已知；其余任务给出文件、接口、命令和边界。
- **类型一致性：** `llm.Client` 是 app 依赖的接口，`Usage`/`Response` 保持跨 SDK 稳定；具体 SDK 类型仅存在于 llm 实现内。
