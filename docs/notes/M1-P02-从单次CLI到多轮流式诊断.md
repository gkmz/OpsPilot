# M1-P02：从单次 CLI 到多轮流式诊断

> 本篇记录 OpsPilot 从一次性模型调用演进到多轮流式诊断的过程。重点不是展示最终代码，而是还原开发工程中遇到的问题、定位方式、修复过程和最终取舍。

## 一、从单次 CLI 开始的原因

OpsPilot 的第一个目标不是马上实现 Agent，而是先建立一条可以测试、可以解释的模型调用链路：

```text
命令行输入
  → 读取配置
  → 构造诊断消息
  → 调用 OpenAI 兼容端点
  → 输出模型回答
```

单次非流式调用适合作为基线，因为它先固定了配置、鉴权、Base URL、请求结构、响应结构和错误语义。这个阶段还没有会话和流式输出，但已经可以验证模型客户端是否真正可用。

然而，故障排查很少只需要一个问题。用户可能先描述“接口延迟升高”，模型提出需要连接池指标，用户再补充“数据库连接池等待时间也升高”。如果第二次请求没有携带第一次的消息，模型就无法理解补充信息对应的上下文。

## 二、P02 的目标与边界

P02 增加四项能力：

- 同一进程内的多轮消息历史；
- SSE 流式增量输出；
- 模型请求和交互输入的取消语义；
- 失败时不把半截 assistant 写入历史。

P02 明确不实现：

- 退出程序后的会话恢复；
- 自动重试、限流和熔断；
- 多 Provider 路由；
- RAG、Agent、MCP；
- OpenAI SDK 迁移。

把边界写清楚很重要。否则“做多轮对话”很容易膨胀成数据库、重试策略和生产可靠性治理，反而无法验证当前最核心的消息和流式控制流。

## 三、多轮会话保存的不是 HTTP 连接

多轮对话不要求一直保持一条 HTTP 连接。每轮请求都可以独立完成：

```text
CLI 进程
  ├─ 创建 Conversation
  ├─ 第 1 轮 HTTP 请求，结束后关闭响应体
  ├─ 第 2 轮 HTTP 请求，结束后关闭响应体
  └─ 用户退出后销毁 Conversation
```

第二轮请求能够理解第一轮，是因为客户端重新发送完整消息历史：

```text
system: 你是一个 Go HTTP 微服务故障分析助手
user: 服务请求延迟突然升高
assistant: 第一轮完整分析
user: 数据库连接池等待时间也升高
```

因此要保存的是消息状态，而不是网络连接。

P02 的 `Conversation` 只保存在进程内存中：

```text
启动程序 → Conversation 存在于内存 → 进程退出 → 会话消失
```

如果要求退出后恢复，就要引入 JSON 文件、SQLite 或数据库，同时处理脱敏、版本和并发写入。这属于 P03。

## 四、命令行参数如何进入第一轮

交互模式有两种输入来源：

- 命令行参数：第一轮问题；
- 标准输入：第二轮及之后的补充问题。

例如：

```bash
go run ./cmd/opspilot "Go 服务请求延迟突然升高"
```

期望流程是：

```text
args
  → initialSymptom
  → CommitUser
  → Stream
  → CommitAssistant
  → 等待 stdin 后续输入
```

这里遇到的第一个实际问题是，原来的 `readSymptom(args, stdin)` 会在没有参数时调用 `io.ReadAll(stdin)`。一次性读取适合 P01，却不适合交互模式，因为它会把标准输入全部消费掉，后续 `bufio.Scanner` 没有输入可读。

因此交互模式要直接处理参数：

```go
initialSymptom := strings.TrimSpace(strings.Join(args, " "))
```

然后把 stdin 留给交互循环。

## 五、Conversation 的值接收者陷阱

会话对象内部保存消息切片：

```go
type Conversation struct {
	messages []llm.Message
}
```

最初很容易写成：

```go
func (c Conversation) CommitUser(content string) {
	c.messages = append(c.messages, llm.Message{
		Role:    "user",
		Content: content,
	})
}
```

这里的 `Conversation` 是值接收者。方法拿到的是对象副本。即使切片暂时共享底层数组，`append` 更新的也是副本中的切片长度，原对象后续读取时可能看不到新增消息。

