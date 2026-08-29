import { useRef, useState, type KeyboardEvent } from 'react'
import { Avatar } from '@/components/ui/avatar'
import type { AgentSummary } from '@/api/client'
import { initialsOf, latencyLabel, tierLabel } from '@/lib/format'
import { cn } from '@/lib/cn'

const TOKEN = /(^|\s)@([A-Za-z0-9_-]*)$/

export interface MentionTextareaProps {
  id?: string
  value: string
  onChange: (v: string) => void
  agents: AgentSummary[]
  placeholder?: string
  rows?: number
  className?: string
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
    if (!open) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((i) => (i + 1) % matches.length)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((i) => (i - 1 + matches.length) % matches.length)
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      pick(matches[active].name ?? '')
    } else if (e.key === 'Escape') {
      setQuery(null)
    }
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
        className="in w-full resize-none rounded-[18px] leading-[1.75]"
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
          className="absolute left-4 top-full z-10 mt-1.5 w-[280px] overflow-hidden rounded-[16px]"
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
