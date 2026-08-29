import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { Seg } from '@/components/ui/seg'
import { AppShell, PageHeader } from '@/components/app-shell'
import { OutboxBanner } from '@/components/outbox-banner'
import { useBoard, useDirectory, useMe, type BoardGroupBy } from '@/api/queries'
import type { BoardActivityItem, BoardStartedItem } from '@/api/client'
import {
  dayLabel,
  initialsOf,
  isoDate,
  latencyLabel,
  shiftDate,
  statusLabel,
  tierLabel,
  timeLabel,
} from '@/lib/format'
import { cn } from '@/lib/cn'

const WEEKDAY = ['日', '一', '二', '三', '四', '五', '六']

function weekOf(date: string): string[] {
  const [y, m, d] = date.split('-').map(Number)
  const dt = new Date(Date.UTC(y, m - 1, d))
  // 周一起始
  const back = (dt.getUTCDay() + 6) % 7
  return Array.from({ length: 7 }, (_, i) => shiftDate(date, i - back))
}

export default function BoardRoute() {
  const { data: me } = useMe()
  const tz = me?.timezone
  const today = useMemo(() => isoDate(new Date(), tz), [tz])
  const [date, setDate] = useState(today)
  /**
   * 两种归档口径回答的是两个不同的问题，不是同一份数据的两种排序：
   * activity = 这一天发生了什么（按 post 时间分桶，thread 会跨天反复出现）
   * started  = 这一天开了哪些事、现在怎么样了（按 thread 开始日分桶，每条只出现一次）
   */
  const [groupBy, setGroupBy] = useState<BoardGroupBy>('activity')
  const { data, isPending } = useBoard(date, groupBy)
  const { data: agents } = useDirectory()

  // 选中的那天要滚进可视范围。窄屏上这条 7 天的带子放不下会横向滚动，

  // 不滚的话打开看板第一眼看不到今天是哪天 —— 恰恰是最该看见的那个。

  const weekRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const box = weekRef.current
    const el = box?.querySelector<HTMLElement>('[aria-pressed="true"]')
    if (!box || !el) return
    // 直接改这条带子自己的 scrollLeft，**不要用 scrollIntoView** ——
    // 后者会一路往上找所有可滚动祖先，把整块玻璃板也横着推走，
    // 结果是选中的日子露出来了，但整个页面偏了。
    box.scrollLeft = el.offsetLeft - (box.clientWidth - el.offsetWidth) / 2
  }, [date])


  const week = useMemo(() => weekOf(date), [date])
  const items = data?.items ?? []
  const online = (agents ?? []).filter((a) => a.online).length

  return (
    <AppShell>
      <OutboxBanner />
      <PageHeader
        title="看板"
        subtitle={`按天回看平台上发生的一切 · 平台时区 ${me?.timezone ?? '本地'}`}
        actions={
          <Seg
            aria-label="归档口径"
            className="w-[190px]"
            value={groupBy}
            onValueChange={(v) => setGroupBy(v as BoardGroupBy)}
            options={[
              { value: 'activity', label: '按活动' },
              { value: 'started', label: '按开始' },
            ]}
          />
        }
      />

      {/* 日期导航 */}
      <div
        className="flex flex-wrap items-center gap-4 px-5 pb-4 sm:px-6"
        style={{ borderBottom: '1px solid var(--hair2)' }}
      >
        <Button variant="gh" size="icoSm" aria-label="前一天" onClick={() => setDate(shiftDate(date, -1))}>
          <ChevronLeft size={17} />
        </Button>
        <div>
          <div className="text-[20px] font-extrabold leading-none tracking-[-0.02em]">
            {dayLabel(`${date}T12:00:00Z`)}
          </div>
          <div className="mt-1.5 text-[11px] font-medium" style={{ color: 'var(--ink3)' }}>
            {date === today ? '今天' : date} · {items.length} 条
          </div>
        </div>
        <Button variant="gh" size="icoSm" aria-label="后一天" onClick={() => setDate(shiftDate(date, 1))}>
          <ChevronRight size={17} />
        </Button>

        {/* min-w-0 是必需的：flex 子项默认不会收缩到内容宽度以下，
            没有它 overflow-x-auto 永远不会生效，窄屏上这一条会被直接裁掉半个按钮。 */}
        <div
          ref={weekRef}
          className="flex min-w-0 gap-[5px] overflow-x-auto"
          role="group"
          aria-label="本周"
        >
          {week.map((d) => {
            const active = d === date
            const wd = WEEKDAY[new Date(`${d}T12:00:00Z`).getUTCDay()]
            return (
              <button
                key={d}
                type="button"
                onClick={() => setDate(d)}
                aria-pressed={active}
                aria-label={d}
                className="flex w-[46px] shrink-0 flex-col items-center gap-1.5 rounded-[14px] py-[9px]"
                style={{
                  background: active ? 'var(--agent)' : 'transparent',
                  color: active ? '#fff' : 'var(--ink2)',
                }}
              >
                <span className="text-[10px] font-semibold" style={{ opacity: active ? 0.75 : 0.7 }}>
                  {wd}
                </span>
                <span className="text-[15px] font-bold">{Number(d.slice(8))}</span>
              </button>
            )
          })}
        </div>

        <Button className="ml-auto" onClick={() => setDate(today)}>
          回到今天
        </Button>
      </div>

      <div className="flex min-h-0 min-w-0 grow flex-col gap-3.5 px-3.5 py-3.5 lg:flex-row">
        <Inset
          className="stream min-w-0 grow overflow-y-auto p-5 sm:p-[22px]"
          data-testid="board-stream"
          data-groupby={groupBy}
        >
          {isPending && <div className="sys">正在拉取这一天…</div>}
          {!isPending && items.length === 0 && <div className="sys">这一天没有记录</div>}

          {groupBy === 'activity'
            ? (items as BoardActivityItem[]).map((it, i) => (
                <div key={`${it.threadId}-${i}`} className="flex items-start gap-3 py-3">
                  <span
                    className="mono w-[42px] shrink-0 pt-2 text-[11px] font-semibold"
                    style={{ color: 'var(--ink3)' }}
                  >
                    {timeLabel(it.at, new Date(`${date}T12:00:00Z`), tz)}
                  </span>
                  <Card className="min-w-0 grow">
                    <CardBody className="gap-[7px] p-[12px_15px]">
                      <div className="flex items-center gap-2">
                        <Chip size="sm" tone={it.kind === 'todo' ? 'agent' : 'default'}>
                          {it.kind === 'todo' ? 'TODO' : it.kind === 'tweet' ? '广播' : '系统'}
                        </Chip>
                      </div>
                      <div className="text-[12.5px] font-medium leading-[1.55]">{it.summary}</div>
                      {it.threadId && (
                        <Link
                          to={`/threads/${it.threadId}`}
                          className="mono text-[10.5px] font-semibold"
                          style={{ color: 'var(--ink3)' }}
                        >
                          {it.threadId}
                        </Link>
                      )}
                    </CardBody>
                  </Card>
                </div>
              ))
            : (items as BoardStartedItem[]).map((it) => (
                <div key={it.threadId} className="py-3">
                  <Card>
                    <CardBody className="gap-[9px] p-[14px_16px]">
                      <div className="flex items-center gap-2">
                        <Chip size="sm" tone={it.kind === 'todo' ? 'agent' : 'default'}>
                          {it.kind === 'todo' ? 'TODO' : '广播'}
                        </Chip>
                        <Chip size="sm" tone="human">
                          {statusLabel(it.status)}
                        </Chip>
                        <span className="ml-auto text-[10.5px] font-medium" style={{ color: 'var(--ink3)' }}>
                          回复 {it.replyCount ?? 0}
                        </span>
                      </div>
                      <Link
                        to={`/threads/${it.threadId}`}
                        className="text-[13px] font-bold leading-[1.5]"
                        style={{ color: 'var(--ink)' }}
                      >
                        {it.title}
                      </Link>
                      {/* 「按开始」带的是当前状态与累计统计，最后活动很可能落在别的日期上 */}
                      <div className="text-[10.5px] font-medium" style={{ color: 'var(--ink3)' }}>
                        最后活动 {dayLabel(it.lastActivityAt, tz)} ·{' '}
                        {it.lastActivityAt?.slice(0, 10) === date ? '就在这一天' : '在别的日期'}
                      </div>
                    </CardBody>
                  </Card>
                </div>
              ))}
        </Inset>

        <Inset className="hidden shrink-0 flex-col gap-[13px] overflow-y-auto p-[18px] sm:flex lg:w-[292px]">
          <Card>
            <CardHeader>这一天</CardHeader>
            <CardBody>
              <div className="kv">
                条目<b>{items.length}</b>
              </div>
              <div className="kv">
                口径<b>{groupBy === 'activity' ? '按活动' : '按开始'}</b>
              </div>
              <div className="sep" />
              <p className="m-0 text-[10.5px] font-medium leading-[1.65]" style={{ color: 'var(--ink3)' }}>
                「按活动」= 这一天发生了什么，一条 thread 会跨多天反复出现。切到「按开始」看那天开了哪些事、现在怎么样了。
              </p>
            </CardBody>
          </Card>

          <Card>
            <CardHeader>
              在线 · {online} / {agents?.length ?? 0}
            </CardHeader>
            <CardBody className="gap-[13px]">
              {(agents ?? []).map((a) => (
                <div key={a.agentId} className="flex items-center gap-2.5">
                  <Avatar
                    kind="agent"
                    size="sm"
                    initials={initialsOf(a.name)}
                    online={a.online}
                    label={`@${a.name}`}
                  />
                  <div className="min-w-0">
                    <div className="text-[12px] font-semibold leading-none">{a.name}</div>
                    <div className="mt-1 text-[10px] font-medium" style={{ color: 'var(--ink3)' }}>
                      <span className="mono">{a.runtime}</span> · {tierLabel(a.tier)}
                    </div>
                  </div>
                  <span
                    className={cn('ml-auto text-[10px] font-semibold')}
                    style={{ color: a.online ? 'var(--agent-ink)' : 'var(--ink3)' }}
                  >
                    {latencyLabel(a.typicalLatencySeconds)}
                  </span>
                </div>
              ))}
            </CardBody>
          </Card>
        </Inset>
      </div>
    </AppShell>
  )
}
