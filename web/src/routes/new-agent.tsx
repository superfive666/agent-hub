import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { AlertTriangle, Info } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { AppShell, PageHeader } from '@/components/app-shell'
import { OutboxBanner } from '@/components/outbox-banner'
import { RegistrationTokenPanel } from '@/components/registration-token'
import { RuntimePicker } from '@/components/runtime-picker'
import { useCreateAgent } from '@/api/queries'
import type { CreatedAgent } from '@/api/client'

/**
 * 名称的字符集。**这不是洁癖**：正文里的 `@` 提及靠这个字符集匹配，
 * 名字里有空格或中文的 agent 根本 @ 不到，而 @ 是平台上唯一的连接动作 ——
 * 一个 @ 不到的 agent 等于接不进协作。所以在前端就挡住，别等服务端 400。
 */
const NAME_RE = /^[A-Za-z0-9_-]+$/
const NAME_MAX = 64

/** 返回人话的错误，合法则返回 null。空串不报错 —— 还没开始填就标红是在骂人。 */
export function nameErrorOf(raw: string): string | null {
  const name = raw.trim()
  if (!name) return null
  if (name.length > NAME_MAX) return `太长了：最多 ${NAME_MAX} 个字符，现在有 ${name.length} 个`
  if (!NAME_RE.test(name))
    return '只能用字母、数字、下划线和连字符 —— 正文里的 @ 提及靠这个字符集匹配，带空格或中文的名字别人根本 @ 不到'
  return null
}

/**
 * 添加 agent。
 *
 * **为什么是一个独立页面而不是对话框。** 三个理由，按分量排：
 * 1. 成功后那一屏是**注册 token 的明文，只出现这一次**。对话框天然带着「点外面就关掉」
 *    的语义，误关一次这串 token 就永久没了，只能作废重发。一条真实的 URL 关不掉，
 *    也能在两块玻璃板上把「怎么交给 connector」摊开讲。
 * 2. 仓库里**没有对话框基元** —— Radix 只装了 avatar / slot / toggle-group，
 *    设计约定又写明「不新增 Radix 之外的 UI 依赖」。自己糊一个 focus trap
 *    做不好无障碍，为一个表单不值当。
 * 3. 和 `new-todo.tsx` 同构：同样是「填两个字段 → 提交 → 跳走」，两处形状一致。
 *
 * 路径挂在 `/directory/new` 而不是 `/agents/new`：`navValueOf` 用 `startsWith`
 * 判断高亮哪个导航项，挂在 /directory 下面，侧栏才会继续亮着「名录」。
 */
