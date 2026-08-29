import { useState } from 'react'
import { Link } from 'react-router'
import { Info, KeyRound, Plus } from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardBody } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { AppShell, PageHeader } from '@/components/app-shell'
import { OutboxBanner } from '@/components/outbox-banner'
import { RegistrationTokenPanel } from '@/components/registration-token'
import { useAdminAgents, useDirectory, useIssueRegistrationToken } from '@/api/queries'
import {
  agentStatusLabel,
  dateTimeLabel,
  initialsOf,
  latencyLabel,
  tierLabel,
} from '@/lib/format'

function skillLabel(skill: Record<string, unknown>, i: number): string {
  const v = skill.name ?? skill.id ?? skill.title
  return typeof v === 'string' ? v : `能力 ${i + 1}`
}

export default function DirectoryRoute() {
  const { data: agents, isPending, isError, error } = useDirectory()
  /**
   * **这一页要拉两份数据，因为它们回答的不是同一个问题。**
   *
   * `useDirectory()` 是 Agent Card 的摘要（「该找谁」）—— **没写 Card 的 agent 查不到**；
   * `useAdminAgents()` 是运维视角（「还活着吗、手上压了多少事」），刚建出来还没接入的
   * 记录只在这一份里。只画前者，就会出现「我明明加了一个 agent，页面上却找不到」；
   * 只画后者，又丢掉了能力与边界这些真正用来决定「该找谁」的信息。
   *
   * 所以页面分成两栏讲：上面是「还没进名录的」（带接入指引和重新签发 token），
   * 下面是名录本身。两栏的标题各自说清楚自己是什么。
   */
  const { data: records, isPending: recPending, isError: recError } = useAdminAgents()
  const issue = useIssueRegistrationToken()
  const [onlyOnline, setOnlyOnline] = useState(false)
  /** 重新签发出来的明文 token —— 同样只出现这一次，收起就没了，所以留在 state 里由用户自己关 */
  const [issued, setIssued] = useState<{
    agentId: string
    name: string
    token: string
    expiresAt?: string
  } | null>(null)

  const list = (agents ?? []).filter((a) => !onlyOnline || a.online)
  /** 名录条目上补一句运维状态：被停用的 agent 在 Card 摘要里看不出任何异常 */
  const recordOf = (agentId: string | undefined) =>
    agentId ? (records ?? []).find((r) => r.agentId === agentId) : undefined
  /** 还没进名录的：没写 Card 的都算 —— 未接入的固然没有，接入了不写 Card 的一样查不到 */
  const offstage = (records ?? []).filter((r) => !r.hasCard)
  const loading = isPending || recPending
  /** 两份数据都拿到了、两边都是空的 —— 平台上真的一个 agent 都没有 */
  const nothingAtAll =
    !loading && !recError && (agents?.length ?? 0) === 0 && (records?.length ?? 0) === 0
  /** 有 Card 的 agent 存在，只是被「只看在线」筛没了 —— 这和「没有 agent」是两回事 */
  const filteredOut = !loading && (agents?.length ?? 0) > 0 && list.length === 0

  return (
    <AppShell>
      <OutboxBanner />
      <PageHeader
        title="名录"
        subtitle="平台上还有谁、各自擅长什么 · Agent Card 采用 A2A v1.0，由 agent 自己撰写"
        actions={
          <>
            <Button
              onClick={() => setOnlyOnline((v) => !v)}
              aria-pressed={onlyOnline}
              className="text-[11.5px]"
            >
              {onlyOnline ? '只看在线' : '全部'}
            </Button>
            <Button variant="pri" data-testid="add-agent" asChild>
              <Link to="/directory/new">
                <Plus size={15} aria-hidden /> 添加 agent
              </Link>
            </Button>
          </>
        }
      />

      <div
        className="flex items-start gap-2.5 px-5 py-3 sm:px-6"
        style={{ background: 'var(--agent-soft)' }}
      >
        <span style={{ color: 'var(--agent-ink)' }} className="mt-0.5 shrink-0">
          <Info size={14} aria-hidden />
        </span>
        <span className="text-[12px] font-medium leading-[1.5]" style={{ color: 'var(--agent-ink)' }}>
          agent 也能拉这份名录 —— skill 里写明了：<b>先查名录再 @ 人，不要凭印象点名</b>
          。注册和 Card 更新会自动广播出去，所以大家的认知不会过期。
        </span>
      </div>

      <div className="flex min-h-0 grow flex-col p-3.5">
        <Inset className="stream flex min-w-0 grow flex-col gap-5 overflow-y-auto p-5 sm:p-[22px]">
          {loading && <div className="sys">正在拉取名录…</div>}
          {isError && (
            <div role="alert" className="sys" style={{ color: 'var(--alert)' }}>
              读不到名录：{(error as Error).message}
            </div>
          )}
          {recError && (
            <div role="alert" className="sys" style={{ color: 'var(--alert)' }}>
              读不到 agent 记录 —— 未接入的那些这次列不出来
            </div>
          )}
          {issue.isError && (
            <div role="alert" className="sys" style={{ color: 'var(--alert)' }}>
              没能重新签发 token：{(issue.error as Error).message}
            </div>
          )}

          {/* 空态一：一个 agent 都没有。/threads 的空态正好把人引到这儿，
              所以这里必须接得住，不能是一片什么都不画的空白。 */}
          {nothingAtAll && (
            <div className="m-auto w-full max-w-[440px] py-4" data-testid="directory-empty">
              <Card>
                <CardBody className="gap-3.5 p-[20px_22px]">
                  <p className="m-0 text-[13.5px] font-bold leading-[1.5]">
                    平台上还没有任何 agent
                  </p>
                  <p
                    className="m-0 text-[11.5px] font-medium leading-[1.75]"
                    style={{ color: 'var(--ink3)' }}
                  >
                    先建一条记录、签一张一次性注册 token，交给那台机器上的 connector
                    去换长期凭证。它接入并写完 Agent Card，才会出现在这份名录里。
                  </p>
                  <Button variant="pri" size="block" className="py-3" asChild>
                    <Link to="/directory/new">添加第一个 agent</Link>
                  </Button>
                </CardBody>
              </Card>
            </div>
          )}

          {/* 重新签发出来的 token：就地展开在最上面，收起前一直在 */}
          {issued && (
            <RegistrationTokenPanel
              agentName={issued.name}
              token={issued.token}
              expiresAt={issued.expiresAt}
              footer={
                <Button size="block" className="py-3" onClick={() => setIssued(null)}>
                  我存好了，收起
                </Button>
              }
            />
          )}

          {/* ── 第一栏：还没进名录的记录 ── */}
          {offstage.length > 0 && (
            <section data-testid="offstage" className="flex flex-col gap-3">
              <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
                <span className="lbl">还没出现在名录里 · {offstage.length}</span>
                <span className="text-[11px] font-medium" style={{ color: 'var(--ink3)' }}>
                  名录只收录写了 Agent Card 的 agent —— 下面这些建好了，但还没接入或还没写 Card
                </span>
              </div>

              {offstage.map((r) => (
                <Card key={r.agentId} data-testid="offstage-row">
                  <CardBody className="gap-2.5 p-[15px_17px]">
                    <div className="flex flex-wrap items-center gap-2.5">
                      <Avatar
                        kind="agent"
                        size="sm"
                        initials={initialsOf(r.name)}
                        online={r.online}
                        label={`@${r.name ?? ''}`}
                      />
                      <span className="text-[13px] font-bold">{r.name}</span>
                      <Chip
                        tone={r.status === 'pending_registration' ? 'warn' : 'default'}
                        size="sm"
                      >
                        {agentStatusLabel(r.status)}
                      </Chip>
                      {r.status === 'pending_registration' && (
                        <Button
                          className="ml-auto text-[11.5px]"
                          aria-label={`给 ${r.name} 重新签发注册 token`}
                          disabled={issue.isPending}
                          onClick={() =>
                            issue.mutate(r.agentId!, {
                              onSuccess: (res) =>
                                setIssued({
                                  agentId: r.agentId!,
                                  name: r.name ?? '',
                                  token: res.registrationToken,
                                  expiresAt: res.expiresAt,
                                }),
                            })
                          }
                        >
                          <KeyRound size={13} aria-hidden /> 重新签发 token
                        </Button>
                      )}
                    </div>

                    <div
                      className="text-[11.5px] font-medium leading-[1.7]"
                      style={{ color: 'var(--ink2)' }}
                    >
                      {r.purpose || '（建的时候没写用途）'}
                    </div>

                    {/* 两种「查不到」的原因不一样，得分开说，否则用户只会觉得平台坏了 */}
                    <div
                      className="rounded-[13px] px-[13px] py-[10px] text-[11px] font-medium leading-[1.65]"
                      style={{
                        background:
                          r.status === 'pending_registration' ? 'var(--warn-soft)' : 'var(--agent-soft)',
                        color:
                          r.status === 'pending_registration' ? 'var(--warn)' : 'var(--agent-ink)',
                      }}
                    >
                      {r.status === 'pending_registration'
                        ? '还没拿注册 token 换过长期凭证 —— 上一张 token 24 小时就过期，过期了就在这里重新签一张交给它。'
                        : '已经接入了，但还没写 Agent Card —— 没有能力与边界，别人无从判断该不该找它。这一步只能它自己做。'}
                    </div>

                    <div
                      className="flex flex-wrap items-center gap-x-3.5 gap-y-1.5 text-[10.5px] font-medium"
                      style={{ color: 'var(--ink3)' }}
                    >
                      <span>建于 {dateTimeLabel(r.createdAt)}</span>
                      <span>手上 {r.openTodos ?? 0} 条未完成</span>
                      {r.runtime && <span className="mono">{r.runtime}</span>}
                      <span className="mono ml-auto">{r.agentId}</span>
                    </div>
                  </CardBody>
                </Card>
              ))}
            </section>
          )}

          {/* ── 第二栏：名录本身（Card 摘要） ── */}
          {!nothingAtAll && (
            <section className="flex flex-col gap-3">
              <div className="flex flex-wrap items-baseline gap-x-2.5 gap-y-1">
                <span className="lbl">名录 · {list.length}</span>
                <span className="text-[11px] font-medium" style={{ color: 'var(--ink3)' }}>
                  写了 Agent Card 的才在这里 —— 这是「该找谁」的唯一依据
                </span>
              </div>

              {/* 空态二：有 agent，只是被「只看在线」筛没了。和「没有 agent」必须分开说，
                  不然用户会以为自己的 agent 消失了。 */}
              {filteredOut && (
                <div data-testid="directory-filtered-empty" className="py-2">
                  <div className="sys">
                    「只看在线」把 {agents?.length ?? 0} 个 agent 全筛掉了 —— 现在没有一个在线的。
                  </div>
                  <div className="mt-2.5 flex justify-center">
                    <Button onClick={() => setOnlyOnline(false)}>切回全部</Button>
                  </div>
                </div>
              )}

              {!loading && !filteredOut && list.length === 0 && (
                <div data-testid="directory-nocard-empty" className="sys">
                  名录还是空的 —— 记录建好了，但没有一个 agent 写过 Agent Card。Card 只能由 agent
                  自己接入后撰写。
                </div>
              )}

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                {list.map((a) => (
                  <Card key={a.agentId} data-testid="agent-card">
                    <CardBody className="gap-[13px] p-[18px_19px]">
                      <div className="flex items-center gap-3">
                        <Avatar
                          kind="agent"
                          initials={initialsOf(a.name)}
                          online={a.online}
                          label={`@${a.name}`}
                        />
                        <div className="min-w-0">
                          <div className="text-[14px] font-bold leading-none">{a.name}</div>
                          <div
                            className="mt-1.5 text-[10.5px] font-medium leading-none"
                            style={{ color: 'var(--ink3)' }}
                          >
                            @{a.name}
                          </div>
                        </div>
                        <div className="ml-auto flex shrink-0 flex-wrap items-center justify-end gap-1.5">
                          {/* 停用/未接入是运维状态，只有 /api/admin/agents 知道 ——
                              不补上去的话，一个被停用的 agent 在名录里看着和正常的一模一样 */}
                          {recordOf(a.agentId)?.status &&
                            recordOf(a.agentId)!.status !== 'active' && (
                              <Chip tone="alert" size="sm">
                                {agentStatusLabel(recordOf(a.agentId)!.status)}
                              </Chip>
                            )}
                          <Chip tone={a.online ? 'agent' : 'default'} size="sm">
                            {a.online ? '在线' : '离线'}
                          </Chip>
                        </div>
                      </div>

                      <div className="text-[12px] leading-[1.65]" style={{ color: 'var(--ink2)' }}>
                        {a.description ?? '（尚未撰写 Agent Card）'}
                      </div>

                      <div>
                        <div className="lbl mb-[7px]">能做</div>
                        <div className="flex flex-wrap gap-[5px]">
                          {(a.skills ?? []).map((s, i) => (
                            <Chip key={i} size="sm">
                              {skillLabel(s, i)}
                            </Chip>
                          ))}
                          {(a.skills ?? []).length === 0 && (
                            <span className="text-[11px]" style={{ color: 'var(--ink3)' }}>
                              还没声明
                            </span>
                          )}
                        </div>
                      </div>

                      {/* 「不能做」单独拎出来用告警色块 —— 它比能力清单更有信息量 */}
                      <div
                        data-testid="limitations"
                        className="rounded-[13px] px-[13px] py-[11px]"
                        style={{ background: 'var(--alert-soft)' }}
                      >
                        <div className="lbl mb-1.5" style={{ color: 'var(--alert)' }}>
                          不能做
                        </div>
                        <div
                          className="text-[11px] font-medium leading-[1.6]"
                          style={{ color: 'var(--alert)' }}
                        >
                          {(a.limitations ?? []).length > 0
                            ? (a.limitations ?? []).join(' · ')
                            : 'Card 里没写能力边界 —— 别人无从判断该不该找它'}
                        </div>
                      </div>

                      <div className="sep" />
                      <div
                        className="flex items-center gap-3.5 text-[10.5px] font-medium"
                        style={{ color: 'var(--ink3)' }}
                      >
                        <span className="mono">{a.runtime ?? '—'}</span>
                        <span>{tierLabel(a.tier)}</span>
                        <span className="ml-auto">{latencyLabel(a.typicalLatencySeconds)}</span>
                      </div>
                    </CardBody>
                  </Card>
                ))}
              </div>
            </section>
          )}
        </Inset>
      </div>
    </AppShell>
  )
}
