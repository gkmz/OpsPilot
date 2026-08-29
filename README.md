# OpsPilot

OpsPilot 是一个面向 Go HTTP 微服务的研发故障诊断与处置助手，也是“大模型应用开发实战”课程的持续实践项目。

它逐步使用日志、Git 变更、Markdown Runbook 和历史事故样本，帮助开发者形成带证据的诊断结论和排查步骤。项目默认只读；没有有效证据时不输出确定结论；高风险操作必须经过人工确认。

## 当前阶段

当前处于 `v0.1` 起步阶段，正在实现 `M1-P01`，目标是先建立一个可测试的 Go CLI 单次调用骨架：

- 输入故障现象并获得单次分析；
- 后续支持多轮补充信息；
- 后续支持可取消的流式输出；
- 后续记录模型 usage；
- 后续保存可以复查的会话和运行证据。

`v0.1` 不包含 RAG、Agent、MCP、自动修复和生产级可靠性治理。

## 项目文档

- [课程与项目映射](docs/course-map.md)
- [v0.1 里程碑](docs/milestones/v0.1.md)
- [M1-P01 开发说明](docs/milestones/v0.1-m1-p01.md)
- [M1-P02 开发说明](docs/milestones/v0.1-m1-p02.md)
- [实践证据规范](docs/evidence/README.md)
- [项目边界 ADR](docs/adr/0001-course-project-boundary.md)

## 运行 M1-P01

当前实现使用 OpenAI 兼容的 `chat/completions` 协议。运行时必须显式配置 Base URL、API Key 和模型名称，代码不会默认绑定某个模型服务商。客户端会在 Base URL 后追加 `/chat/completions` 发起请求。

```bash
export OPSPILOT_BASE_URL="https://provider.example.com/v1"
export OPSPILOT_API_KEY="你的 API Key"
export OPSPILOT_MODEL="当前可用模型名"

go run ./cmd/opspilot "Go 服务请求延迟突然升高"
```

`OPSPILOT_TIMEOUT` 是可选配置，未设置时默认为 `60s`：

```bash
export OPSPILOT_TIMEOUT="30s"
```

运行本地验证：

```bash
go test ./...
go vet ./...
```

## 版本路线

| 版本 | 课程阶段 | 主要能力 |
|---|---|---|
| `v0.1` | 大模型与 API 基础 | 单次、多轮、流式故障分析 CLI |
| `v0.2` | Prompt 与结构化输出 | 诊断 Schema、Prompt 版本和回归样本 |
| `v0.3` | 本地模型与路由 | 本地 / 云端 Provider、路由和降级 |
| `v0.4` | RAG | Runbook、历史事故、引用和拒答 |
| `v0.5` | Agent 与工作流 | 只读证据收集、有限步骤和审计 |
| `v0.6` | AI 应用工程化 | HTTP 服务、可靠性、成本、安全和观测 |
| `v0.7` | MCP | 对外开放被授权的诊断能力 |
| `v0.8` | Skill | 沉淀可测试的领域排障能力 |
| `v1.0` | 产品化 | React 工作台、权限、部署和运维 |
