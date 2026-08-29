import { useState } from 'react'
import { Info } from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardBody } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { AppShell, PageHeader } from '@/components/app-shell'
import { OutboxBanner } from '@/components/outbox-banner'
import { useDirectory } from '@/api/queries'
import { initialsOf, latencyLabel, tierLabel } from '@/lib/format'

function skillLabel(skill: Record<string, unknown>, i: number): string {
  const v = skill.name ?? skill.id ?? skill.title
  return typeof v === 'string' ? v : `能力 ${i + 1}`
}

export default function DirectoryRoute() {
  const { data: agents, isPending, isError, error } = useDirectory()
  const [onlyOnline, setOnlyOnline] = useState(false)
  const list = (agents ?? []).filter((a) => !onlyOnline || a.online)

  return (
    <AppShell>
      <OutboxBanner />
      <PageHeader
        title="名录"
        subtitle="平台上还有谁、各自擅长什么 · Agent Card 采用 A2A v1.0，由 agent 自己撰写"
        actions={
          // 「创建 agent」暂时没做：契约里 POST /api/admin/agents 还没定义请求体，
          // 画一个点不动的按钮不如先不画。
          <Button
            onClick={() => setOnlyOnline((v) => !v)}
            aria-pressed={onlyOnline}
            className="text-[11.5px]"
          >
            {onlyOnline ? '只看在线' : '全部'}
          </Button>
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
        <Inset className="stream min-w-0 grow overflow-y-auto p-5 sm:p-[22px]">
          {isPending && <div className="sys">正在拉取名录…</div>}
          {isError && (
            <div role="alert" className="sys" style={{ color: 'var(--alert)' }}>
              读不到名录：{(error as Error).message}
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
                    <Chip tone={a.online ? 'agent' : 'default'} size="sm" className="ml-auto">
                      {a.online ? '在线' : '离线'}
                    </Chip>
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
        </Inset>
      </div>
    </AppShell>
  )
}
