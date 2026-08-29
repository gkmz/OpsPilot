# M1-P02 Multiturn Streaming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有单次非流式 CLI 基础上，增加可测试的多轮会话、流式增量输出、上下文取消和失败回滚能力。

**Architecture:** 保留 `internal/llm` 作为 OpenAI 兼容协议客户端，在其中增加流式请求和 SSE 增量解析；新增轻量会话对象管理消息历史，并由 `internal/app` 负责交互编排。assistant 消息只在流式响应完整结束后提交，取消、断流和协议错误只影响当前请求，不污染历史。

**Tech Stack:** Go 1.27、标准库 `net/http`、`bufio`、`encoding/json/v2`、`context`、`httptest`。

## Global Constraints

- 后端主要使用 Go，保持按模块和业务职责拆分，避免继续扩大单个 `main.go`。
- 每段关键代码添加明确的中文注释，公开方法添加中文文档注释。
- 不引入第三方 SSE 或 CLI 框架，先使用标准库理解协议和控制流。
- 不在 P02 实现重试、限流、熔断、Provider 路由、持久化、RAG、Agent 或 MCP。
- 每个功能点先补失败测试，再实现最小代码，最后运行针对性测试和 `go vet`。
- 不提交 API Key、真实端点、完整生产日志或未脱敏会话内容。

## 文件职责地图

- `internal/llm/client.go`：保留非流式 `Chat`，增加流式 HTTP 请求入口和响应生命周期管理。
- `internal/llm/stream.go`：负责 SSE 行读取、JSON 增量事件解析、完成标记和流式协议错误；避免继续扩大 `client.go`。
- `internal/llm/client_test.go`：验证 HTTP 请求、非流式兼容性和流式客户端行为。
- `internal/llm/stream_test.go`：验证 SSE 解析、分片、`[DONE]`、断流和非法事件。
- `internal/app/conversation.go`：负责会话消息历史和 assistant 成功提交规则。
- `internal/app/conversation_test.go`：验证多轮顺序、复制隔离和失败不提交。
- `internal/app/run.go`：保留单次模式；将配置和客户端组装复用到多轮/流式流程。
- `internal/app/interactive.go`：负责终端输入循环、每轮输出和错误/取消状态，不把协议解析写进应用层。
- `internal/app/interactive_test.go`：使用本地假服务验证多轮交互、增量输出和失败回滚。
- `cmd/opspilot/main.go`：增加明确的 P02 CLI 模式选择，并保留现有单次调用兼容行为。
- `README.md`：补充 P02 命令示例、交互输入约定和取消方式。
- `docs/evidence/M1-P02-multiturn-streaming.md`：记录可复查的正常、取消、断流和错误验证结果。
- `docs/milestones/v0.1-m1-p02.md`：实现完成后勾选验收项并记录实际限制。

## Task 0: 确认开发基线

**目的：** 确认你正在 P02 分支上，并保存 P01 已通过的基线结果。

**Files:**
- Modify: 无
- Test: 无

- [ ] 查看 `git status --short --branch`，确认分支为 `feature/m1-p02-multiturn-streaming`；如果仍在 `feature/m1-p01-basic-cli`，先切换到 P02 分支。
- [ ] 运行 `go test ./...`、`go vet ./...` 和 `go build ./cmd/opspilot`，确认修改前基线通过。
- [ ] 阅读 `docs/milestones/v0.1-m1-p02.md`，将本计划作为实现清单，不提前实现 P03 内容。

## Task 1: 建立会话消息历史

**目的：** 先解决“哪些消息进入下一轮请求”这个核心问题，不涉及网络流式协议。

**Files:**
- Create: `internal/app/conversation.go`
- Create: `internal/app/conversation_test.go`
- Modify: `internal/diagnosis/prompt.go`（仅在需要复用 system message 时调整）

**Interfaces:**
- Produces `NewConversation(systemMessage llm.Message) *Conversation`。
- Produces `Messages() []llm.Message`，返回副本，避免调用方绕过会话规则修改内部状态。
- Produces `AddUser(content string)`，追加一条非空 user 消息。
- Produces `CommitAssistant(content string)`，只提交完整且非空的 assistant 消息。