export default function NewAgentRoute() {
  const navigate = useNavigate()
  const create = useCreateAgent()
  const [name, setName] = useState('')
  const [purpose, setPurpose] = useState('')
  /**
   * 选中的 runtime。**它不会被提交到后端** —— runtime 存在 agent_card.runtime 里，
   * 由 agent 接入之后自己上报（Card 是它自己写的，见 ADR-0003）。这里选它只有一个
   * 用途：把接入命令里的 `RUNTIME=` 拼对。以前那条命令永远写死 codex，
   * 跑 claude-code 的人复制过去必然失败。
   */
  const [runtime, setRuntime] = useState('claude-code')
  /** 成功后把响应留在本地：它带着只出现一次的明文 token，不能靠重新拉接口拿回来 */
  const [created, setCreated] = useState<(CreatedAgent & { name: string }) | null>(null)

  const nameError = nameErrorOf(name)
  const canSubmit = name.trim().length > 0 && !nameError && !create.isPending

  const submit = () => {
    if (!canSubmit) return
    const n = name.trim()
    create.mutate(
      { name: n, purpose: purpose.trim() || undefined },
      { onSuccess: (r) => setCreated({ ...r, name: n }) },
    )
  }

  return (
    <AppShell>
      <OutboxBanner />
      <PageHeader
        title={created ? '接下来把 token 交给它' : '添加一个 agent'}
        subtitle={
          created
            ? '记录建好了 · 它现在是「未接入」，换过长期凭证才算真的进来'
            : '只建一条记录并签一张一次性注册 token · Agent Card 由 agent 自己接入后撰写'
        }
        actions={
          created ? (
            <Button variant="pri" asChild>
              <Link to="/directory">回名录</Link>
            </Button>
          ) : (
            <>
              <Button variant="gh" onClick={() => navigate(-1)}>
                取消
              </Button>
              <Button
                variant="pri"
                data-testid="create-agent"
                disabled={!canSubmit}
                aria-disabled={!canSubmit}
                onClick={submit}
              >
                创建并签发 token
              </Button>
            </>
          )
        }
      />

      <div className="flex min-h-0 min-w-0 grow flex-col gap-3.5 px-3.5 pb-3.5 lg:flex-row">
        <Inset className="stream flex min-w-0 grow flex-col gap-5 overflow-y-auto p-5 sm:p-[22px]">
          {created ? (
            <RegistrationTokenPanel
              agentName={created.name}
              token={created.registrationToken ?? ''}
              expiresAt={created.expiresAt}
              runtime={runtime}
              footer={
                <div className="flex flex-col gap-2 sm:flex-row">
                  <Button
                    size="block"
                    className="py-3"
                    onClick={() => {
                      // 「再建一个」= 把这一屏丢掉。丢掉之前那串 token 就真的没了，
                      // 所以按钮文案要说清楚，不要写成「继续」这种听不出后果的词。
                      setCreated(null)
                      setName('')
                      setPurpose('')
                      create.reset()
                    }}
                  >
                    我存好了，再建一个
                  </Button>
                  <Button variant="pri" size="block" className="py-3" asChild>
                    <Link to="/directory">回名录看这一条</Link>
                  </Button>
                </div>
              }
            />
          ) : (
            <>
              <Card>
                <CardBody className="gap-5 p-[22px_24px]">
                  <div>
                    <label className="lbl mb-2 block" htmlFor="agent-name">
                      名称
                    </label>
                    {/* 等宽：这一栏填的是**机器要匹配的标识符**（@ 提及靠它），
                        不是显示名 —— 字体本身就在说明这件事。 */}
                    <input
                      id="agent-name"
                      className="in mono w-full text-[15px] font-bold"
                      placeholder="rover"
                      autoComplete="off"
                      spellCheck={false}
                      aria-invalid={!!nameError}
                      aria-describedby="agent-name-hint"
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') submit()
                      }}
                    />
                    <div
                      id="agent-name-hint"
                      className="mt-2.5 text-[11px] font-medium leading-[1.7]"
                      style={{ color: 'var(--ink3)' }}
                    >
                      别人靠 <span className="mono">@{name.trim() || '名称'}</span> 找到它 ——
                      所以只能用字母、数字、下划线、连字符，最多 {NAME_MAX} 个字符，且不能和别人重名。
                    </div>
                    {nameError && (
                      <div
                        data-testid="name-invalid"
                        role="alert"
                        className="mt-2.5 rounded-[14px] px-[15px] py-[13px] text-[11.5px] font-medium leading-[1.7]"
                        style={{ background: 'var(--alert-soft)', color: 'var(--alert)' }}
                      >
                        {nameError}
                      </div>
                    )}
                  </div>

                  <div className="sep" />

                  <div>
                    <label className="lbl mb-2 block" htmlFor="agent-purpose">
                      简介 / 用途
                    </label>
                    <textarea
                      id="agent-purpose"
                      className="in w-full resize-none rounded-[18px] text-[13px] leading-[1.75]"
                      rows={4}
                      placeholder="这个 agent 是干什么的 —— 给你自己看的备注，比如「跑在书房那台小机器上，负责夜间巡检」"
                      value={purpose}
                      onChange={(e) => setPurpose(e.target.value)}
                    />
                    {/* 最容易误解的一点：这里不是填能力清单的地方 */}
                    <div
                      className="mt-2.5 flex items-start gap-2.5 rounded-[14px] px-[15px] py-[13px]"
                      style={{ background: 'var(--agent-soft)' }}
                    >
                      <span style={{ color: 'var(--agent-ink)' }} className="mt-0.5 shrink-0">
                        <Info size={14} aria-hidden />
                      </span>
                      <span
                        className="text-[11.5px] font-medium leading-[1.7]"
                        style={{ color: 'var(--agent-ink)' }}
                      >
                        <b>能力清单和能力边界不在这儿填。</b>那是 Agent Card（A2A v1.0），
                        由 agent 自己接入之后撰写并广播 —— 它比你更清楚自己能做什么、不能做什么。
                        这一栏只是给你自己看的备注。
                      </span>
                    </div>
                  </div>

                  <div className="sep" />

                  <div>
                    <span className="lbl mb-2 block">对方机器上跑的是哪个 runtime</span>
                    <RuntimePicker value={runtime} onChange={setRuntime} />
                    <p
                      className="m-0 mt-2.5 text-[11px] font-medium leading-[1.7]"
                      style={{ color: 'var(--ink3)' }}
                    >
                      这一项<b>不会存进 agent 记录</b> —— runtime 是 Agent Card 的一部分，
                      由 agent 接入之后自己上报。选它只是为了把下一屏的接入命令拼对；
                      选错了在命令里改掉 <span className="mono">RUNTIME=</span> 也行。
                    </p>
                  </div>
                </CardBody>
              </Card>

              {create.isError && (
                <div
                  data-testid="create-agent-error"
                  role="alert"
                  className="flex items-start gap-2.5 rounded-[16px] px-[16px] py-[14px]"
                  style={{ background: 'var(--alert-soft)' }}
                >
                  <span style={{ color: 'var(--alert)' }} className="mt-0.5 shrink-0">
                    <AlertTriangle size={15} aria-hidden />
                  </span>
                  <span
                    className="text-[12px] font-semibold leading-[1.7]"
                    style={{ color: 'var(--alert)' }}
                  >
                    {/* 撞名（409 agent_name_taken）前端拦不住 —— 只有服务端知道名字被占了。
                        HttpError 的 message 就是后端给的中文，原样展示，不要吞掉。 */}
                    没能创建：{(create.error as Error).message}
                  </span>
                </div>
              )}
            </>
          )}
        </Inset>

        {/* 右栏在 640–1023 会「下沉」到表单下面（§4）。竖排时**不许带裸 shrink-0**：
            它的自然高度是两张卡片叠起来，不许收缩就会把外面这条 flex 撑破。
            max-h 框住 + 自己滚，≥1024 并排时才轮到固定宽度、不收缩。 */}
        <Inset className="hidden min-h-0 max-h-[45%] flex-col gap-[13px] overflow-y-auto p-[18px] sm:flex lg:max-h-none lg:w-[292px] lg:shrink-0">
          <Card>
            <CardHeader>建完之后它在哪</CardHeader>
            <CardBody className="gap-3.5">
              <div>
                <div className="text-[12px] font-bold leading-none">先出现在「未接入」那一栏</div>
                <div
                  className="mt-[7px] text-[11px] font-medium leading-[1.7]"
                  style={{ color: 'var(--ink2)' }}
                >
                  新记录的状态是 <span className="mono">pending_registration</span>，只是一条占位。
                  它<b>还不会出现在名录里</b> —— 名录是 Agent Card 的摘要，Card 要等它自己接入后写。
                </div>
              </div>
              <div className="sep" />
              <div>
                <div className="text-[12px] font-bold leading-none">换过凭证才算真的进来</div>
                <div
                  className="mt-[7px] text-[11px] font-medium leading-[1.7]"
                  style={{ color: 'var(--ink2)' }}
                >
                  agent 拿注册 token 换到长期凭证后翻成 <span className="mono">active</span>，
                  写完 Card 就进名录，别人才能判断该不该找它。
                </div>
              </div>
            </CardBody>
          </Card>

          <Card>
            <CardHeader>为什么不直接发长期凭证</CardHeader>
            <CardBody>
              <p className="m-0 text-[11px] font-medium leading-[1.75]" style={{ color: 'var(--ink2)' }}>
                长期凭证的明文<b>从来不经过控制台</b>：agent 拿一次性 token 自己去换，
                换完 token 立即作废。所以这里能给的只有那张一次性票，泄漏了作废重发就行。
              </p>
              <div className="sep" />
              <div className="flex flex-wrap gap-1.5">
                <Chip size="sm" tone="warn">
                  token 24 小时过期
                </Chip>
                <Chip size="sm">用过即废</Chip>
                <Chip size="sm">明文只显示一次</Chip>
              </div>
            </CardBody>
          </Card>
        </Inset>
      </div>
    </AppShell>
  )
}
