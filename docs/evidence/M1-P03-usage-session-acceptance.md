# 实践证据：M1-P03 usage、会话和错误分类

## 实践问题

验证 OpsPilot 是否能在多轮流式诊断中记录 usage、保存脱敏会话、区分主要错误类别，并在保存失败、取消和断流时保持结果与会话历史边界正确。

## 环境

- 日期：2026-09-01
- 操作系统：macOS / Apple Silicon
- Go 版本：`go1.27.0 darwin/arm64`
- Provider / 模型：本地 `httptest` OpenAI 兼容假服务
- SDK：`github.com/openai/openai-go/v3 v3.54.0`
- 对应分支：`feature/m1-p03-usage-session-acceptance`

## 实现范围

- 非流式和流式 Chat Completions 都返回项目自己的 `llm.Usage`。
- Provider 未返回 usage 时使用 `Known=false`，不把零值当作准确统计。
- Conversation 保存每轮 usage 和累计 usage，并通过快照交给 session 层。
- 成功轮次保存 JSON 会话；默认路径为 `~/.opspilot/sessions`，可用 `OPSPILOT_SESSION_DIR` 覆盖。
- `diagnose --no-session` 关闭本次会话持久化。
- 会话目录和文件权限分别为 `0700`、`0600`，记录不包含 API Key、Authorization Header 或 Base URL。
- 配置、网络、HTTP、协议、取消、输出和存储错误保留底层 cause。

## 自动化验证

执行命令：

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/opspilot
git diff --check
```

验证结果：全部通过。

## 核心场景

### usage 与多轮累计

- `TestClientChat` 验证非流式输入、输出和总 Token。
- `TestClientStreamReceivesChunksAndSendsStreamFlag` 验证流式 usage 和增量输出。
- `TestClientChatMarksMissingUsageUnknown` 验证缺失 usage 时 `Known=false`。
- `TestConversationAccumulatesKnownUsage`、`TestConversationMarksAccumulatedUsageUnknown` 验证每轮和累计 usage。
- `TestRunReadsSymptomFromArgument` 验证保存记录包含消息、每轮 usage 和累计 usage。

### 会话安全保存

- `TestStoreSavesAndLoadsRecord` 验证会话文件可以保存和读取。
- `TestStoreUsesRestrictedFilePermissions` 验证文件权限为 `0600`。
- `TestRecordJSONContainsOnlySessionData` 验证记录不包含凭据字段。
- `TestRunInteractiveReportsSaveFailureWithoutDiscardingResult` 验证保存失败只产生可理解警告，不丢弃模型结果。
- `TestDiagnoseSupportsDisablingSessionPersistence` 验证 CLI 提供 `--no-session` 选项。

### 错误与失败边界

- `TestClientStreamCanBeCanceled` 验证取消错误可识别。
- `TestClientStreamRejectsUnexpectedEOF` 验证未收到 `[DONE]` 的断流不会被当作成功。
- `TestClientStreamReturnsHTTPError`、`TestClientStreamRejectsInvalidEvent` 验证 HTTP 和协议错误。
- `TestRunInteractiveDoesNotCommitPartialAssistant` 验证失败轮次不提交半截 assistant 消息。

## 失败与修复

验收前检查发现默认 session 路径使用 `os.UserConfigDir()`，macOS 实际会落到 `~/Library/Application Support`，与产品约定的 `~/.opspilot` 不一致。已改为使用 `os.UserHomeDir()`，并增加 `TestNewDefaultStoreUsesHomeDirectory` 防止回归。

随后补充 CLI 关闭保存选项时，新增测试先按预期失败，暴露 `diagnose` 缺少 `--no-session`；实现参数和 app 存储注入入口后，定向测试恢复通过。

## 结论与边界

- 本次证明了 usage、会话快照、安全保存、错误分类和失败轮次边界可以组合工作。
- 本阶段只保存本地 JSON，不提供历史会话列表、删除、恢复到新进程或跨设备同步。
- usage 是 Provider 统计展示，不是成本结算系统；Provider 不返回 usage 时只能展示未知。
- session 保存发生在成功轮次之后；保存失败不影响当前进程内对话，但需要用户查看警告。
- 本阶段不实现自动重试、限流、熔断、Provider 路由、RAG、Agent、MCP、自动修复或 React 工作台。

## 文章关联

- 知识库文章：`docs/notes/M1-P03-usage-session-acceptance.md`
- 文章状态：草稿