- [ ] 写测试：新会话包含一条 system 消息。
- [ ] 写测试：依次添加 user、提交 assistant、再添加第二条 user，验证消息顺序为 system/user/assistant/user。
- [ ] 写测试：`Messages()` 返回副本，外部修改不会改变会话内部历史。
- [ ] 写测试：空 assistant 不提交；模拟失败请求时不调用提交方法，历史不出现半截回答。
- [ ] 运行 `go test ./internal/app -run Conversation -v`，确认测试先失败。
- [ ] 实现最小 `Conversation`，补充中文文档注释和关键状态说明。
- [ ] 重新运行上述测试，再运行 `go vet ./internal/app`。

## Task 2: 实现 SSE 增量事件解析

**目的：** 单独理解 OpenAI 兼容流式响应的传输格式，确保断流和非法数据不会被误判为成功。

**Files:**
- Create: `internal/llm/stream.go`
- Create: `internal/llm/stream_test.go`

**Interfaces:**
- 定义内部流式事件结构，解析 `choices[0].delta.content` 和可选 `usage`。
- 提供内部解析函数，例如 `readStream(reader io.Reader, onChunk func(string) error) (Usage, error)`；函数遇到 `[DONE]` 才能正常完成。
- 不让应用层直接处理 `data:` 前缀、空行或 JSON 细节。

- [ ] 写测试：多个 `data:` 事件按顺序回调文本片段，并在 `[DONE]` 后成功返回。
- [ ] 写测试：忽略 SSE 空行和注释行；空 content 事件不产生错误。
- [ ] 写测试：JSON 非法、事件缺少必要结构、响应提前 EOF 都返回明确错误。
- [ ] 写测试：回调返回错误时立即停止读取并原样包装错误。
- [ ] 运行 `go test ./internal/llm -run Stream -v`，确认测试先失败。
- [ ] 用 `bufio.Scanner` 或等价标准库实现有大小上限的事件读取，避免无限读取单行数据。
- [ ] 重新运行流式解析测试和 `go vet ./internal/llm`。

## Task 3: 增加 LLM 流式客户端

**目的：** 将解析器接到真实 HTTP 生命周期中，并验证请求参数和上下文取消。

**Files:**
- Modify: `internal/llm/client.go`
- Modify: `internal/llm/stream.go`
- Modify: `internal/llm/client_test.go`
- Modify: `internal/llm/stream_test.go`

**Interfaces:**
- 提供公开方法 `Stream(ctx context.Context, messages []Message, onChunk func(string) error) (Usage, error)`。
- `Stream` 请求体在已有 `model` 和 `messages` 外增加 `stream: true`。
- `Stream` 使用现有鉴权、Base URL、超时和错误分类约定；正常结束返回 usage，未提供 usage 时 `Known=false`。

- [ ] 写测试：假服务校验 `stream=true`、模型名、消息历史和 Authorization。
- [ ] 写测试：假服务发送多个 SSE 事件，确认回调按顺序收到片段和最终 usage。
- [ ] 写测试：HTTP 非 2xx 返回状态码和响应正文摘要。
- [ ] 写测试：上下文取消期间服务端持续发送数据，客户端及时返回 `context.Canceled` 或可识别的取消错误。
- [ ] 写测试：服务端未发送 `[DONE]` 就关闭连接，客户端返回断流错误。
- [ ] 运行 `go test ./internal/llm -run 'Chat|Stream' -v`，确认新增测试先失败。
- [ ] 实现 `Stream`，确保 `resp.Body.Close()`、请求上下文和读取循环都受控。
- [ ] 重新运行 LLM 测试、`go test -race ./internal/llm` 和 `go vet ./internal/llm`。

## Task 4: 编排单轮流式会话

**目的：** 让应用层可以把一次用户输入发送给模型，并把增量内容写到输出，同时只在成功后提交 assistant。

**Files:**
- Create: `internal/app/interactive.go`
- Create: `internal/app/interactive_test.go`
- Modify: `internal/app/run.go`

**Interfaces:**
- 建议提供 `RunInteractive(ctx context.Context, client *llm.Client, conversation *Conversation, input io.Reader, stdout, stderr io.Writer) error`，或等价的清晰接口。
- 每轮流程固定为：读取用户输入 → `AddUser` → `Stream` → 增量写 stdout → 成功后 `CommitAssistant`。
- 输出层不负责解析协议；协议错误从 `llm` 层传递到应用层。

