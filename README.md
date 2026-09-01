# OpsPilot

OpsPilot 是一个面向 Go HTTP 微服务的研发故障诊断与处置助手，也是“大模型应用开发实战”课程的持续实践项目。

它逐步使用日志、Git 变更、Markdown Runbook 和历史事故样本，帮助开发者形成带证据的诊断结论和排查步骤。项目默认只读；没有有效证据时不输出确定结论；高风险操作必须经过人工确认。

## 当前阶段

当前处于 `v0.1` 阶段，已完成 `M1-P03` 的 usage、会话持久化、错误分类和验收闭环：

- 命令行参数可以作为第一轮故障描述直接发起流式分析；
- 同一个 CLI 进程中可以继续补充多轮信息；
- 上下文取消、服务端断流和错误响应不会保存半截 assistant 消息；
- 每轮调用结束后显示输入、输出和总 Token；Provider 未返回 usage 时显示“未知”；
- 成功轮次默认保存脱敏会话到 `~/.opspilot/sessions`，可通过 `OPSPILOT_SESSION_DIR` 覆盖；
- 使用 `diagnose --no-session` 可以关闭本次会话保存；
- 配置、网络、HTTP、协议、取消、输出和存储错误使用稳定类别展示。

`v0.1` 不包含 RAG、Agent、MCP、自动修复和生产级可靠性治理。

## 项目文档

- [项目总纲与路线图](docs/project-roadmap.md)
- [课程与项目映射](docs/course-map.md)
- [v0.1 里程碑](docs/milestones/v0.1.md)
- [M1-P01 开发说明](docs/milestones/v0.1-m1-p01.md)
- [M1-P02 开发说明](docs/milestones/v0.1-m1-p02.md)
- [M1-P02 实践证据](docs/evidence/M1-P02-multiturn-streaming.md)
- [M1-P03 开发说明](docs/milestones/v0.1-m1-p03.md)
- [专业技术内容积累笔记](docs/notes/README.md)
- [实践证据规范](docs/evidence/README.md)
- [项目边界 ADR](docs/adr/0001-course-project-boundary.md)

## 运行 M1-P03

当前实现使用 OpenAI 官方 Go SDK v3 的 Chat Completions API，并兼容支持该协议的模型服务商。运行时必须显式配置 Base URL、API Key 和模型名称，代码不会默认绑定某个模型服务商。

```bash
export OPSPILOT_BASE_URL="https://provider.example.com/v1"
export OPSPILOT_API_KEY="你的 API Key"
export OPSPILOT_MODEL="当前可用模型名"

go run ./cmd/opspilot "Go 服务请求延迟突然升高"
```

默认情况下，每轮成功调用都会将当前会话保存为 JSON 文件。目录和文件权限分别限制为 `0700` 和 `0600`，记录不包含 API Key、Authorization Header、Base URL 或其他运行配置：

```text
~/.opspilot/sessions/session-*.json
```

如果本次诊断不希望落盘，可以关闭会话保存：

```bash
go run ./cmd/opspilot diagnose --no-session "Go 服务请求延迟突然升高"
```

会话记录包含系统消息、用户消息、已完成的 assistant 回复、每轮 usage 和累计 usage。流式请求中断或取消时，不会把半截 assistant 回复提交到会话历史；保存失败只提示警告，不影响当前诊断结果。

第一轮回答结束后，程序会继续等待补充信息。输入 `/exit`、`/quit` 或发送 EOF 可退出当前会话：

```text
> Go 服务请求延迟突然升高
模型流式输出……

> 数据库连接池等待时间也升高
模型结合前文继续分析……

> /exit
```

`OPSPILOT_TIMEOUT` 是可选配置，未设置时默认为 `60s`：

```bash
export OPSPILOT_TIMEOUT="30s"
```

Token usage 仅表示 Provider 返回的本轮统计，不等同于费用结算。每轮回答结束后显示本轮统计；退出会话时显示所有已完成轮次的累计统计。多轮会话只要有一轮 usage 未知，累计 usage 就明确提示“统计不完整”。

运行本地验证：

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/opspilot
```

## 版本路线

| 版本 | 课程阶段 | 主要能力 |
|---|---|---|
| `v0.1` | 大模型与 API 基础 | 单次、多轮、流式故障分析 CLI、usage、会话保存和错误分类 |
| `v0.2` | Prompt 与结构化输出 | 诊断 Schema、Prompt 版本和回归样本 |
| `v0.3` | 本地模型与路由 | 本地 / 云端 Provider、路由和降级 |
| `v0.4` | RAG | Runbook、历史事故、引用和拒答 |
| `v0.5` | Agent 与工作流 | 只读证据收集、有限步骤和审计 |
| `v0.6` | AI 应用工程化 | HTTP 服务、可靠性、成本、安全和观测 |
| `v0.7` | MCP | 对外开放被授权的诊断能力 |
| `v0.8` | Skill | 沉淀可测试的领域排障能力 |
| `v1.0` | 产品化 | React 工作台、权限、部署和运维 |
