import { useMemo, useState } from 'react'
import { useParams } from 'react-router'
import type { Post } from '@/api/client'
import { Check, Send } from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { AppShell, PageHeader } from '@/components/app-shell'
import { MessageRow } from '@/components/message-row'
import { OutboxBanner } from '@/components/outbox-banner'
import {
  useCreatePost,
  useDirectory,
  useThread,
  useTodoAction,
  useTodos,
} from '@/api/queries'
import {
  dateTimeLabel,
  dayLabel,
  initialsOf,
  latencyLabel,
  progressOf,
  statusLabel,
  tierLabel,
} from '@/lib/format'

export default function ThreadRoute() {
  const params = useParams<{ threadId?: string }>()
  const { data: todos } = useTodos()
  const threadId = params.threadId ?? todos?.[0]?.threadId
  const { data: thread, isPending, isError, error } = useThread(threadId)
  const { data: agents } = useDirectory()
  const [draft, setDraft] = useState('')
  const post = useCreatePost(threadId)
  const act = useTodoAction(threadId)

  const primaryId = thread?.primaryAgentId
  const primaryWatcher = thread?.watchers?.find((w) => w.reason === 'primary')
  const primaryCard = agents?.find((a) => a.agentId === primaryId)
  const primaryName = primaryWatcher?.name ?? primaryCard?.name ?? '—'
  const watchers = (thread?.watchers ?? []).filter((w) => w.reason !== 'primary')
  const progress = progressOf(thread?.status)

  // 按天插分隔条：thread 会跨多天，没有分隔就分不清"今天"发生了什么
  const stream = useMemo(() => {
    const out: { key: string; day?: string; post?: Post }[] = []
    let lastDay = ''
    for (const p of thread?.posts ?? []) {
      const d = dayLabel(p.createdAt)
      if (d !== lastDay) {
        out.push({ key: `day-${d}`, day: d })
        lastDay = d
      }
      out.push({ key: p.id, post: p })
    }
    return out
  }, [thread])

  const send = () => {
    const body = draft.trim()
    if (!body) return
    post.mutate({ body }, { onSuccess: () => setDraft('') })
  }

  return (
    <AppShell activeThreadId={threadId}>
      <OutboxBanner />
      <PageHeader
        title={thread?.title ?? (isPending ? '加载中…' : '对话')}
        subtitle={
          thread ? (
            <>
              <span className="mono">{thread.threadId}</span> · 开始于{' '}
              {dateTimeLabel(thread.startedAt)} · {thread.watchers.length} 个 agent 在这个 thread 里
            </>
          ) : undefined
        }
        actions={thread?.status ? <Chip tone="human">{statusLabel(thread.status)}</Chip> : null}
      />

      {isError && (
        <div role="alert" className="px-5 pb-3 text-[12px] font-semibold sm:px-6" style={{ color: 'var(--alert)' }}>
          读不到这条 thread：{(error as Error).message}
        </div>
      )}

      {/* <640px：右栏内容压成 thread 顶部状态带（§4） */}
      {thread && (
        <div className="flex shrink-0 items-center gap-2 overflow-x-auto px-5 pb-3 sm:hidden">
          <Chip tone="agent" size="sm">
            主 agent @{primaryName}
          </Chip>
          <Chip size="sm">关注者 {watchers.length}</Chip>
          {thread.dueAt && <Chip size="sm">截止 {dateTimeLabel(thread.dueAt)}</Chip>}
        </div>
      )}

      {/* ── 嵌套内板：消息流 + 右详情。≥1024 才并排 ── */}
      <div className="flex min-h-0 min-w-0 grow flex-col gap-3.5 px-3.5 pb-3.5 lg:flex-row">
        <Inset className="stream flex min-w-0 grow flex-col gap-[15px] overflow-y-auto p-5 sm:p-[22px]">
          {stream.map((item) =>
            item.day ? (
              <div key={item.key} className="sys">
                {item.day}
              </div>
            ) : (
              <MessageRow key={item.key} post={item.post!} thread={thread} />
            ),
          )}
          {!isPending && stream.length === 0 && (
            <div className="sys">这条 thread 还没有发言</div>
          )}
        </Inset>

        {thread && (
          <Inset className="hidden shrink-0 flex-col gap-[13px] overflow-y-auto p-[18px] sm:flex lg:w-[292px]">
            {/* 流光只给主 agent 卡片（§1.3） */}
            <Card className="glow runner">
              <CardHeader>主 AGENT · 必须响应</CardHeader>
              <CardBody>
                <div className="flex items-center gap-[11px]">
                  <Avatar
                    kind="primary"
                    initials={initialsOf(primaryName)}
                    online={primaryWatcher?.online ?? primaryCard?.online}
                    label={`@${primaryName}`}
                  />
                  <div>
                    <div className="text-[13.5px] font-bold leading-none">{primaryName}</div>
                    <div
                      className="mt-[5px] text-[10.5px] font-semibold leading-none"
                      style={{ color: 'var(--agent-ink)' }}
                    >
                      {(primaryWatcher?.online ?? primaryCard?.online) ? '在线' : '离线'} ·{' '}
                      {tierLabel(primaryCard?.tier)}
                    </div>
                  </div>
                </div>
                <div className="sep" />
                <div className="kv">
                  runtime
                  <b className="mono text-[11px]">{primaryCard?.runtime ?? '—'}</b>
                </div>
                <div className="kv">
                  典型响应<b>{latencyLabel(primaryCard?.typicalLatencySeconds)}</b>
                </div>
                <p
                  className="m-0 text-[10.5px] font-medium leading-[1.65]"
                  style={{ color: 'var(--ink3)' }}
                >
                  一条 todo 有且只有一个主 agent。转派要在 thread 里留痕。
                </p>
              </CardBody>
            </Card>

            <Card>
              <CardHeader>关注者 · {watchers.length} · 不必回复</CardHeader>
              <CardBody>
                {watchers.map((w) => (
                  <div key={w.agentId} className="flex items-center gap-2.5">
                    <Avatar
                      kind="agent"
                      size="sm"
                      initials={initialsOf(w.name)}
                      online={w.online}
                      label={`@${w.name}`}
                    />
                    <div className="min-w-0">
                      <div className="text-[12px] font-semibold leading-none">{w.name}</div>
                      <div className="mt-1 text-[10px] font-medium" style={{ color: 'var(--ink3)' }}>
                        {w.reason === 'mentioned' ? '正文被 @' : '回过帖'}
                      </div>
                    </div>
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
                  开始于<b>{dateTimeLabel(thread.startedAt)}</b>
                </div>
                {thread.dueAt && (
                  <div className="kv">
                    截止
                    <b style={{ color: 'var(--human)' }}>{dateTimeLabel(thread.dueAt)}</b>
                  </div>
                )}
              </CardBody>
            </Card>

            {thread.status === 'awaiting_review' && (
              <div className="flex flex-col gap-2">
                <Button
                  variant="pri"
                  size="block"
                  className="py-3"
                  disabled={act.isPending}
                  onClick={() => act.mutate('confirm')}
                >
                  <Check size={14} aria-hidden /> 确认完成
                </Button>
                <Button
                  size="block"
                  className="py-3"
                  disabled={act.isPending}
                  onClick={() => act.mutate('reject')}
                >
                  打回，继续做
                </Button>
                {act.isError && (
                  <span role="alert" className="text-[10.5px] font-semibold" style={{ color: 'var(--alert)' }}>
                    {(act.error as Error).message}
                  </span>
                )}
              </div>
            )}
          </Inset>
        )}
      </div>

      {/* ── 输入区：管理员发言 authorKind=admin，界面据此靠右 ── */}
      <div
        className="shrink-0 px-5 pb-5 pt-3.5 sm:px-6"
        style={{
          borderTop: '1px solid var(--hair2)',
          // 这一条要是不透明的底：它压在会滚动的消息流上方，
          // 透太多的话滚过去的字会从底下浮上来和输入框叠在一起。
          background: 'var(--composer-bar)',
          backdropFilter: 'blur(18px)',
          WebkitBackdropFilter: 'blur(18px)',
          paddingBottom: 'max(20px, env(safe-area-inset-bottom))',
        }}
      >
        <div
          className="flex items-end gap-[11px] rounded-[22px] py-[9px] pl-[18px] pr-[9px]"
          style={{
            background: 'var(--composer-bg)',
            border: '1px solid var(--composer-bd)',
            boxShadow: 'var(--inset-sh)',
          }}
        >
          <label className="sr-only" htmlFor="composer">
            回复这条 thread
          </label>
          <textarea
            id="composer"
            rows={1}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="说点什么… 输入 @ 把别的 agent 拉进来关注"
            className="grow resize-none bg-transparent py-2 text-[13.5px] leading-[1.5] outline-none placeholder:opacity-70"
            style={{ color: 'var(--ink)' }}
          />
          <Button
            variant="pri"
            aria-label="发送"
            className="px-3.5 py-2.5"
            disabled={!draft.trim() || post.isPending}
            onClick={send}
          >
            <Send size={16} />
          </Button>
        </div>
        {thread && (
          <div className="mt-2.5 flex items-center gap-2 pl-1.5">
            <Chip tone="human" size="sm">
              回复会通知 {primaryName} 与 {watchers.length} 位关注者
            </Chip>
          </div>
        )}
      </div>
    </AppShell>
  )
}
