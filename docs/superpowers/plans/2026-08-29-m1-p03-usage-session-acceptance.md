# M1-P03 Usage Session Acceptance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 P02 的多轮流式基础上，记录可靠 usage、保存脱敏会话、分类错误并完成 v0.1 验收。

**Architecture:** 保留现有 `llm`、`app` 和 `conversation` 边界，在 LLM 层输出可判断的 usage 与错误 cause，在应用层聚合每轮运行结果，在独立 session 层负责脱敏和本地保存。会话保存是显式的副作用，不能污染核心模型调用和内存会话状态。

**Tech Stack:** Go 1.27、标准库 `encoding/json/v2`、`errors`、`os`、`path/filepath`、`time`、`httptest`。

## Global Constraints

- 先写失败测试，再实现最小代码。
- API Key、Authorization Header 和未脱敏敏感数据不得进入 session 文件、证据或日志。
- Provider 不返回 usage 时必须保留 `Known=false`，不能把零值解释成真实统计。
- 取消、断流和协议失败不能提交不完整 assistant 消息。
- 不在 P03 迁移 OpenAI SDK，不实现重试、限流、熔断和 Provider 路由。
- 每段关键代码添加中文注释，公开方法添加中文文档注释。
- 文件保存和错误分类必须可测试，不能把所有逻辑堆入 `main.go`。

## 文件职责地图

- `internal/llm/client.go`：提供每次调用的 usage 和稳定错误 cause。
- `internal/llm/client_test.go`：验证 usage 已知/未知、流式 usage 和底层错误。
- `internal/app/run.go`：编排每轮 usage、会话快照和保存时机。
- `internal/app/run_test.go`：验证多轮累计 usage、取消和保存失败行为。
- `internal/conversation/conversation.go`：维护内存消息，并导出只读快照。
- `internal/conversation/conversation_test.go`：验证快照和失败消息状态。
- `internal/session/record.go`：定义可保存的 session record 和 usage 汇总。
- `internal/session/sanitize.go`：执行字段级脱敏，禁止密钥进入记录。
- `internal/session/store.go`：实现本地保存、目录创建、权限和写入错误。
- `internal/session/*_test.go`：验证序列化、脱敏、保存成功/失败和关闭保存。
- `internal/errors/errors.go`：定义配置、网络、HTTP、协议和取消类别；若实现很小可并入现有包。
- `cmd/opspilot/main.go`：解析 session 保存和关闭保存选项，并传递取消上下文。
- `README.md`：补充运行参数、保存位置和安全说明。
- `docs/evidence/M1-P03-usage-session-acceptance.md`：保存最终验证记录。
- `docs/milestones/v0.1.md`：同步 P03 和 v0.1 状态。

## Task 0: 建立 P03 基线

- [ ] 确认分支为 `feature/m1-p03-usage-session-acceptance`。
- [ ] 确认工作区没有 P02 遗留的未提交无关改动。
- [ ] 运行 `go test ./...`、`go test -race ./...`、`go vet ./...` 和 `go build ./cmd/opspilot`。
- [ ] 阅读 `docs/milestones/v0.1-m1-p03.md`，确认本阶段不提前做 v0.2 或 P04 内容。

## Task 1: 统一 usage 模型与错误类别

**目标：** 让上层可以区分 usage 未知和 usage 为零，并能够稳定判断错误类别。

- [ ] 写测试：非流式响应有 usage 时 `Known=true` 且数值正确。
- [ ] 写测试：非流式响应没有 usage 时 `Known=false`。
- [ ] 写测试：流式响应有 usage 时正确返回；没有 usage 时保持未知。
- [ ] 写测试：配置、网络、HTTP、协议和取消错误可以使用 `errors.Is` 或 `errors.As` 判断。
- [ ] 运行 `go test ./internal/llm ./internal/config -v`，确认新增测试失败。
- [ ] 定义错误类别和包装方式，保留底层 cause，不改变 P02 已有错误文本的可读性。
- [ ] 让 Chat 和 Stream 复用 usage 转换逻辑，避免两套统计语义。
- [ ] 运行 LLM/config 测试和 `go vet`。

## Task 2: 实现多轮 usage 聚合

**目标：** 将每轮模型返回的 usage 转换成可复查的单轮和累计统计。

