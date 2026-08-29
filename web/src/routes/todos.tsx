import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { Plus } from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardBody } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { Seg } from '@/components/ui/seg'
import { AppShell, PageHeader } from '@/components/app-shell'
import { OutboxBanner } from '@/components/outbox-banner'
import { useTodos } from '@/api/queries'
import { STATUS_FLOW, dateTimeLabel, initialsOf, statusLabel, timeLabel } from '@/lib/format'

/**
 * 「等你确认」不是一个 status，它是 `confirmedAt` 这一位数据（ADR-0008）。
 * 但它必须能被筛出来：闸门挡着的 todo 在列表里和别的长得一样，
 * 人只会以为 agent 在偷懒，而实际上是在等自己点一下。
 */
const AWAITING_CONFIRM = 'awaiting_confirm'

const FILTERS = [
  { value: 'all', label: '全部' },
  { value: AWAITING_CONFIRM, label: '等你确认' },
  ...STATUS_FLOW.map((s) => ({ value: s, label: statusLabel(s) })),
]

/** 还没确认，且事情还没结束 —— 已完成/已取消的老 todo 不该被催 */
function needsConfirm(t: { confirmedAt?: string | null; status?: string }): boolean {
  return !t.confirmedAt && t.status !== 'done' && t.status !== 'cancelled'
}

export default function TodosRoute() {
  const { data: todos, isPending } = useTodos()
  const [filter, setFilter] = useState('all')
  const navigate = useNavigate()
  const list = (todos ?? []).filter((t) =>
    filter === 'all' ? true : filter === AWAITING_CONFIRM ? needsConfirm(t) : t.status === filter,
  )
  const waiting = (todos ?? []).filter(needsConfirm).length

  return (
    <AppShell>
      <OutboxBanner />
      <PageHeader
        title="待办"
        subtitle={
          waiting > 0 ? (
            <>
              <b style={{ color: 'var(--warn)' }}>{waiting} 条在等你确认</b> ·
              确认之前主 agent 只会提问和澄清，不会开始做
            </>
          ) : (
            '每条 todo 有且只有一个主 agent · 被 @ 的只是关注者'
          )
        }
        actions={
          <Button variant="pri" onClick={() => navigate('/todos/new')}>
            <Plus size={15} aria-hidden /> 新建
          </Button>
        }
      />

      <div className="px-5 pb-3 sm:px-6">
        <Seg
          aria-label="按状态筛选"
          value={filter}
          onValueChange={setFilter}
          options={FILTERS}
          className="max-w-[560px]"
        />
      </div>

      <div className="flex min-h-0 grow flex-col p-3.5">
        <Inset className="stream min-w-0 grow overflow-y-auto p-5 sm:p-[22px]">
          {isPending && <div className="sys">正在拉取…</div>}
          {!isPending && list.length === 0 && <div className="sys">这个筛选下没有 todo</div>}
          <div className="flex flex-col gap-3">
            {list.map((t) => (
              <Card key={t.threadId} data-testid="todo-row">
                <CardBody className="gap-2.5 p-[15px_17px]">
                  <div className="flex items-center gap-2.5">
                    <Avatar
                      kind="primary"
                      size="sm"
                      initials={initialsOf(t.primaryAgentName)}
                      online={t.primaryAgentOnline}
                      label={`@${t.primaryAgentName ?? ''}`}
                    />
                    <Link
                      to={`/threads/${t.threadId}`}
                      className="min-w-0 grow truncate text-[13px] font-bold"
                      style={{ color: 'var(--ink)' }}
                    >
                      {t.title}
                    </Link>
                    {needsConfirm(t) && (
                      <Chip tone="warn" size="sm" data-testid="awaiting-confirm">
                        等你确认
                      </Chip>
                    )}
                    <Chip tone="human" size="sm">
                      {statusLabel(t.status)}
                    </Chip>
                  </div>
                  <div
                    className="flex flex-wrap items-center gap-x-3.5 gap-y-1.5 text-[10.5px] font-medium"
                    style={{ color: 'var(--ink3)' }}
                  >
                    <span>
                      主 agent <b style={{ color: 'var(--agent-ink)' }}>@{t.primaryAgentName}</b>
                    </span>
                    <span>关注者 {(t.watchers ?? []).length}</span>
                    <span>回复 {t.replyCount ?? 0}</span>
                    <span>开始于 {timeLabel(t.startedAt)}</span>
                    {t.dueAt && <span style={{ color: 'var(--human)' }}>截止 {dateTimeLabel(t.dueAt)}</span>}
                    <span className="mono ml-auto">{t.threadId}</span>
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
