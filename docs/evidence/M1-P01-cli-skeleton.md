# 实践证据：M1-P01 单次故障分析 CLI 骨架

## 实践问题

在加入多轮和流式输出前，先验证 OpsPilot 是否能用清晰的模块边界完成一次可测试的故障分析请求，并对配置错误、HTTP 错误和协议错误给出明确反馈。

## 环境

- 日期：2026-08-11
- 操作系统：macOS / Apple Silicon
- Go 版本：`go1.27.0 darwin/arm64`
- Provider / 模型：本地 `httptest` 假服务；真实兼容端点已调用成功，Provider 和模型待脱敏登记
- 对应 commit / tag：初始基线 `29f78b79ccd67c7845556363d8bfd7bb95f9fe99`；`course-m1-p01` 待创建

## 输入与前置条件

本地测试使用以下模拟输入：

```text
服务 延迟
```

假服务返回 OpenAI 兼容的非流式 `chat/completions` JSON。测试只验证协议和应用组装，不验证真实模型的诊断质量。

## 执行命令

```bash
gofmt -w cmd internal
go test -race ./...
go vet ./...
go build ./cmd/opspilot
```

## 结果

```text
?    github.com/gkmz/OpsPilot/cmd/opspilot          [no test files]
ok   github.com/gkmz/OpsPilot/internal/app
ok   github.com/gkmz/OpsPilot/internal/config
ok   github.com/gkmz/OpsPilot/internal/diagnosis
ok   github.com/gkmz/OpsPilot/internal/llm

=== RUN   TestRunReadsSymptomFromArgument
--- PASS: TestRunReadsSymptomFromArgument (0.00s)
PASS
```

已验证：

- CLI 可以从命令行参数读取故障描述；
- 配置从环境变量读取，不在代码中保存密钥；
- 请求发送到可配置的 OpenAI 兼容端点；
- System Prompt 明确“初步分析”和证据缺口；
- 客户端可以解析模型文本和可选 usage；
- 非 2xx 响应会返回包含状态码的错误；
- HTTP 200 但 `choices` 为空时会返回明确的协议错误；
- 空输入、缺少配置、非法 URL 和非法超时有测试或校验。

## 真实端点验证

作者已使用真实 OpenAI 兼容端点完成一次 CLI 调用，大模型成功返回，说明 Base URL、远程鉴权、模型名称、请求编码和响应解析可以组成一条可运行链路。

以下信息尚未登记，发布文章前必须补齐：

- Provider 名称和模型名称；
- 脱敏后的执行命令；
- 不包含敏感信息的响应节选；
- usage 是否返回，以及返回时的 Token 数；
- 真实调用中遇到的错误和修复过程（如果有）。

在这些信息补齐前，本节只能作为作者确认，不能视为第三方可以复查的完整运行证据。API Key 不进入 evidence、文章、日志或截图。

## 失败与修复

第一版配置读取逻辑在 `OPSPILOT_TIMEOUT` 非法时静默使用默认值。这会让用户误以为自定义超时已经生效，也不利于文章解释真实配置。

修复后 `LoadFromEnv` 显式返回错误，非法值会得到类似以下提示：

```text
OPSPILOT_TIMEOUT 无效: "not-duration"
```

并增加 `TestLoadFromEnvRejectsInvalidTimeout` 防止回归。

## 结论与边界

- 本次证明了什么：目录边界、配置、单次非流式协议和错误路径可以在不访问真实模型的情况下完成自动测试。
- 本次没有证明什么：真实调用尚缺少可复查细节，也没有验证诊断质量、多轮、流式、取消和会话持久化。
- 对 OpsPilot 设计的影响：模块一先使用一个可配置的 OpenAI 兼容端点，不提前实现模块三的 Provider 路由。
- 可以进入文章的结论：一个 API Demo 进入项目时，首先需要钉住配置、输入输出、Prompt 边界和错误语义，而不是先堆更多模型能力。

## 文章关联

- 知识库文章：`17-OpsPilot实战01：从空仓库到可测试的故障分析CLI.md`
- 文章状态：审校中
