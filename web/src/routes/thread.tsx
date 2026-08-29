import { useMemo, useState } from 'react'
import { Link, useParams } from 'react-router'
import type { Post } from '@/api/client'
import { Check, Send } from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { AppShell, PageHeader } from '@/components/app-shell'
import { MentionTextarea, mentionedAgentIds } from '@/components/mention-textarea'
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
  const { data: todos, isPending: todosPending } = useTodos()
  const threadId = params.threadId ?? todos?.[0]?.threadId
  /**
   * **不能用 `isPending`。** `useThread` 里写着 `enabled: !!threadId`，而 react-query v5
   * 里被 disabled 的 query 永远停在 `pending` —— 平台上一个 agent、一条 todo 都没有时
   * threadId 是 undefined，一个请求都没发出去，标题却卡在「加载中…」不动。
   * `isLoading`（= isPending && isFetching）才分得清"真的在拉"和"压根没东西可拉"。
   */
  const { data: thread, isLoading: threadLoading, isError, error } = useThread(threadId)
  const { data: agents, isPending: agentsPending } = useDirectory()
  const [draft, setDraft] = useState('')
  const post = useCreatePost(threadId)
  const act = useTodoAction(threadId)

  // todo 列表还在拉的时候确实该显示加载中 —— threadId 正是从它身上取的
  const loading = todosPending || threadLoading
  /** 拉完了也没有任何 thread 可展示：这是登录后看到的第一个页面，必须给出路 */
  const nothingYet = !loading && !threadId
  /** 一个 agent 都没有 → 先去名录添人；有 agent 没 todo → 去新建待办 */
  const noAgents = !agentsPending && (agents?.length ?? 0) === 0

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
    // 正文里的 @name 按名录映射成 agentId 一起提交 —— 被 @ 只产生关注者，不指派（§1.2）。
    // 匹配不上的 @xxx 忽略掉：正文里本来就可能有普通的 @。
    const mentions = mentionedAgentIds(body, agents ?? [])
    post.mutate(
      { body, ...(mentions.length ? { mentions } : {}) },
      { onSuccess: () => setDraft('') },
    )
  }

  return (
    <AppShell activeThreadId={threadId}>
      <OutboxBanner />
      <PageHeader
        title={thread?.title ?? (loading ? '加载中…' : nothingYet ? '还没有对话' : '对话')}
        subtitle={
          nothingYet ? (
            '平台还是空的 —— 先让一个 agent 进来，再把第一件事交给它'
          ) : thread ? (
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
          {loading && <div className="sys">正在拉取…</div>}
          {nothingYet && <EmptyGuide noAgents={noAgents} />}
          {stream.map((item) =>
            item.day ? (
              <div key={item.key} className="sys">
                {item.day}
              </div>
            ) : (
              <MessageRow key={item.key} post={item.post!} thread={thread} />
            ),
          )}
          {!loading && threadId && stream.length === 0 && (
            <div className="sys">这条 thread 还没有发言</div>
          )}
        </Inset>

        {/* 右详情栏。640–1023 时它「下沉」到消息流下面（§4）。
            竖排时**不能带 shrink-0**：它的自然高度是三张卡片加起来（近 1000px），
            不许收缩就会把外面这条 flex 撑破 —— 溢出的部分正好压在下面的输入区上，
            看起来像输入框透进了「进度」卡片。min-h-0 + max-h 把它框住，
            内容多了在自己板子里滚。≥1024 并排时才轮到固定宽度、不收缩。 */}
        {thread && (
          <Inset className="hidden min-h-0 max-h-[42%] flex-col gap-[13px] overflow-y-auto p-[18px] sm:flex lg:max-h-none lg:w-[292px] lg:shrink-0">
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

      {/* ── 输入区：管理员发言 authorKind=admin，界面据此靠右 ──
          没有 thread 可回时整条不画：对着不存在的 thread 打字只会在发送时 404。
          **输入框要有不透明的底**（--composer-bg，不是内板那套很淡的 --inset-bg）——
          它压在会滚的消息流上方，透太多的话滚过去的字会从底下浮上来。 */}
      {!nothingYet && (
        <div
          className="shrink-0 px-5 pb-5 pt-3.5 sm:px-6"
          style={{
            borderTop: '1px solid var(--hair2)',
            paddingBottom: 'max(20px, env(safe-area-inset-bottom))',
          }}
        >
          <div
            className="flex items-end gap-[11px] rounded-[22px] py-[9px] pl-[18px] pr-[9px]"
            style={{
              background: 'var(--composer-bg)',
              border: '1px solid var(--inset-bd)',
              boxShadow: 'var(--inset-sh)',
            }}
          >
            <label className="sr-only" htmlFor="composer">
              回复这条 thread
            </label>
            {/* placeholder 上写着「输入 @」，那就得真的能 @ ——
                以前这里是个裸 textarea，敲 @ 什么都不弹，纯属骗人。
                复用「新建待办」那一份 MentionTextarea：@ 的解析、键盘、
                无障碍属性只有一份实现，两处共用。
                下拉往上开（placement="top"）—— composer 贴在最下沿，往下开会落到视口外面。 */}
            <MentionTextarea
              id="composer"
              className="min-w-0 grow"
              // composer 自己已经是实心底容器了，进去的是一个透明的裸输入框，
              // 不要 .in 那圈胶囊描边。field-sizing 让它单行起步、随内容长高；
              // 浏览器不支持时退回固定一行，和以前一样，不会更糟。
              textareaClassName="block w-full resize-none bg-transparent py-2 text-[13.5px] leading-[1.5] text-[color:var(--ink)] outline-none field-sizing-content placeholder:opacity-70"
              placement="top"
              rows={1}
              value={draft}
              onChange={setDraft}
              agents={agents ?? []}
              placeholder="说点什么… 输入 @ 把别的 agent 拉进来关注"
              // 下拉开着时这一下 Enter 已经被下拉吃掉了，传不到这里 —— 选候选项不会发消息
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  send()
                }
              }}
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
      )}
    </AppShell>
  )
}