**Interfaces:**
- 建议定义 `TurnUsage` 和 `UsageSummary`，字段明确区分 `Known`。
- 每轮记录 prompt、completion、total 和是否已知。
- 累计统计只有在所有参与累计的数值都已知时才标记为完整；未知数据不能静默按零相加。

- [ ] 写测试：两轮已知 usage 正确计算累计值。
- [ ] 写测试：任意一轮未知时，累计结果明确表示不完整或未知。
- [ ] 写测试：取消和断流轮次不伪造 completion usage。
- [ ] 运行目标测试确认失败。
- [ ] 在应用层接入 usage 聚合，保持 `llm` 层只负责单次调用结果。
- [ ] 验证多轮请求和 usage 输出不影响现有流式输出。

## Task 3: 定义会话记录和脱敏规则

**目标：** 将内存会话转换为安全、可序列化、可复查的记录。

- [ ] 写测试：记录包含 session ID、创建时间、消息和 usage。
- [ ] 写测试：记录中不出现 API Key、Authorization、Base URL 中的敏感凭据或环境变量全量转储。
- [ ] 写测试：消息内容保持原样，但敏感配置字段被删除或替换为固定脱敏标记。
- [ ] 写测试：失败/取消轮次不出现半截 assistant 消息。
- [ ] 运行 session 测试确认失败。
- [ ] 创建 `internal/session/record.go` 和 `sanitize.go`，只保留必要字段。
- [ ] 定义未知 usage 的 JSON 表达，避免读者把零值误认为真实统计。
- [ ] 运行 session 测试和 `go vet`。

## Task 4: 实现本地会话保存

**目标：** 提供可关闭、可测试且默认安全的本地保存能力。

- [ ] 写测试：保存成功后文件可读取并还原记录。
- [ ] 写测试：目标目录不存在时按约定创建。
- [ ] 写测试：文件写入失败返回明确错误。
- [ ] 写测试：关闭保存时不创建文件，也不阻塞核心诊断结果。
- [ ] 写测试：重复保存采用明确策略，不产生不可预测的覆盖行为。
- [ ] 运行 session store 测试确认失败。
- [ ] 使用临时目录实现文件存储，设置合理文件权限，避免把真实路径写死在业务代码中。
- [ ] 在应用层决定保存时机：完整成功结果可保存；失败轮次按记录策略保存但不伪造 assistant。
- [ ] 运行保存测试、竞态测试和 `go vet`。

## Task 5: 接入 CLI 配置和错误展示

**目标：** 让用户能理解 usage、保存状态和错误类别，并能关闭会话保存。

- [ ] 写测试：默认配置、显式关闭保存和自定义保存路径行为明确。
- [ ] 写测试：配置错误在发起模型请求前返回。
- [ ] 写测试：普通 HTTP/协议错误显示类别并允许按既定策略结束或继续。
- [ ] 写测试：取消错误不会被显示成普通服务端失败。
- [ ] 运行 CLI 测试确认失败。
- [ ] 在 `cmd/opspilot/main.go` 或独立参数模块中接入参数，避免增加大段业务逻辑。
- [ ] 更新 README，说明保存位置、关闭方式、usage 未知含义和敏感信息边界。
- [ ] 运行全量测试和构建。

## Task 6: 完成证据和 v0.1 复盘

- [ ] 创建 `docs/evidence/M1-P03-usage-session-acceptance.md`，记录环境、命令和结果。
- [ ] 记录正常 usage、usage 缺失、会话保存、关闭保存、保存失败、错误分类和取消证据。
- [ ] 执行 `gofmt -w cmd internal`。
- [ ] 执行 `go test ./...`。
- [ ] 执行 `go test -race ./...`。
- [ ] 执行 `go vet ./...`。
- [ ] 执行 `go build ./cmd/opspilot`。
- [ ] 执行 `git diff --check` 并检查 `git status --short`。
- [ ] 更新 `docs/milestones/v0.1-m1-p03.md` 和 `docs/milestones/v0.1.md`，只勾选有证据支撑的项目。
- [ ] 写一篇 P03 实战笔记，说明 usage 未知、脱敏保存和错误分类的取舍。
- [ ] 只有所有检查通过后，才创建 `v0.1.0` tag。

## 完成判定

P03 不是“加一个保存文件”的小改动。只有当统计语义、安全边界、错误可诊断性、测试和证据形成闭环后，才可以将 v0.1 标记为可发布，并进入 v0.2 的 Prompt 与结构化输出工作。
