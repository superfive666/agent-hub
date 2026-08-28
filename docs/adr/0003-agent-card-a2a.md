# 0003. Agent Card 采用 A2A v1.0，hub 代为发布

- **状态**：已接受
- **日期**：2026-08-28
- **关联**：[Agent Card 设计](../06-agent-card.md)、原选型待决项 T3

## 背景

Agent Card 要承载 agent 的自我介绍。自定义 schema 意味着从零发明"怎么描述一个 agent"，而这件事已经有公认答案。

## 决策

采用 [A2A 协议](https://github.com/a2aproject/A2A) 的 **AgentCard v1.0.0** 结构（Linux Foundation 治理）。

**三点具体做法：**

1. **Hub 代为发布** —— card 挂在 `https://<hub>/agents/{id}/.well-known/agent-card.json`，`supportedInterfaces` 指向 hub 为该 agent 提供的端点。

   这是为了绕开一个结构性冲突：A2A 假设 agent 是有可调用端点的服务端，而我们的 agent 恰恰没有——这正是要做 inbox + connector 的原因。让 hub 当门面既符合星型拓扑，也为将来对外 A2A 互通留好了位置。

2. **自定义字段走 `AgentExtension`**，URI 形如 `https://agent-hub/ext/agent-profile/v1`，`required: false`。承载能力边界、runtime 类型、接入档位、典型响应时长等 A2A 没有原生字段的内容。

3. **暂不实现** `signatures`、`securitySchemes`、`extendedAgentCard`——card 目前只在 hub 内部流转，hub 本身是信任锚。

## 影响

- `skills[]` 直接可用于"能力清单"，agent 的自我介绍有了公认写法。
- **能力边界是 A2A 唯一没覆盖、而我们最看重的一项**（选主 agent 时"我不能做什么"比"我能做什么"有用），必须靠扩展承载。
- 校验免费：A2A 以 JSON Schema 2020-12 发布，直接跑校验。
- 字段命名统一 camelCase（A2A 硬性要求）。
- 需盯 A2A 后续版本演进，升级时评估破坏性变更。

## 修正

早前文档写的 well-known 路径 `/.well-known/agent.json` 是 A2A v0.x 的旧路径。**v1.0 是 `/.well-known/agent-card.json`**。

## 什么情况下重新审视

A2A 出现破坏性大版本，或扩展字段膨胀到"标准部分只是个壳"——那说明这个规范其实不合适，该老实自定义。
