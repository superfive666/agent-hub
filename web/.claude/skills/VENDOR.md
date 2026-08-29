# 第三方 skill 来源

这两套 skill 是从上游仓库 vendor 进来的，不是本项目编写的。升级时重新拉取上游并覆盖，不要就地改。

| skill | 上游 | commit | 许可证 |
|---|---|---|---|
| `impeccable/` | https://github.com/pbakaus/impeccable | `ea36002` | Apache 2.0（见 `impeccable/LICENSE`） |
| `taste-skill/` 等 13 个 | https://github.com/Leonxlnx/taste-skill | `ccbc156` | MIT（见 `LICENSE.taste-skill`） |

vendor 于 2026-08-28。

## 用哪个

agent-hub 的 web 是**管理控制台**：dashboard、数据表格、多步表单、thread 视图。

- **impeccable** 明确覆盖 dashboard、product UI、app shell、表单、设置页，是这个项目的主力。
- **taste-skill** 里的旗舰 skill `design-taste-frontend` 自己写明了适用范围是
  "landing pages, portfolios, and redesigns. **Not dashboards, not data tables, not multi-step product UI**"
  —— 正好把我们的主要界面排除在外。它在做**开发者文档站、API 文档站**（`developer-docs/`、`api-docs/`）
  这类偏展示的页面时才对口；同仓库的 `brandkit`、`output-skill` 等辅助 skill 不受这条限制。

设计基线见 [../../../docs/design/](../../../docs/design/) 与已发布的设计稿，不要脱离既有视觉系统另起一套。
