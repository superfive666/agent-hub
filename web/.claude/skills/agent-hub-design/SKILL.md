---
name: agent-hub-design
description: 在 web/ 里做任何界面工作时用 —— 新页面、改样式、调动效、加组件。它把项目已定的设计语言（毛玻璃 + 虹彩流光 + 亮暗双主题）和不能破的功能性不变量交给你，并说明什么时候该叫上 impeccable 或 taste-skill。写第一行 CSS 之前读。
---

# 在 agent-hub 里做设计改动

## 先读这两样

1. **[设计语言](../../../docs/07-design-language.md)** —— 不变量、质感配方、色彩与字体、响应式。
2. **设计稿的「设计系统」页** —— 两套色板、动效清单、可直接粘的 CSS。源码在 `docs/design/Tokens.dc.html`。

## 什么时候叫上哪个 skill

| 任务 | 用 |
|---|---|
| 控制台的页面、组件、表单、设置、数据密集视图 | **`impeccable`** |
| `api-docs/` 或 `developer-docs/` 这类展示型站点 | **`taste-skill`** |
| 已有页面的小改动（改一个间距、加一个 chip） | **都不用**，照着现有 token 改就行 |

`taste-skill` 的旗舰 skill 自己写明适用范围是 "landing pages, portfolios, and redesigns.
**Not dashboards, not data tables, not multi-step product UI**" —— 控制台的主要界面正好被它排除在外。
别拿它去改 thread 视图或看板。

## 五条不能破的（详见设计语言 §1）

1. **人和 agent 一眼分得开** —— 位置 / 气泡 / `@` 前缀 / 「人类」chip，四重叠加，少一重都不行。位置是最强的那一重。
2. **主 agent 与关注者有明确层级** —— 主 agent 呼吸辉光，关注者虚线透明。虚线是语义，不是装饰。
3. **特效只给有语义的地方** —— 流光只在主 agent 卡片和当前会话上。加新特效前先回答"它在表达什么状态"。
4. **outbox 告警不可折叠、不可降级** —— worker 挂掉是静默失败，这是唯一能发现它的地方。
5. **尊重 `prefers-reduced-motion`** —— 全局兜底已经写好，别绕过去。

破其中任何一条之前，先改 [设计语言](../../../docs/07-design-language.md) 并说明理由——和 ADR 一样的规矩。

## 实现约定

- **只引用语义变量**（`var(--agent)` / `var(--surface)` / `var(--line)`），不写死颜色。
  这是亮暗双主题能成立的唯一原因。
- shadcn 组件复制进来之后，**第一件事是把默认 token 换成我们这一层**。
  它自带的 new-york 皮肤（Inter + 中性灰圆角）现在满互联网都是，不换就白做设计了。
- 虹彩四色 `--i1..--i4` **只用于流光与球体**，不要拿去编码信息（状态、类型、优先级都不行）。
- 等宽字体只给真正的机器内容：agent id、seq、token、runtime 名、代码。不拿它做风格。
- `backdrop-filter` 很贵，同屏同时模糊的层控制在 ~8 个以内。列表行不要逐行加。
- 新动画走已有的 `--ease`（`cubic-bezier(.22,.61,.28,1)`）与既有时长档位，别每处自己定一套。

## 检查清单

改完之后自己过一遍：

- [ ] 亮色和暗色都看过，不是只看了一个
- [ ] 缩到 390px 宽，§1 的四重区分信号仍然全在
- [ ] 正文与关键标签的对比度过 WCAG AA（霓虹好看 ≠ 可读）
- [ ] 辉光/流光不是任何信息的**唯一**载体
- [ ] 开着 `prefers-reduced-motion: reduce` 页面仍然可用
- [ ] 没有新增第三方 UI 依赖（Radix 之外）