修复为指针接收者：

```go
func (c *Conversation) CommitUser(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	c.messages = append(c.messages, llm.Message{
		Role:    "user",
		Content: content,
	})
}
```

`CommitAssistant` 也必须使用指针接收者。这个问题只有在下一轮读取历史时才会暴露，所以要用消息数量和顺序测试锁定行为。

## 六、为什么 Messages 返回副本

如果直接返回内部切片：

```go
func (c *Conversation) Messages() []llm.Message {
	return c.messages
}
```

调用方就能绕过会话对象修改内部状态：

```go
messages := chat.Messages()
messages[0].Content = "外部修改"
```

当前实现返回副本：

```go
func (c *Conversation) Messages() []llm.Message {
	messages := make([]llm.Message, len(c.messages))
	copy(messages, c.messages)
	return messages
}
```

这会产生一次小的复制成本，但能保护会话封装。当前模型请求的主要成本在网络和推理，不应该为了消除一次切片复制而暴露内部状态。

## 七、从完整响应到增量响应

非流式响应可以使用：

```go
responseBody, err := io.ReadAll(resp.Body)
```

流式响应则会在一段时间内持续到达。如果仍使用 `io.ReadAll`，客户端必须等响应体关闭后才开始解析，用户看不到实时输出。

SSE 流式响应通常是多条事件：

```text
data: {"choices":[{"delta":{"content":"建议"}}]}

data: {"choices":[{"delta":{"content":"检查连接池"}}]}

data: [DONE]
```

非流式内容通常位于：

```text
choices[0].message.content
```

流式内容通常位于：

```text
choices[0].delta.content
```

这也是为什么不能直接复用非流式响应结构。

客户端使用 `bufio.Scanner` 逐行读取：

```go
scanner := bufio.NewScanner(resp.Body)
scanner.Buffer(make([]byte, 64*1024), 1<<20)
```

每行读取后过滤空行、解析 `data:`，遇到普通 JSON 就提取增量内容，遇到 `[DONE]` 才标记完整结束。

## 八、请求头与 http.Flusher 的误用

客户端发送的是 JSON：

```http
Content-Type: application/json
```

客户端希望接收 SSE：

```http
Accept: text/event-stream
```

把请求的 `Content-Type` 设置为 `text/event-stream` 是错误的，因为它描述的是发送内容，不是接收内容。

另一个容易出现的错误是：

```go
flusher, ok := req.(http.Flusher)
```

`http.Flusher` 是服务端 `http.ResponseWriter` 的能力，用于服务端刷新响应。客户端接收流式响应时只需要读取 `resp.Body`，不能把 `*http.Request` 转成 `http.Flusher`。

## 九、Stream 接口为什么使用回调

当前客户端接口是：

```go
func (c *Client) Stream(
	ctx context.Context,
	messages []Message,
	onChunk func(string) error,
) (Usage, error)
```

每收到一个增量片段，就调用一次 `onChunk`：

```text
llm.Stream
  → onChunk("建议")
  → CLI 输出
  → onChunk("检查连接池")
  → CLI 继续输出
```

这样 `internal/llm` 不需要知道 stdout、文件或其他输出目标。协议解析留在 LLM 层，输出策略留在应用层。回调返回错误时，客户端可以立即停止读取并把错误传回上层。

## 十、assistant 消息为什么延迟提交

流式过程中，终端需要实时输出，但会话历史需要保持一致。因此应用层同时维护输出和临时缓冲：

```go
var content strings.Builder
```

每个 chunk：

```text
chunk
  ├─ 立即写入 stdout
  └─ 追加到 strings.Builder
```

只有 `Stream` 收到 `[DONE]` 并成功返回后，才执行：

```go
conversation.CommitAssistant(content.String())
```

如果在每个 chunk 到达时直接提交，下一轮可能携带多条半截消息：

```text
assistant: 建议检查
assistant: 数据库连接
assistant: 池状态
```

正确历史应该只有一条完整 assistant 消息。

现实中的取舍是：终端中已经显示的半截文本无法可靠回滚，但内部会话历史不能保存半截回答。显示状态和会话状态必须分开管理。

## 十一、失败请求如何影响下一轮

当前 `runTurn` 的顺序是：

```text
CommitUser
  → Stream
  → 成功后 CommitAssistant
```