- [ ] 写测试：单轮输入得到多个增量片段，stdout 按到达顺序输出，完整文本提交到会话。
- [ ] 写测试：流式失败时 stdout 可以保留已显示片段，但会话历史没有 assistant 消息。
- [ ] 写测试：空输入和 EOF 的行为明确，不发送空 user 消息。
- [ ] 运行 `go test ./internal/app -run Interactive -v`，确认测试先失败。
- [ ] 实现最小单轮编排，并复用现有配置读取和客户端创建逻辑。
- [ ] 重新运行应用层测试、`go test -race ./internal/app` 和 `go vet ./internal/app`。

## Task 5: 接入 CLI 多轮交互和取消

**目的：** 把应用能力暴露给用户，同时不破坏原有单次命令。

**Files:**
- Modify: `cmd/opspilot/main.go`
- Modify: `internal/app/run.go`
- Modify: `internal/app/interactive.go`
- Modify: `internal/app/interactive_test.go`
- Modify: `README.md`

**Interfaces:**
- 保留现有 `go run ./cmd/opspilot "故障描述"` 的单次非流式行为。
- 增加一个明确的 P02 入口，例如 `--interactive`；如果选择流式模式，使用 `--stream` 或让交互模式默认流式，但必须在 README 中固定约定。
- 终端输入以空行、EOF 或明确命令结束一轮/退出；不要依赖不可测试的终端特性。

- [ ] 先确定并写下 CLI 约定：单次模式、交互模式、退出方式和错误输出位置。
- [ ] 为参数解析和模式选择写测试，验证旧命令仍然走 `Run`。
- [ ] 为多轮输入写测试：第二轮请求必须包含第一轮完整 user/assistant 历史。
- [ ] 为输入 EOF、空行、模型错误和上下文取消写测试。
- [ ] 运行目标测试，确认测试先失败。
- [ ] 实现 CLI 模式选择和交互循环；`main` 只负责参数、上下文、I/O 和退出码，不承载业务循环细节。
- [ ] 增加取消信号处理，确保取消当前请求后退出循环并返回可识别错误；不要保存半截 assistant。
- [ ] 更新 README，提供不含真实密钥的本地假服务/行为说明、交互示例和取消方式。
- [ ] 运行 `go test -race ./...`、`go vet ./...` 和 `go build ./cmd/opspilot`。

## Task 6: 完成证据、回归和里程碑收尾

**目的：** 将 P02 从“代码能跑”变成可复查的课程实践交付物。

**Files:**
- Create: `docs/evidence/M1-P02-multiturn-streaming.md`
- Modify: `docs/milestones/v0.1-m1-p02.md`
- Modify: `README.md`（仅在验证命令或行为有变化时）

- [ ] 用本地 `httptest` 或专用假服务记录正常流式、多轮、取消、断流和 HTTP 错误证据。
- [ ] 记录测试命令、Go 版本、操作系统、结果摘要和已知限制；不得写入 API Key。
- [ ] 执行 `gofmt -w cmd internal`。
- [ ] 执行 `go test -race ./...`。
- [ ] 执行 `go vet ./...`。
- [ ] 执行 `go build ./cmd/opspilot`。
- [ ] 查看 `git diff --check`、`git diff` 和 `git status --short`，确认没有生成物或无关改动。
- [ ] 更新 `docs/milestones/v0.1-m1-p02.md` 的验收清单，仅勾选真实验证通过的项目。
- [ ] 如果 P02 全部完成，再考虑使用 Conventional Commits 创建一个聚合提交，例如 `feat: add multiturn streaming diagnosis`；在此之前不要提交中间状态。

## 推荐开发节奏

每完成一个 Task 都暂停一次，确认三件事：

1. 测试是否覆盖了这个 Task 的失败路径，而不只是成功路径；
2. 新增代码是否仍然保持 `llm`、会话和 CLI 编排的职责边界；
3. 是否能用一句话解释“为什么 assistant 只能在完整成功后进入历史”。

建议先完成 Task 1，再进入 Task 2。不要同时修改流式协议、CLI 参数和会话模型；分层完成更容易定位问题，也更适合熟悉 Go 的 `context`、HTTP 响应体和测试替身。
