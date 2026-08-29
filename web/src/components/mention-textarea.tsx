import { useRef, useState, type KeyboardEvent } from 'react'
import { Avatar } from '@/components/ui/avatar'
import type { AgentSummary } from '@/api/client'
import { initialsOf, latencyLabel, tierLabel } from '@/lib/format'
import { cn } from '@/lib/cn'

/**
 * 光标前那一小段：行首或空白之后的 `@`，后面跟着还在打的名字。
 * `*` 而不是 `+` 是关键 —— 刚敲下 `@`、一个字都还没打时 query 是空串，
 * 空串能前缀匹配任何名字，下拉立刻把全部 agent 摊开给你看。
 * 这正是 facebook 式 @ 的基本盘，写成 `+` 就变成"要先猜对首字母才有提示"。
 */
const TOKEN = /(^|\s)@([A-Za-z0-9_-]*)$/

/** 名字里可能有正则元字符（`.`、`+`），拼进 RegExp 之前先转义。 */
function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * 从正文里解析出被 @ 到的 agent，按名录映射成 agentId。
 *
 * 匹配不上的 `@xxx` **直接忽略，不报错** —— 正文里本来就可能有普通的 @。
 * 后面用 `(?![A-Za-z0-9_-])` 收边，否则 `@nova` 会把 `@nova2` 也算进来。
 */
export function mentionedAgentIds(text: string, agents: AgentSummary[]): string[] {
  const ids: string[] = []
  for (const a of agents) {
    if (!a.name || !a.agentId) continue
    if (new RegExp(`(^|\\s)@${escapeRe(a.name)}(?![A-Za-z0-9_-])`).test(text)) ids.push(a.agentId)
  }
  return ids
}

export interface MentionTextareaProps {
  id?: string
  value: string
  onChange: (v: string) => void
  agents: AgentSummary[]
  placeholder?: string
  rows?: number
  /** 外层定位容器的类名 */
  className?: string
  /**
   * 覆盖 textarea 自己的类名。默认那套（`.in` 胶囊 + 18px 圆角）是给「新建待办」
   * 的多行表单写的；thread 的 composer 已经有自己的实心底容器，进去要的是一个透明的裸输入框。
   * **别为此复制一份组件出来** —— @ 的解析、键盘、无障碍属性只该有一份实现。
   */
  textareaClassName?: string
  /**
   * 下拉往哪边开。composer 贴在页面最下沿，往下开会整块落到视口外面，只能往上。
   */
  placement?: 'bottom' | 'top'
  /**
   * 下拉没有消费掉的按键才交给它。**下拉开着时 Enter / Tab / 上下 / Esc 一律归下拉**，
   * 不会传下来 —— 否则在 composer 里选候选项的那一下会顺手把消息发出去。
   */
  onKeyDown?: (e: KeyboardEvent<HTMLTextAreaElement>) => void
  'aria-label'?: string
}

/**
 * 正文里的 `@` 提及。**只产生关注者，不指派** —— 主 agent 是单独选的那一个。
 * 下拉里的说明文字不是装饰，是这条规则唯一露脸的地方。
 */
export function MentionTextarea({
  id,
  value,
  onChange,
  agents,
  placeholder,
  rows = 5,
  className,
  textareaClassName,
  placement = 'bottom',
  onKeyDown: onKeyDownOuter,
  ...rest
}: MentionTextareaProps) {
  const ref = useRef<HTMLTextAreaElement>(null)
  const [query, setQuery] = useState<string | null>(null)
  const [active, setActive] = useState(0)

  const matches =
    query === null
      ? []
      : agents.filter((a) => (a.name ?? '').toLowerCase().startsWith(query.toLowerCase())).slice(0, 6)
  const open = query !== null && matches.length > 0

  const sync = (next: string, caret: number) => {
    const m = TOKEN.exec(next.slice(0, caret))
    setQuery(m ? m[2] : null)
    setActive(0)
  }

  const pick = (name: string) => {
    const el = ref.current
    const caret = el?.selectionStart ?? value.length
    const head = value.slice(0, caret).replace(TOKEN, `$1@${name} `)
    const next = head + value.slice(caret)
    onChange(next)
    setQuery(null)
    queueMicrotask(() => {
      el?.focus()
      el?.setSelectionRange(head.length, head.length)
    })
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // 输入法组词过程中的 Enter 是"选这个候选词"，既不是选 agent 也不是发送。
    // 中文输入下这一下按得最多，误判一次就是把半句话发出去。
    if (e.nativeEvent.isComposing) return
    // 下拉开着时这几个键归下拉，**不再往外传**：composer 上 Enter 是"发送"，
    // 传下去就成了"选中候选项的同时把消息发出去"。
    if (open) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActive((i) => (i + 1) % matches.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActive((i) => (i - 1 + matches.length) % matches.length)
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        pick(matches[active].name ?? '')
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setQuery(null)
        return
      }
    }
    onKeyDownOuter?.(e)
  }

  return (
    <div className={cn('relative', className)}>
      <textarea
        id={id}
        ref={ref}
        rows={rows}
        value={value}
        placeholder={placeholder}
        aria-label={rest['aria-label']}
        aria-expanded={open}
        aria-controls={open ? 'mention-list' : undefined}
        className={textareaClassName ?? 'in w-full resize-none rounded-[18px] leading-[1.75]'}
        onChange={(e) => {
          onChange(e.target.value)
          sync(e.target.value, e.target.selectionStart)
        }}
        onKeyDown={onKeyDown}
        onBlur={() => setTimeout(() => setQuery(null), 120)}
      />
      {open && (
        <div
          id="mention-list"
          role="listbox"
          aria-label="@ 提及"
          data-testid="mention-list"
          className={cn(
            'absolute left-4 z-10 w-[280px] overflow-hidden rounded-[16px]',
            placement === 'top' ? 'bottom-full mb-1.5' : 'top-full mt-1.5',
          )}
          style={{
            background: 'var(--pane-bg)',
            border: '1px solid var(--pane-bd)',
            boxShadow: 'var(--pane-sh)',
            backdropFilter: 'blur(18px) saturate(180%)',
          }}
        >
          <div className="lbl px-3.5 py-2.5">@ 提及 · 只是拉人关注，不指派</div>
          {matches.map((a, i) => (
            <button
              key={a.agentId}
              type="button"
              role="option"
              aria-selected={i === active}
              className="flex w-full items-center gap-2.5 px-3.5 py-2.5 text-left"
              style={{ background: i === active ? 'var(--chip-bg)' : 'transparent' }}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pick(a.name ?? '')}
            >
              <Avatar kind="agent" size="sm" initials={initialsOf(a.name)} online={a.online} />
              <span className="min-w-0">
                <span className="block text-[12px] font-semibold leading-none">{a.name}</span>
                <span
                  className="mt-1 block text-[9.5px] font-medium leading-none"
                  style={{ color: 'var(--ink3)' }}
                >
                  <span className="mono">{a.runtime}</span> · {tierLabel(a.tier)}
                </span>
              </span>
              <span
                className="ml-auto text-[9.5px] font-medium"
                style={{ color: 'var(--ink3)' }}
              >
                {latencyLabel(a.typicalLatencySeconds)}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
