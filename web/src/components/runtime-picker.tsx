import { useLayoutEffect, useRef, useState } from 'react'
import { cn } from '@/lib/cn'

/**
 * 可选的 runtime。**这份清单必须和 connector 的注册表对齐**
 * （`connector/src/adapters/registry.ts` 的 builtinAdapters），因为它唯一的用途
 * 就是拼进接入命令的 `RUNTIME=` —— 这里多一个注册表里没有的值，用户复制过去
 * 只会撞上「不认识的 RUNTIME」。
 *
 * `id` 用全称（claude-code），不是产品名（claude）：connector 认两种写法，
 * 但生成的命令用全称，免得别人照着这条命令去查文档时对不上。
 */
export interface RuntimeOption {
  id: string
  /** 界面上的名字。用产品名，人认的是这个 */
  label: string
  /** 形态决定要不要 RUNTIME_URL，不是审美分类 */
  form: 'cli' | 'service'
  /** 一句话说清它是什么、要准备什么 */
  hint: string
}

export const RUNTIMES: RuntimeOption[] = [
  { id: 'claude-code', label: 'Claude Code', form: 'cli', hint: '本机要有 `claude` 命令 · 同 thread 续接会话' },
  { id: 'codex', label: 'Codex', form: 'cli', hint: '本机要有 `codex` 命令 · 同 thread 续接会话' },
  { id: 'opencode', label: 'OpenCode', form: 'cli', hint: '本机要有 `opencode` 命令 · 同 thread 续接会话' },
  { id: 'openclaw', label: 'OpenClaw', form: 'cli', hint: '还要给 SUBCOMMAND —— 本项目不替你猜' },
  { id: 'hermes', label: 'Hermes', form: 'service', hint: '常驻服务 · 先配好 Webhook 通道拿到 URL' },
  { id: 'openhuman', label: 'OpenHuman', form: 'service', hint: '常驻服务 · 建一个 webhook 触发的工作流拿 URL' },
  { id: 'generic-shell', label: '其它（shell）', form: 'cli', hint: '兜底：给一条命令模板，事件 JSON 走 stdin' },
]

export function runtimeById(id: string): RuntimeOption | undefined {
  return RUNTIMES.find((r) => r.id === id)
}

/**
 * runtime 选择器：一块会滑到选中项底下的玻璃滑块。
 *
 * **为什么不用下拉菜单**：下拉把选项藏起来，而这里的选项本身带信息量 ——
 * 「它是 CLI 还是常驻服务」直接决定了接下来要不要准备一个 webhook URL。
 * 藏起来的结果是用户选完才发现还差一样东西。
 *
 * **为什么滑块位置靠测量而不是 `translateX(idx * 100%)`**：那种算法要求所有项
 * 等宽且永不换行。这一行有 7 个选项，窄屏必然换行，一换行纯 CSS 的算法就
 * 整个错位。测量选中元素的实际盒子是唯一在换行、响应式、任意项数下都成立的做法。
 *
 * 动效走 CSS transition，`prefers-reduced-motion` 的全局兜底会把它关掉（§1.5），
 * 这里不绕过它 —— 关掉之后滑块直接跳到位，功能一点不少。
 */
export function RuntimePicker({
  value,
  onChange,
  className,
  'aria-label': ariaLabel = '选择 runtime',
}: {
  value: string
  onChange: (id: string) => void
  className?: string
  'aria-label'?: string
}) {
  const trackRef = useRef<HTMLDivElement>(null)
  const [thumb, setThumb] = useState<{ left: number; top: number; w: number; h: number } | null>(null)

  // useLayoutEffect 而不是 useEffect：滑块要在这一帧就位，用 useEffect 会先画一帧
  // 在左上角的滑块再跳过去，看起来像闪了一下。
  useLayoutEffect(() => {
    const track = trackRef.current
    if (!track) return
    const place = () => {
      const el = track.querySelector<HTMLElement>('[aria-checked="true"]')
      if (!el) return setThumb(null)
      setThumb({ left: el.offsetLeft, top: el.offsetTop, w: el.offsetWidth, h: el.offsetHeight })
    }
    place()
    // 换行位置随宽度变，光在 value 变化时算一次是不够的。
    // 特性检测不是为了 jsdom：**滑块跟不跟随 resize 是增强，不是这个控件能不能用的前提**。
    // 缺了 ResizeObserver 就退回「只在选中项变化时定位一次」，选择功能一点不少；
    // 直接 new 的话，任何没有它的环境会在渲染期抛异常，整个表单白屏。
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(place)
    ro.observe(track)
    return () => ro.disconnect()
  }, [value])

  /** 左右方向键在选项间走 —— radiogroup 的标准交互，缺了它键盘用户只能 Tab 穿过整组 */
  const onKeyDown = (e: React.KeyboardEvent) => {
    const dir = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1
      : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0
    if (!dir) return
    e.preventDefault()
    const i = RUNTIMES.findIndex((r) => r.id === value)
    onChange(RUNTIMES[(i + dir + RUNTIMES.length) % RUNTIMES.length].id)
  }

  const picked = runtimeById(value)

  return (
    <div className={cn('flex flex-col gap-2.5', className)}>
      <div
        ref={trackRef}
        role="radiogroup"
        aria-label={ariaLabel}
        data-testid="runtime-picker"
        onKeyDown={onKeyDown}
        className="rpick"
      >
        {thumb && (
          <span
            aria-hidden
            className="rpick-thumb"
            style={{ left: thumb.left, top: thumb.top, width: thumb.w, height: thumb.h }}
          />
        )}
        {RUNTIMES.map((r) => {
          const on = r.id === value
          return (
            <button
              key={r.id}
              type="button"
              role="radio"
              aria-checked={on}
              // 只有选中项进 Tab 序 —— radiogroup 是一个控件，不是 7 个
              tabIndex={on ? 0 : -1}
              data-testid={`runtime-${r.id}`}
              className="rpick-item"
              onClick={() => onChange(r.id)}
            >
              {r.label}
            </button>
          )
        })}
      </div>
      {picked && (
        <p
          className="m-0 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] font-medium leading-[1.6]"
          style={{ color: 'var(--ink3)' }}
          data-testid="runtime-hint"
        >
          <span
            className="rounded-pill px-2 py-[3px] text-[9.5px] font-bold"
            style={{
              background: 'var(--chip-bg)',
              color: picked.form === 'service' ? 'var(--human)' : 'var(--agent-ink)',
            }}
          >
            {picked.form === 'service' ? '常驻服务' : '命令行'}
          </span>
          {picked.hint}
        </p>
      )}
    </div>
  )
}
