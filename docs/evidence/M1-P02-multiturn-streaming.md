# 实践证据：M1-P02 多轮与流式诊断

## 实践问题

验证 OpsPilot 是否能够在同一个 CLI 进程中完成多轮故障补充，通过 SSE 增量输出模型回答，并在取消、断流、HTTP 错误或非法事件时保持会话历史一致。

## 环境

- 日期：2026-08-29
- 操作系统：macOS / Apple Silicon
- Go 版本：`go1.27.0 darwin/arm64`
- Provider / 模型：本地 `httptest` OpenAI 兼容假服务
- 对应分支：`feature/m1-p02-multiturn-streaming`

## 实现范围

- 命令行参数作为第一轮问题立即发送，不需要用户再次输入。
- 程序在第一轮完成后保持运行，继续从标准输入读取补充信息。
- 会话历史包含 `system`、`user` 和完整的 `assistant` 消息。
- 流式客户端逐行解析 SSE 的 `data:` 事件和 `choices[].delta.content`。
- 只有收到 `[DONE]` 后，完整 assistant 消息才会提交到会话历史。
- 流式请求支持上下文取消，并关闭 HTTP 响应体。
- 断流、HTTP 非 2xx、非法 JSON 和输出回调错误均返回明确错误。

## 自动化验证

执行命令：

```bash
gofmt -w cmd internal
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/opspilot
git diff --check
```

验证结果：全部通过。

## 核心场景

### 正常流式输出

`TestClientStreamReceivesChunksAndSendsStreamFlag` 验证：

- 请求体包含 `stream: true`；
- 请求使用 `Content-Type: application/json`；
- 客户端声明接收 `text/event-stream`；
- 多个增量片段按顺序交给回调；
- 收到 `[DONE]` 后正常结束；
- 兼容端点返回 usage 时可以正确读取。

### 多轮消息历史

`TestRunInteractiveSendsFollowUpWithHistory` 验证第二轮请求包含：

```text
system
user: 第一轮
assistant: 第一轮完整回答
user: 第二轮
```

### 断流与失败回滚

`TestClientStreamRejectsUnexpectedEOF` 验证服务端未发送 `[DONE]` 就关闭连接时返回断流错误。

`TestRunInteractiveDoesNotCommitPartialAssistant` 验证第一轮输出部分内容后断流，第二轮消息历史只包含两轮用户消息，不包含半截 assistant 消息，并且交互会话可以继续。

### 上下文取消

`TestClientStreamCanBeCanceled` 使用可刷新假服务发送首个片段后取消上下文，验证流式调用及时返回并且错误可以通过 `errors.Is(err, context.Canceled)` 识别。

### HTTP、协议和输出错误

- `TestClientStreamReturnsHTTPError` 验证 HTTP `429` 错误。
- `TestClientStreamRejectsInvalidEvent` 验证非法流式 JSON。
- `TestClientStreamReturnsCallbackError` 验证终端输出或其他消费端失败时停止读取并返回原始错误链。

## 结论与边界

- 本次证明了进程内多轮会话、流式增量输出、取消、断流识别和失败回滚可以稳定组合。
- 会话当前只保存在内存中，CLI 退出后不会保留；持久化属于 `M1-P03`。
- 流式 usage 只在兼容端点主动返回时可用，未返回时保持 `Known=false`。
- 本阶段不实现重试、限流、熔断、Provider 路由、RAG、Agent 或 MCP。
