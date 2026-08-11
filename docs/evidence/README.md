# 实践证据规范

`docs/evidence/` 保存课程文章可以引用的工程事实。它不是随手日志，也不保存密钥、真实用户数据或未经脱敏的生产日志。

## 应保存的内容

- 学习与实践单元 ID；
- 运行环境和前置条件；
- 执行命令；
- 关键输出摘要；
- 测试结果；
- 可比较指标；
- 失败样本和修复结果；
- 对设计和文章结论的影响；
- 对应 commit 或 tag。

## 不应保存的内容

- API Key、Cookie、访问 Token；
- 真实用户数据和未脱敏日志；
- 无法复现的口头结论；
- 只有“成功了”而没有命令和条件的记录；
- 为了文章效果而补造的数据。

## 命名规则

```text
docs/evidence/M1-P01-cli-skeleton.md
docs/evidence/M1-P02-stream-cancel.md
docs/evidence/M1-P03-usage.md
```

一份证据可以支撑多篇文章，但文章必须说明它验证了什么、没有验证什么。

新记录从 [TEMPLATE.md](TEMPLATE.md) 复制结构。
