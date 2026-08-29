import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { AlertTriangle } from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { AppShell, PageHeader } from '@/components/app-shell'
import { MentionTextarea } from '@/components/mention-textarea'
import { OutboxBanner } from '@/components/outbox-banner'
import { useCreateTodo, useDirectory } from '@/api/queries'
import { initialsOf, latencyLabel, tierLabel } from '@/lib/format'

export default function NewTodoRoute() {
  const navigate = useNavigate()
  const { data: agents } = useDirectory()
  const create = useCreateTodo()
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  /** 必选且唯一 —— 不选就不给提交。数据库层也是 primary_agent_id NOT NULL。 */
  const [primaryAgentId, setPrimaryAgentId] = useState<string | null>(null)

  // 按响应速度排序：谁最快回你，谁排前面
  const candidates = useMemo(
    () =>
      [...(agents ?? [])].sort(
        (a, b) => (a.typicalLatencySeconds ?? 1e9) - (b.typicalLatencySeconds ?? 1e9),
      ),
    [agents],
  )
  const primary = candidates.find((a) => a.agentId === primaryAgentId)
  const mentioned = (agents ?? []).filter(
    (a) => a.name && new RegExp(`@${a.name}\\b`).test(body) && a.agentId !== primaryAgentId,
  )

  const canSubmit = !!primaryAgentId && title.trim().length > 0 && !create.isPending

  const submit = () => {
    if (!canSubmit) return
    create.mutate(
      { title: title.trim(), body, primaryAgentId: primaryAgentId! },
      { onSuccess: (r) => navigate(r?.threadId ? `/threads/${r.threadId}` : '/todos') },
    )
  }

  return (
    <AppShell>
      <OutboxBanner />
      <PageHeader
        title="开一个新 thread"
        subtitle="一条 todo 就是一个 thread · 主 agent 必选，@ 只产生关注者"
        actions={
          <>
            <Button variant="gh" onClick={() => navigate(-1)}>
              取消
            </Button>
            <Button
              variant="pri"
              data-testid="create-todo"
              disabled={!canSubmit}
              aria-disabled={!canSubmit}
              onClick={submit}
            >
              创建并通知
            </Button>
          </>
        }
      />

      <div className="flex min-h-0 min-w-0 grow flex-col gap-3.5 px-3.5 pb-3.5 lg:flex-row">
        <Inset className="stream flex min-w-0 grow flex-col gap-5 overflow-y-auto p-5 sm:p-[22px]">
          <Card>
            <CardBody className="gap-5 p-[22px_24px]">
              <label className="sr-only" htmlFor="todo-title">
                标题
              </label>
              <input
                id="todo-title"
                className="in border-none bg-transparent p-0 text-[17px] font-bold leading-[1.4] shadow-none"
                placeholder="一句话说清要做什么"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
              <div className="sep" />
              <label className="sr-only" htmlFor="todo-body">
                正文
              </label>
              <MentionTextarea
                id="todo-body"
                aria-label="正文"
                value={body}
                onChange={setBody}
                agents={agents ?? []}
                placeholder="背景、约束、验收标准… 输入 @ 把别的 agent 拉进来关注"
              />
            </CardBody>
          </Card>

          <Card>
            <CardBody className="gap-[15px] p-[20px_24px]">
              <div className="flex items-center gap-2.5">
                <span className="text-[13.5px] font-bold">谁来负责</span>
                <Chip tone="alert" size="sm">
                  必选 · 只能一个
                </Chip>
                <span className="ml-auto text-[11px] font-medium" style={{ color: 'var(--ink3)' }}>
                  按响应速度排序
                </span>
              </div>

              <div
                className="flex gap-1.5 overflow-x-auto pb-1"
                role="radiogroup"
                aria-label="主 agent"
                aria-required="true"
              >
                {candidates.map((a) => {
                  const on = a.agentId === primaryAgentId
                  return (
                    <button
                      key={a.agentId}
                      type="button"
                      role="radio"
                      aria-checked={on}
                      aria-label={`主 agent ${a.name}`}
                      onClick={() => setPrimaryAgentId(a.agentId ?? null)}
                      className="flex min-w-[104px] shrink-0 flex-col items-center gap-2 rounded-[16px] px-2.5 py-[13px]"
                      style={
                        on
                          ? {
                              background: 'var(--agent-soft)',
                              boxShadow: 'inset 0 0 0 2px var(--agent)',
                            }
                          : undefined
                      }
                    >
                      <Avatar
                        kind={on ? 'primary' : 'agent'}
                        initials={initialsOf(a.name)}
                        online={a.online}
                      />
                      <span className="text-center">
                        <span className="block text-[12px] font-bold leading-none">{a.name}</span>
                        <span
                          className="mt-1.5 block text-[9.5px] font-medium leading-[1.4]"
                          style={{ color: on ? 'var(--agent-ink)' : 'var(--ink3)' }}
                        >
                          <span className="mono">{a.runtime}</span>
                          <br />
                          {a.online ? latencyLabel(a.typicalLatencySeconds) : '离线'}
                        </span>
                      </span>
                    </button>
                  )
                })}
              </div>

              {!primaryAgentId && (
                <div
                  data-testid="primary-required"
                  className="rounded-[14px] px-[15px] py-[13px] text-[11.5px] font-medium leading-[1.7]"
                  style={{ background: 'var(--alert-soft)', color: 'var(--alert)' }}
                >
                  还没选主 agent —— 不选无法提交。一件事必须有且只有一个负责人，否则要么没人管，要么互相等。
                </div>
              )}

              {primary && !primary.online && (
                <div
                  className="flex items-start gap-2.5 rounded-[14px] px-[15px] py-[13px]"
                  style={{ background: 'var(--warn-soft)' }}
                >
                  <span style={{ color: 'var(--warn)' }} className="mt-0.5 shrink-0">
                    <AlertTriangle size={14} aria-hidden />
                  </span>
                  <span
                    className="text-[11.5px] font-medium leading-[1.7]"
                    style={{ color: 'var(--warn)' }}
                  >
                    {primary.name} 现在离线也可以选 —— 事件会堆在它的 inbox 里，上线后按 cursor
                    一条不少地送达。只是你该知道自己在等什么。
                  </span>
                </div>
              )}
            </CardBody>
          </Card>
        </Inset>

        <Inset className="hidden shrink-0 flex-col gap-[13px] overflow-y-auto p-[18px] sm:flex lg:w-[292px]">
          <Card>
            <CardHeader>两条硬规则</CardHeader>
            <CardBody className="gap-3.5">
              <div>
                <div className="text-[12px] font-bold leading-none">必须选一个主 agent</div>
                <div
                  className="mt-[7px] text-[11px] font-medium leading-[1.7]"
                  style={{ color: 'var(--ink2)' }}
                >
                  不选就无法提交。数据库层 `primary_agent_id NOT NULL` 也拦一遍。
                </div>
              </div>
              <div className="sep" />
              <div>
                <div className="text-[12px] font-bold leading-none">@ 不等于指派</div>
                <div
                  className="mt-[7px] text-[11px] font-medium leading-[1.7]"
                  style={{ color: 'var(--ink2)' }}
                >
                  正文里 @ 的 agent 只成为关注者：收通知、订阅更新，不要求回复。@
                  的正好是主 agent 也不会重复入队。
                </div>
              </div>
            </CardBody>
          </Card>

          <Card>
            <CardHeader>创建后会发生什么</CardHeader>
            <CardBody className="gap-3">
              {[
                primary
                  ? `${primary.name} 收到 todo.assigned（P0），进它的队列`
                  : '选定主 agent 后，它会收到 todo.assigned（P0）',
                mentioned.length
                  ? `${mentioned.map((m) => m.name).join('、')} 收到 todo.mentioned（P1），只关注不入队`
                  : '正文里 @ 到的 agent 会收到 todo.mentioned（P1），只关注不入队',
                'thread 建好，主 agent 的首次回复把状态推进到「澄清中」',
              ].map((text, i) => (
                <div key={i} className="flex items-start gap-2.5">
                  <span
                    className="shrink-0 rounded-pill text-center text-[10px] font-bold leading-[19px]"
                    style={{
                      width: 19,
                      height: 19,
                      background: 'var(--agent-soft)',
                      color: 'var(--agent-ink)',
                    }}
                    aria-hidden
                  >
                    {i + 1}
                  </span>
                  <span
                    className="text-[11px] font-medium leading-[1.65]"
                    style={{ color: 'var(--ink2)' }}
                  >
                    {text}
                  </span>
                </div>
              ))}
            </CardBody>
          </Card>

          <Card>
            <CardHeader>档位</CardHeader>
            <CardBody>
              <div className="kv">
                主 agent
                <b>{primary ? `@${primary.name}` : '未选'}</b>
              </div>
              <div className="kv">
                档位<b>{tierLabel(primary?.tier)}</b>
              </div>
              <div className="kv">
                关注者<b>{mentioned.length}</b>
              </div>
            </CardBody>
          </Card>
        </Inset>
      </div>
    </AppShell>
  )
}
