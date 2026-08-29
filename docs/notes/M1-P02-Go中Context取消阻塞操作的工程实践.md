# M1-P02：Go 中 Context 取消阻塞操作的工程实践

> 在流式 CLI 中，`context.Context` 不只是一个需要传递的参数。它决定了网络请求、用户输入、错误路径和程序退出能否形成一致的控制流。本篇从 OpsPilot 的实际问题出发，解释为什么“监听了 context”不等于“所有操作都可取消”。

## 一、问题是怎样出现的

为了支持 `Ctrl+C`，入口使用信号创建上下文：

```go
ctx, stop := signal.NotifyContext(
	context.Background(),
	os.Interrupt,
	syscall.SIGTERM,
)
defer stop()
```

应用层把它传给流式调用：

```go
client.Stream(ctx, messages, onChunk)
```

这条链路可以取消正在进行的 HTTP 请求，但交互程序还有另一个阻塞点：

```go
scanner.Scan()
```

如果用户没有输入，程序会停在那里等待 stdin。此时 context 即使已经取消，`scanner.Scan()` 也不会自动返回。

## 二、Context 的真实职责

Context 表达的是取消信号和生命周期边界，具体操作必须主动配合。判断一个函数是否需要 context，关键不是它是不是业务函数，而是它是否可能长时间等待外部资源。

| 操作 | 是否可能阻塞 | 当前取消方式 |
|---|---:|---|
| 读取环境变量 | 否 | 不需要 context |
| 修改内存会话 | 否 | 不需要 context |
| HTTP 请求 | 是 | `NewRequestWithContext` |
| 流式响应读取 | 是 | 绑定 HTTP context |
| `scanner.Scan()` | 是 | goroutine + channel |
| 终端输出 | 通常否 | 依赖 Writer 行为 |

没有必要把 context 机械传给每一个函数，但所有可能阻塞的操作都要有明确的取消策略。

## 三、HTTP 流式请求为什么可以取消

创建请求时绑定上下文：

```go
req, err := http.NewRequestWithContext(
	ctx,
	http.MethodPost,
	url,
	body,
)
```

当上层调用 `cancel()`，HTTP 请求和响应体读取会收到取消信号。流式读取最终返回错误，客户端再检查：

```go
if ctx.Err() != nil {
	return usage, fmt.Errorf("流式请求已取消: %w", ctx.Err())
}
```

这样可以把底层网络错误归类为用户主动取消，而不是普通读取失败。

无论正常完成、HTTP 错误、协议错误还是取消，都必须关闭响应体：

```go
defer resp.Body.Close()
```

取消不仅要让函数返回，还要释放已经占用的 HTTP 资源。

## 四、为什么循环开头的 select 不够

下面的写法只能在进入 `scanner.Scan()` 之前检查一次：

```go
for {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if !scanner.Scan() {
		return nil
	}
}
```

实际执行过程是：

```text
select 检查 ctx
  → ctx 尚未取消
  → scanner.Scan() 阻塞
  → ctx 被取消
  → 代码仍然卡在 scanner.Scan()
```

`select` 不会在后台监控后续的同步函数调用。它只能等待 channel，不能把不支持 context 的同步函数自动变成可取消函数。

## 五、goroutine + channel 如何桥接阻塞读取

如果要同时等待用户输入和 context 取消，可以把阻塞读取放进 goroutine：

```go
type scanResult struct {
	line string
	err  error
	ok   bool
}

func scanLine(scanner *bufio.Scanner) <-chan scanResult {
	result := make(chan scanResult, 1)

	go func() {
		if scanner.Scan() {
			result <- scanResult{
				line: scanner.Text(),
				ok:   true,
			}
			return
		}

		result <- scanResult{err: scanner.Err()}
	}()

	return result
}
```

主流程同时等待两个事件：

```go
result := scanLine(scanner)

select {
case <-ctx.Done():
	return ctx.Err()
case scan := <-result:
	if scan.err != nil {
		return scan.err
	}
	if !scan.ok {
		return nil
	}
	// 处理 scan.line
}
```

这里不是让 `Scan` 本身获得了取消能力，而是让主 goroutine 不再直接等待 `Scan` 返回，转而等待一个可以参与 `select` 的结果 channel。

## 六、`scanResult.ok` 为什么必要

`bufio.Scanner` 的返回信息如下：

| 情况 | `Scan()` | `Text()` | `Err()` |
|---|---:|---|---|
| 普通输入 | `true` | 非空文本 | `nil` |
| 空行 | `true` | `""` | `nil` |
| EOF | `false` | `""` | `nil` |
| 读取错误 | `false` | `""` | 非 `nil` |

空行和 EOF 都可能产生空字符串，但语义不同：

