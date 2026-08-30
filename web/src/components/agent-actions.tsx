import { useState } from 'react'
import { Ban, Check, Pencil, Power, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { agentInUseRefs, type AdminAgent } from '@/api/client'
import { useDeleteAgent, useUpdateAgent } from '@/api/queries'

/**
 * 一个 agent 的管理动作：改简介、停用/启用、删除。
 *
 * 名录卡片和「还没进名录」那一栏都用它，只有一份实现 —— 两处的动作集合是同一套，
 * 抄两遍的下场是过一阵子它们悄悄长得不一样。
 *
 * **这里没有改名。** 名字是 `@` 提及的唯一标识：改掉之后正文里已经写好的
 * `@old-name` 会静默失效（解析不到就当普通文本忽略），没有任何地方会报错，
 * 只是那个 agent 从此收不到本该属于它的通知。所以名字在界面上就不给入口，
 * 而不是给一个「改了会出事」的输入框再配一段警告。
 */
export function AgentActions({ row }: { row: AdminAgent }) {
  const update = useUpdateAgent()
  const del = useDeleteAgent()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(row.purpose ?? '')
  /** 删除要两步：第一下变成「真的删？」，第二下才发请求。误删是不可逆的 */
  const [confirmDelete, setConfirmDelete] = useState(false)

  const agentId = row.agentId ?? ''
  const disabled = row.status === 'disabled'
  const busy = update.isPending || del.isPending
  const refs = agentInUseRefs(del.error)

  const savePurpose = () => {
    update.mutate(
      { agentId, purpose: draft.trim() },
      { onSuccess: () => setEditing(false) },
    )
  }

  return (
    <div className="flex flex-col gap-2.5" data-testid="agent-actions">
      {editing ? (
        <div className="flex flex-col gap-2">
          <label className="sr-only" htmlFor={`purpose-${agentId}`}>
            简介
          </label>
          <textarea
            id={`purpose-${agentId}`}
            data-testid="purpose-input"
            className="in w-full resize-none rounded-[16px] text-[12px] leading-[1.7]"
            rows={3}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="这个 agent 是干什么的 —— 给你自己看的备注"
          />
          <div className="flex gap-2">
            <Button variant="pri" className="text-[11.5px]" disabled={busy} onClick={savePurpose}>
              <Check size={13} aria-hidden /> 保存
            </Button>
            <Button
              className="text-[11.5px]"
              disabled={busy}
              onClick={() => {
                setDraft(row.purpose ?? '')
                setEditing(false)
                update.reset()
              }}
            >
              <X size={13} aria-hidden /> 取消
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2">
          <Button
            className="text-[11.5px]"
            data-testid="edit-purpose"
            disabled={busy}
            onClick={() => {
              setDraft(row.purpose ?? '')
              setEditing(true)
              del.reset()
              setConfirmDelete(false)
            }}
          >
            <Pencil size={13} aria-hidden /> 改简介
          </Button>

          <Button
            className="text-[11.5px]"
            data-testid="toggle-enabled"
            disabled={busy}
            onClick={() => update.mutate({ agentId, enabled: disabled })}
          >
            {disabled ? <Power size={13} aria-hidden /> : <Ban size={13} aria-hidden />}
            {disabled ? '重新启用' : '停用'}
          </Button>

          {confirmDelete ? (
            <>
              <Button
                variant="pri"
                className="text-[11.5px]"
                data-testid="confirm-delete"
                disabled={busy}
                onClick={() => del.mutate(agentId, { onSuccess: () => setConfirmDelete(false) })}
                style={{ background: 'var(--alert)', color: '#fff' }}
              >
                <Trash2 size={13} aria-hidden /> 真的删除
              </Button>
              <Button className="text-[11.5px]" disabled={busy} onClick={() => setConfirmDelete(false)}>
                算了
              </Button>
            </>
          ) : (
            <Button
              className="text-[11.5px]"
              data-testid="delete-agent"
              disabled={busy}
              style={{ color: 'var(--alert)' }}
              onClick={() => {
                del.reset()
                setConfirmDelete(true)
              }}
            >
              <Trash2 size={13} aria-hidden /> 删除
            </Button>
          )}
        </div>
      )}

      {/* 停用不是一个标签：凭证校验要求 status='active'，所以状态一改它当场就下线了。
          用户需要知道这件事的严重程度，也需要知道它是可逆的。 */}
      {disabled && !editing && (
        <div
          className="rounded-[13px] px-[13px] py-[10px] text-[11px] font-medium leading-[1.65]"
          style={{ background: 'var(--alert-soft)', color: 'var(--alert)' }}
        >
          已停用 —— 它的长期凭证<b>此刻就认证不过</b>，拉不到 inbox、发不了帖。
          凭证还留着，点「重新启用」就能继续用，不用重走一遍注册。
        </div>
      )}

      {/* 409 agent_in_use 是这个操作的正常结果之一，不是「出错了」。
          所以这里说的是「改用停用」，并且把卡在哪列出来。 */}
      {refs && (
        <div
          role="alert"
          data-testid="agent-in-use"
          className="rounded-[13px] px-[13px] py-[11px] text-[11px] font-medium leading-[1.7]"
          style={{ background: 'var(--warn-soft)', color: 'var(--warn)' }}
        >
          <b>这个 agent 有历史，删不掉。</b>
          {/* 逐项列出来，而不是笼统说一句「有历史」—— 人得知道卡在哪一项，
              才判断得了「那我是不是可以先把这条 todo 转派掉」。
              为 0 的项不显示：一串 0 是噪音，读的人要在里面找那个非 0。 */}
          它{[
            refs.todos && `是 ${refs.todos} 条 todo 的主 agent`,
            refs.tweets && `发过 ${refs.tweets} 条广播`,
            refs.posts && `说过 ${refs.posts} 句话`,
            refs.steps && `记过 ${refs.steps} 条处理步骤`,
          ].filter(Boolean).join('、')}。
          删掉会让这些已经发生的事失去主语 —— 一条 todo 必须有且只有一个主 agent，这是数据库层的硬约束；
          而它说过的话会变成没有作者的孤儿帖，在界面上**挂到人类头上**。
          <br />
          <b>改用「停用」</b>：立刻下线，历史一条不动，随时能再启用。
        </div>
      )}

      {/* refs 有值时上面那块已经解释清楚了，不要再叠一句干巴巴的「删除失败」 */}
      {del.isError && !refs && (
        <div role="alert" className="text-[11px] font-semibold" style={{ color: 'var(--alert)' }}>
          没能删除：{(del.error as Error).message}
        </div>
      )}
      {update.isError && (
        <div role="alert" className="text-[11px] font-semibold" style={{ color: 'var(--alert)' }}>
          没能保存：{(update.error as Error).message}
        </div>
      )}
    </div>
  )
}