如果 `Stream` 返回断流、HTTP 错误或非法事件：

- 用户消息已经进入历史；
- 已输出的部分文本保留在终端；
- assistant 不进入历史；
- 普通错误允许继续下一轮。

如果错误实际来自已取消的 context，则不能继续下一轮：

```go
if err := runTurn(...); err != nil {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	fmt.Fprintf(output, "请求失败: %v\n", err)
	continue
}
```

这里区分了两种错误层级：普通模型错误影响当前轮，context 取消表示整个运行意图结束。

## 十二、开发中遇到的典型问题

### 1. `ChatSteaming` 拼写错误

公开方法命名错误会扩散到调用方。最终使用更简单的 `Stream`，也避免把业务接口绑定在某个具体协议名称上。

### 2. `streaming` 不是请求字段

正确字段是：

```json
{"stream": true}
```

而不是：

```json
{"streaming": true}
```

字段错误时，服务端可能静默按非流式处理，客户端却按 SSE 解析，最终表现为协议错误。

### 3. EOF 被误判为空行

`bufio.Scanner` 的结果中：

```text
空行：Scan() == true，Text() == ""
EOF： Scan() == false，Err() == nil
```

如果只传递文本和错误，二者都会变成空字符串和 nil。交互层把 EOF 当成空行并 `continue`，就可能产生无限扫描循环。

因此 `scanResult` 需要保存 `Scan()` 的布尔值：

```go
type scanResult struct {
	line string
	err  error
	ok   bool
}
```

### 4. 命令行参数第一轮失败后直接退出

交互模式中，普通模型错误不应该结束整个会话。修复后，初始参数请求失败会显示错误并继续等待 stdin；只有 context 被取消时才退出。

## 十三、测试策略

真实模型不适合覆盖全部自动化场景，因为响应不可完全控制，也难以稳定模拟断流和取消。P02 使用 `httptest.NewServer` 构造本地 OpenAI 兼容假服务。

LLM 层测试覆盖：

- `stream: true`；
- JSON 请求头和 SSE 接收头；
- 多个 `data:` 事件；
- `delta.content` 增量文本；
- `[DONE]`；
- 提前 EOF；
- 非法 JSON；
- HTTP 429；
- 回调错误；
- context 取消。

应用层测试覆盖：

- 命令行参数自动作为第一轮；
- 第二轮携带完整历史；
- 失败后不携带半截 assistant；
- EOF 不被当成空行死循环；
- 普通错误后仍可继续交互。

测试重点不是模型回答质量，而是协议边界、消息顺序和状态一致性。

## 十四、验证与边界

P02 收尾执行：

```bash
gofmt -w cmd internal
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/opspilot
git diff --check
```

这些命令分别验证代码格式、功能、数据竞争、常见可疑写法、CLI 构建和差异质量。

当前实现证明了：

- 进程内多轮消息可以正确传递；
- SSE 增量事件可以实时输出；
- 断流和取消不会提交半截 assistant；
- 普通错误不会破坏后续交互。

当前没有证明：

- 真实 Provider 的所有兼容行为；
- 真实模型的诊断质量；
- 退出程序后的会话恢复；
- 生产环境的吞吐量、成本和可靠性治理。

## 对应代码与证据

- 会话：`internal/conversation/conversation.go`
- 交互编排：`internal/app/run.go`
- 流式客户端：`internal/llm/client.go`
- 信号入口：`cmd/opspilot/main.go`
- 测试：`internal/llm/client_test.go`、`internal/app/run_test.go`、`internal/conversation/conversation_test.go`、`cmd/opspilot/main_test.go`
- 验证证据：`docs/evidence/M1-P02-multiturn-streaming.md`
- 里程碑：`docs/milestones/v0.1-m1-p02.md`

## 总结

从单次 CLI 到多轮流式诊断，真正增加的不是一个 `Stream` 方法，而是一套新的状态和控制流：

```text
持续运行的进程
  → 受保护的消息历史
  → 逐事件读取的 SSE 客户端
  → 可取消的请求和输入流程
  → 完整成功后提交 assistant
  → 假服务覆盖失败路径
```

这条链路为后续 usage、会话持久化、错误分类和更复杂的模型能力提供了可靠基础。
