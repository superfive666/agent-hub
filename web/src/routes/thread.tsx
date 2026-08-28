import { useState } from 'react'
import {
  Check,
  LayoutGrid,
  Megaphone,
  MessagesSquare,
  MoreHorizontal,
  Search,
  Send,
  Settings,
  Users,
} from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { Pane } from '@/components/ui/pane'
import { Seg } from '@/components/ui/seg'
import { MessageRow } from '@/components/message-row'
import { OutboxAlert } from '@/components/outbox-alert'
import { ThemeToggle } from '@/components/theme-toggle'
import { cn } from '@/lib/cn'
import { health, isSystem, me, progress, thread, threads } from '@/mocks/thread'

function ConversationRow({
  active,
  item,
}: {
  active: boolean
  item: (typeof threads)[number]
}) {
  return (
    <button
      type="button"
      // 流光只挂当前会话（§1.3）。别的行不许带。
      className={cn('convo', active && 'glow')}
      data-active={active}
      aria-current={active ? 'true' : undefined}
    >
      <Avatar
        kind={item.primaryAgent.participation === 'primary' ? 'primary' : 'agent'}
        size="sm"
        initials={item.primaryAgent.initials}
        online={item.primaryAgent.online}
        label={`@${item.primaryAgent.name}`}
      />
      <span className="min-w-0 grow">
        <span className="block truncate text-[12.5px] font-bold leading-[1.35]">{item.title}</span>
        <span
          className="mt-1 block truncate text-[11px] font-medium leading-[1.4]"
          style={{ color: 'var(--ink3)' }}
        >
          {item.preview}
        </span>
      </span>
      <span className="flex shrink-0 flex-col items-end gap-1.5">
        <span className="text-[10px] font-medium" style={{ color: 'var(--ink3)' }}>
          {item.at}
        </span>
        {item.unread && (
          <span
            className="rounded-pill text-center text-[10px] font-bold leading-[18px]"
            style={{ width: 18, height: 18, background: 'var(--agent)', color: '#fff' }}
          >
            {item.unread}
          </span>
        )}
      </span>
    </button>
  )
}

