# M1-P03：usage、会话和错误分类实践复盘

## 学习单元

M1-P03：在多轮流式模型调用之上，补齐 usage、会话持久化、错误分类和 v0.1 验收闭环。

## 关键决策

### usage 分为单轮和会话累计

单轮 usage 属于一次模型调用，适合在每轮回答结束后立即展示；会话累计 usage 属于当前 Conversation 的聚合结果，适合复盘整个诊断过程。两者不能混为一个字段，否则无法解释某一轮消耗，也无法识别未知统计如何影响累计结果。

当任意轮次没有可信 usage 时，累计结果传播 `Known=false`。数值字段仍可以保留 Provider 已返回的部分信息，但不能被当作完整统计。

交互循环使用统一的退出收口逻辑，从 Conversation 读取累计 usage。这样 `/exit`、`/quit`、EOF 和上下文取消不需要分别维护输出代码；没有成功完成任何轮次时不打印空统计。

### 保存完整成功状态，不保存半截回答

流式输出期间可以已经向终端写出部分文本，但只有收到完整结束信号并且客户端成功返回后，才把 assistant 消息提交到 Conversation。这样保存的 session 与后续请求历史保持一致，不会把失败过程中的半截回答当成事实。

### 存储边界独立于模型调用

Conversation 只负责内存中的消息和 usage 聚合，session 负责快照记录、脱敏边界和本地文件写入。保存失败通过警告反馈给用户，不回滚已经完成的模型结果，也不让存储细节侵入 LLM 适配层。

### 默认目录选择用户主目录

产品约定默认目录为 `~/.opspilot/sessions`，因此使用 `os.UserHomeDir()` 比平台配置目录更符合预期。部署或测试环境可以通过 `OPSPILOT_SESSION_DIR` 显式覆盖。

## 错误分类经验

错误分类的目标不是替换底层错误，而是增加稳定的业务类别，同时通过 `Unwrap` 保留 cause。这样 CLI 可以展示“网络错误”或“协议错误”，测试仍可以使用 `errors.Is`、`errors.As` 识别取消、HTTP API 错误和底层网络错误。

## 当前限制

- 本地 session 目前只有保存和读取接口，没有历史列表、删除和恢复命令。
- `--no-session` 只影响当前运行，不改变默认配置。
- usage 依赖 Provider 返回；未知 usage 不进行估算。
- 暂不实现重试、限流、熔断、Provider 路由和成本结算。

## 对应证据

- `docs/evidence/M1-P03-usage-session-acceptance.md`
- `internal/app/run_test.go`
- `internal/llm/client_test.go`
- `internal/session/store_test.go`