```text
空行：继续等待下一次输入
EOF：结束交互循环
```

如果只传递 `line` 和 `err`，它们都会表现为：

```go
line == ""
err == nil
```

交互层把 EOF 当成空行并执行 `continue`，就会不断重复扫描，形成死循环。保存 `Scan()` 的布尔结果后，输入状态和输入内容就能被分开表达：

```text
ok=true：确实读到了一行，即使这一行为空
ok=false 且 err=nil：EOF
ok=false 且 err!=nil：读取错误
```

## 七、取消后为什么不能继续下一轮

交互循环需要区分普通错误和取消：

```go
if err := runTurn(...); err != nil {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	fmt.Fprintf(output, "请求失败: %v\n", err)
	continue
}
```

普通 HTTP 错误、非法事件或服务端断流只影响当前轮，用户仍可能继续输入；但 context 已取消表示整个运行意图结束，不能打印错误后继续回到输入循环。

## 八、主流程退出不等于 Reader 可强制取消

goroutine + channel 方案解决了主流程的及时响应，但有一个必须记录的限制：如果 context 先取消，主流程可以返回，阻塞在 `scanner.Scan()` 的 goroutine 可能仍要等待底层 Reader 返回。

容量为 1 的 channel 可以避免 goroutine 发送结果时因为主流程退出而永久阻塞：

```go
result := make(chan scanResult, 1)
```

但它不能强行关闭任意 `io.Reader`。对于当前 CLI，主进程退出后该 goroutine 会随进程结束；如果未来把交互循环嵌入长期运行的服务，就需要使用可关闭的 Reader，或者设计原生支持 context 的输入接口。

因此：

```text
主流程可及时退出 ≠ 任意底层 Reader 都能被强制取消
```

## 九、取消与会话状态一致性

取消发生在流式回答中间时，终端可能已经显示部分内容：

```text
终端：已经显示 partial
会话：不能保存 partial assistant
```

所以 `runTurn` 使用临时 `strings.Builder`，只有 `Stream` 成功返回后才提交 assistant。取消错误返回时：

```text
不提交 assistant
退出交互循环
释放响应体
```

取消处理不只是让函数返回，还要保证业务状态没有被不完整结果污染。

## 十、如何测试取消行为

一个可靠的取消测试需要控制时序：

```text
假服务发送第一个 chunk
  → flush 到客户端
  → 测试确认已经收到 chunk
  → cancel()
  → 等待 Stream 返回
```

如果刚启动请求就立即取消，测试可能只验证了请求尚未开始时的取消，而没有验证正在读取流时的行为。

当前 P02 使用本地 `httptest` 服务和 `go test -race`，验证请求取消、消息回滚和并发访问没有数据竞争。

## 十一、常见错误认识

### 错误一：有 context 参数就支持取消

不对。必须确认 context 是否真正传入了 HTTP 请求、响应读取或其他阻塞操作。

### 错误二：循环开头检查一次就够了

不对。检查只能覆盖检查时刻，不能中断之后开始的同步阻塞调用。

### 错误三：goroutine 能取消所有阻塞操作

不对。goroutine 只是让主流程可以选择返回，底层 Reader 仍可能持续阻塞。

### 错误四：取消后保存半截回答方便排查

不建议直接把半截回答放进正常 assistant 历史。失败尝试可以作为独立运行记录保存，但不能伪装成完整对话消息。

## 十二、可迁移的设计准则

1. 为每个可能阻塞的操作明确取消策略；
2. 不要只把 context 传到最底层，还要检查上层错误分支；
3. 区分普通失败和整个运行被取消；
4. 把输入状态、输入内容和读取错误分开表达；
5. 对 goroutine 方案记录底层 Reader 的生命周期限制；
6. 用时序可控的假服务测试取消，而不是只测试返回错误；
7. 取消后不仅要停止工作，还要检查是否留下错误的业务状态。

## 对应代码与证据

- 信号入口：`cmd/opspilot/main.go`
- 交互读取：`internal/app/run.go`
- HTTP 取消：`internal/llm/client.go`
- 测试：`internal/llm/client_test.go`、`cmd/opspilot/main_test.go`、`internal/app/run_test.go`
- 证据：`docs/evidence/M1-P02-multiturn-streaming.md`

## 总结

Context 的工程价值不在于把它作为参数传遍所有函数，而在于建立一条可解释的生命周期链路：

```text
信号触发取消
  → 阻塞操作感知取消
  → 上层识别取消而非普通错误
  → 停止继续调度新工作
  → 不提交不完整状态
  → 释放已占用资源
```

这套思路不仅适用于流式模型调用，也适用于数据库查询、消息消费、文件传输和外部命令执行。