export default function ThreadRoute() {
  const [tab, setTab] = useState('thread')
  const primary = thread.primaryAgent

  return (
    <div className="app min-h-dvh">
      {/* ── 玻璃板 1：左会话栏 ── */}
      <Pane className="hidden shrink-0 flex-col py-[18px] md:flex md:w-[86px] lg:w-[278px]">
        <div className="flex items-center gap-[11px] px-5 pb-4">
          <span
            className="flex size-[38px] shrink-0 items-center justify-center rounded-pill text-[13px] font-extrabold"
            style={{
              background: 'var(--pri-grad)',
              color: 'var(--pri-ink)',
              boxShadow: 'var(--pri-sh)',
              animation: 'breathe 5s var(--ease) infinite',
            }}
          >
            ah
          </span>
          <b className="hidden text-[15.5px] font-extrabold tracking-[-0.03em] lg:block">
            agent‑hub
          </b>
          <ThemeToggle className="ml-auto hidden lg:inline-flex" />
        </div>

        <div className="hidden px-4 pb-3.5 lg:block">
          <Seg
            aria-label="视图"
            value={tab}
            onValueChange={setTab}
            options={[
              { value: 'thread', label: '对话' },
              { value: 'board', label: '看板' },
              { value: 'directory', label: '名录' },
            ]}
          />
        </div>
        <div className="flex flex-col items-center gap-2 pb-3 lg:hidden">
          <Button variant="gh" size="icoSm" aria-label="对话">
            <MessagesSquare size={17} />
          </Button>
          <Button variant="gh" size="icoSm" aria-label="看板">
            <LayoutGrid size={17} />
          </Button>
          <Button variant="gh" size="icoSm" aria-label="名录">
            <Users size={17} />
          </Button>
        </div>

        <div className="hidden px-3.5 pb-2 lg:block">
          <div
            className="flex items-center gap-2 rounded-pill px-3 py-[9px] text-[12px] font-medium"
            style={{
              background: 'var(--inset-bg)',
              border: '1px solid var(--inset-bd)',
              color: 'var(--ink3)',
            }}
          >
            <Search size={14} aria-hidden />
            搜索 thread 或 agent
          </div>
        </div>

        <div className="hidden min-h-0 grow flex-col overflow-y-auto px-2.5 lg:flex">
          <span className="lbl px-3 pb-1.5 pt-2">进行中 · 5</span>
          <div className="flex flex-col gap-0.5">
            {threads
              .filter((t) => t.group === 'active')
              .map((t) => (
                <ConversationRow key={t.threadId} item={t} active={t.threadId === thread.threadId} />
              ))}
          </div>
          <span className="lbl px-3 pb-1.5 pt-4">最近</span>
          <div className="flex flex-col gap-0.5">
            {threads
              .filter((t) => t.group === 'recent')
              .map((t) => (
                <ConversationRow key={t.threadId} item={t} active={false} />
              ))}
          </div>
        </div>

        <div className="mt-auto flex flex-col gap-2.5 px-4 pt-3">
          <div className="sep hidden lg:block" />
          <div className="flex items-center gap-2.5">
            <Avatar kind="human" size="sm" initials={me.initials} label={me.name} />
            <div className="hidden min-w-0 lg:block">
              <div className="text-[12px] font-bold leading-none">{me.name}</div>
              <div className="mt-1 text-[10px] font-medium" style={{ color: 'var(--human)' }}>
                唯一管理员
              </div>
            </div>
            <Button variant="gh" size="icoSm" className="ml-auto hidden lg:inline-flex" aria-label="设置">
              <Settings size={14} />
            </Button>
          </div>
        </div>
      </Pane>

      {/* ── 玻璃板 2：主区 ── */}
      <Pane className="flex min-w-0 grow flex-col">
        <header className="relative z-[3] flex items-center gap-3.5 px-5 pb-4 pt-5 sm:px-6">
          <div className="min-w-0">
            <h1 className="m-0 truncate text-[19px] font-extrabold leading-[1.25] tracking-[-0.03em]">
              {thread.title}
            </h1>
            <div className="mt-1.5 text-[11.5px] font-medium" style={{ color: 'var(--ink3)' }}>
              <span className="mono">{thread.ref}</span> · 开始于 {thread.startedAtLabel} ·{' '}
              {thread.participantCount} 人在这个 thread 里
            </div>
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-2.5">
            <Chip tone="human">{thread.statusLabel}</Chip>
            <ThemeToggle className="lg:hidden" />
            <Button variant="gh" size="icoSm" aria-label="更多">
              <MoreHorizontal size={16} />
            </Button>
          </div>
        </header>

        {/* §1.4 outbox 告警：任何宽度、任何主题都不折叠、不降级 */}
        <div className="px-5 pb-3 sm:px-6">
          <OutboxAlert
            lagSeconds={health.outboxLagSeconds}
            workerAlive={health.workerAlive}
            pending={health.outboxPending}
          />
        </div>

        {/* <640px：右栏内容压成 thread 顶部状态带 */}
        <div className="flex items-center gap-2 overflow-x-auto px-5 pb-3 sm:hidden">
          <Chip tone="agent" size="sm">
            主 agent @{primary.name}
          </Chip>
          <Chip size="sm">关注者 {thread.watchers.length}</Chip>
          <Chip size="sm">截止 {thread.dueAtLabel}</Chip>
        </div>

        {/* ── 嵌套内板：消息流 + 右详情 ── */}
        <div className="flex min-h-0 min-w-0 grow flex-col gap-3.5 px-3.5 pb-3.5 xl:flex-row">
          <Inset className="stream flex min-w-0 grow flex-col gap-[15px] overflow-y-auto p-5 sm:p-[22px]">
            {thread.posts.map((item) =>
              isSystem(item) ? (
                <div key={item.postId} className="sys">
                  {item.system}
                </div>
              ) : (
                <MessageRow key={item.postId} post={item} />
              ),
            )}
          </Inset>

          <Inset className="hidden shrink-0 flex-col gap-[13px] p-[18px] sm:flex xl:w-[292px]">
            {/* 流光只给主 agent 卡片（§1.3） */}
            <Card className="glow runner">
              <CardHeader>主 AGENT · 必须响应</CardHeader>
              <CardBody>
                <div className="flex items-center gap-[11px]">
                  <Avatar
                    kind="primary"
                    initials={primary.initials}
                    online={primary.online}
                    label={`@${primary.name}`}
                  />
                  <div>
                    <div className="text-[13.5px] font-bold leading-none">{primary.name}</div>
                    <div
                      className="mt-[5px] text-[10.5px] font-semibold leading-none"
                      style={{ color: 'var(--agent-ink)' }}
                    >
                      在线 · {primary.tier}
                    </div>
                  </div>
                </div>
                <div className="sep" />
                <div className="kv">
                  runtime
                  <b className="mono text-[11px]">{primary.runtime}</b>
                </div>
                <div className="kv">
                  本条已响应<b>{primary.respondedIn}</b>
                </div>
                <div className="kv">
                  当前负责<b>{primary.owning} 条</b>
                </div>
                <p
                  className="m-0 text-[10.5px] font-medium leading-[1.65]"
                  style={{ color: 'var(--ink3)' }}
                >
                  一条 todo 有且只有一个主 agent。转派要在 thread 里留痕。
                </p>
                <Button size="block">转派</Button>
              </CardBody>
            </Card>

            <Card>
              <CardHeader>关注者 · {thread.watchers.length} · 不必回复</CardHeader>
              <CardBody>
                {thread.watchers.map((w) => (
                  <div key={w.name} className="flex items-center gap-2.5">
                    <Avatar
                      kind="agent"
                      size="sm"
                      initials={w.initials}
                      online={w.online}
                      label={`@${w.name}`}
                    />
                    <div className="min-w-0">
                      <div className="text-[12px] font-semibold leading-none">{w.name}</div>
                      <div className="mt-1 text-[10px] font-medium" style={{ color: 'var(--ink3)' }}>
                        {w.reason}
                      </div>
                    </div>
                    <Chip size="sm" className="ml-auto">
                      已回 {w.replies}
                    </Chip>
                  </div>
                ))}
                <p
                  className="m-0 text-[10.5px] font-medium leading-[1.65]"
                  style={{ color: 'var(--ink3)' }}
                >
                  被 @ 只产生关注关系：收通知、订阅更新，没有回复义务。
                </p>
              </CardBody>
            </Card>

            <Card>
              <CardHeader>进度</CardHeader>
              <CardBody className="gap-2.5">
                {progress.map((p) => (
                  <div key={p.label} className="flex items-center gap-[9px]">
                    <span
                      className="shrink-0 rounded-pill text-center text-[9px] font-bold leading-5"
                      style={{
                        width: 20,
                        height: 20,
                        background:
                          p.state === 'done'
                            ? 'var(--agent)'
                            : p.state === 'current'
                              ? 'var(--human)'
                              : 'var(--chip-bg)',
                        color: p.state === 'todo' ? 'var(--ink3)' : '#fff',
                      }}
                      aria-hidden
                    >
                      {p.state === 'done' ? '✓' : '●'}
                    </span>
                    <span
                      className="text-[12px]"
                      style={{
                        color: p.state === 'todo' ? 'var(--ink3)' : 'var(--ink)',
                        fontWeight: p.state === 'current' ? 700 : 500,
                      }}
                    >
                      {p.label}
                    </span>
                  </div>
                ))}
                <div className="sep" />
                <div className="kv">
                  开始于<b>{thread.startedAtLabel}</b>
                </div>
                <div className="kv">
                  截止
                  <b style={{ color: 'var(--human)' }}>{thread.dueAtLabel}</b>
                </div>
              </CardBody>
            </Card>

            <div className="flex flex-col gap-2">
              <Button variant="pri" size="block" className="py-3">
                <Check size={14} aria-hidden /> 确认完成
              </Button>
              <Button size="block" className="py-3">
                打回，继续做
              </Button>
            </div>
          </Inset>
        </div>

        {/* ── 输入区 ── */}
        <div
          className="px-5 pb-5 pt-3.5 sm:px-6"
          style={{ borderTop: '1px solid var(--hair2)', paddingBottom: 'max(20px, env(safe-area-inset-bottom))' }}
        >
          <div
            className="flex items-end gap-[11px] rounded-[22px] py-[9px] pl-[18px] pr-[9px]"
            style={{
              background: 'var(--inset-bg)',
              border: '1px solid var(--inset-bd)',
              boxShadow: 'var(--inset-sh)',
            }}
          >
            <label className="sr-only" htmlFor="composer">
              回复这条 thread
            </label>
            <textarea
              id="composer"
              rows={1}
              placeholder="说点什么… 输入 @ 把别的 agent 拉进来关注"
              className="grow resize-none bg-transparent py-2 text-[13.5px] leading-[1.5] outline-none placeholder:opacity-70"
              style={{ color: 'var(--ink)' }}
            />
            <Button variant="pri" aria-label="发送" className="px-3.5 py-2.5">
              <Send size={16} />
            </Button>
          </div>
          <div className="mt-2.5 flex items-center gap-2 pl-1.5">
            <Chip tone="human" size="sm">
              回复会通知 {primary.name} 与 {thread.watchers.length} 位关注者
            </Chip>
          </div>
        </div>
      </Pane>

      {/* <640px：底部 tab 取代侧栏 */}
      <nav
        className="pane fixed inset-x-3 bottom-3 z-10 flex items-center justify-around gap-2 rounded-pill px-3 py-2 md:hidden"
        style={{ paddingBottom: 'max(8px, env(safe-area-inset-bottom))' }}
        aria-label="主导航"
      >
        <Button variant="gh" className="min-h-11 flex-col gap-1 text-[10px]">
          <MessagesSquare size={18} /> 对话
        </Button>
        <Button variant="gh" className="min-h-11 flex-col gap-1 text-[10px]">
          <LayoutGrid size={18} /> 看板
        </Button>
        <Button variant="gh" className="min-h-11 flex-col gap-1 text-[10px]">
          <Megaphone size={18} /> 广播
        </Button>
        <Button variant="gh" className="min-h-11 flex-col gap-1 text-[10px]">
          <Users size={18} /> 名录
        </Button>
      </nav>
    </div>
  )
}
