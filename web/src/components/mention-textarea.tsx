import { useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'
import { Avatar } from '@/components/ui/avatar'
import type { AgentSummary } from '@/api/client'
import { initialsOf, latencyLabel, tierLabel } from '@/lib/format'
import { cn } from '@/lib/cn'
import { caretViewportPoint } from '@/lib/caret'

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
   * 下拉往哪边开 —— 都是**贴着光标那一行**开，不是贴着输入框的边。
   * composer 贴在页面最下沿，往下开会整块落到视口外面，只能往上。
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
  const menuRef = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState<string | null>(null)
  const [active, setActive] = useState(0)
  /** 光标落点（相对外层定位容器）。null = 还没量出来，退回贴输入框左上角 */
  const [at, setAt] = useState<{ left: number; top: number; lineHeight: number } | null>(null)

  const matches =
    query === null
      ? []
      : agents.filter((a) => (a.name ?? '').toLowerCase().startsWith(query.toLowerCase())).slice(0, 6)
  const open = query !== null && matches.length > 0

  // useLayoutEffect 而不是 useEffect：popover 要在出现的那一帧就在光标底下。
  // 用 useEffect 会先在左上角画一帧再跳过去 —— 打字是高频操作，那一跳很刺眼。
  useLayoutEffect(() => {
    if (!open) return
    const place = () => {
      const el = ref.current
      if (!el) return
      const pt = caretViewportPoint(el, el.selectionStart ?? value.length)
      // 贴着光标开，但不能捅出视口右沿 —— 在行尾敲 `@` 是最常见的情况，
      // 不夹一下的话 popover 有一半在屏幕外面。夹的是左边缘，不改宽度。
      const w = menuRef.current?.offsetWidth ?? 0
      const room = window.innerWidth - w - 8
      setAt({
        left: room > 0 ? Math.min(Math.max(pt.left, 8), room) : 8,
        top: pt.top,
        lineHeight: pt.lineHeight,
      })
    }
    place()
    // position:fixed 的坐标是视口坐标，页面一滚它就不在光标底下了。
    // capture 是必须的：真正在滚的是内板（.stream），滚动事件不冒泡到 window。
    const onScroll = () => place()
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', onScroll)
    return () => {
      window.removeEventListener('scroll', onScroll, true)
      window.removeEventListener('resize', onScroll)
    }
  }, [open, value, query])

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
      {/*
        **挂在 body 上，不是挂在输入框旁边。** 玻璃板 `.pane` 是 `overflow:hidden`
        （§2 的构图靠它裁圆角），popover 只要比板子高出一点点就被切掉半截 ——
        在 thread 的 composer 里几乎必然发生。portal + `position:fixed` 是唯一
        不受任何祖先 overflow / transform 影响的做法。
        无障碍关系靠 id（aria-controls / aria-expanded）维持，跨 portal 照样成立。
      */}
      {open &&
        createPortal(
        <div
          ref={menuRef}
          id="mention-list"
          role="listbox"
          aria-label="@ 提及"
          data-testid="mention-list"
          className="mention-pop"
          style={
            // 视口坐标。往下开落在光标这一行的下沿，往上开压在这一行的上沿。
            // 量不出来（jsdom 不做布局）就退回 (0,0)，功能一点不少 —— 位置是增强。
            placement === 'top'
              ? { left: at?.left ?? 0, bottom: at ? window.innerHeight - at.top + 6 : 0 }
              : { left: at?.left ?? 0, top: (at ? at.top + at.lineHeight : 0) + 6 }
          }
        >
          <div className="mention-pop-hd">@ 提及 · 只是拉人关注，不指派</div>
          {matches.map((a, i) => (
            <button
              key={a.agentId}
              type="button"
              role="option"
              aria-selected={i === active}
              className="mention-pop-item"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pick(a.name ?? '')}
            >
              <Avatar kind="agent" size="sm" initials={initialsOf(a.name)} online={a.online} />
              <span className="min-w-0 flex-1">
                <span className="mention-pop-name">{a.name}</span>
                <span className="mention-pop-sub">
                  <span className="mono">{a.runtime}</span> · {tierLabel(a.tier)}
                </span>
              </span>
              <span className="mention-pop-lat">{latencyLabel(a.typicalLatencySeconds)}</span>
            </button>
          ))}
        </div>,
          document.body,
        )}
    </div>
  )
}