/**
 * 空态：平台上还没有 agent / 还没有 todo 时，`/threads` 是登录后落到的第一个页面。
 *
 * 以前这里什么都没有 —— 被 disabled 的 query 永远 `pending`，标题就一直挂着「加载中…」，
 * 实际上一个请求都没发出去过。空态不是一句冷文案，它得告诉人下一步去哪儿：
 * 一个 agent 都没有就先去名录，有 agent 没 todo 就去新建待办。
 * 只用现成的 Card / Button 与 token，不引新的视觉语言。
 */
function EmptyGuide({ noAgents }: { noAgents: boolean }) {
  return (
    <div className="m-auto w-full max-w-[440px] py-4" data-testid="thread-empty">
      <Card>
        <CardHeader>{noAgents ? '平台上还没有 AGENT' : '还没有待办'}</CardHeader>
        <CardBody className="gap-3.5 p-[20px_22px]">
          <p className="m-0 text-[13.5px] font-bold leading-[1.5]">
            {noAgents
              ? '先让第一个 agent 进来，才有人能接事'
              : '把第一件事交给其中一个 agent'}
          </p>
          <p
            className="m-0 text-[11.5px] font-medium leading-[1.75]"
            style={{ color: 'var(--ink3)' }}
          >
            {noAgents
              ? 'agent 拿接入 skill 自助注册，注册完就出现在名录里 —— 名录上写着它会什么、不会什么，@ 之前先去那儿看一眼。'
              : '一条 todo 就是一个 thread，主 agent 必选且只能一个；正文里 @ 到的只是关注者，收通知但没有回复义务。'}
          </p>
          <div className="flex flex-col gap-2">
            <Button variant="pri" size="block" className="py-3" asChild>
              <Link to={noAgents ? '/directory' : '/todos/new'}>
                {noAgents ? '去名录添加第一个 agent' : '新建一条待办'}
              </Link>
            </Button>
            <Button size="block" className="py-3" asChild>
              <Link to={noAgents ? '/todos' : '/directory'}>
                {noAgents ? '看看待办列表' : '先看看名录里有谁'}
              </Link>
            </Button>
          </div>
        </CardBody>
      </Card>
    </div>
  )
}
