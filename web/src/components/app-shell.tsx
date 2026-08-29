import type { ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router'
import {
  LayoutGrid,
  ListChecks,
  LogOut,
  MessagesSquare,
  Search,
  Settings as SettingsIcon,
  Users,
} from 'lucide-react'
import { Avatar } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Pane } from '@/components/ui/pane'
import { Seg } from '@/components/ui/seg'
import { ThemeToggle } from '@/components/theme-toggle'
import { useLogout, useMe, useTodos } from '@/api/queries'
import { initialsOf, statusLabel, timeLabel } from '@/lib/format'
import { cn } from '@/lib/cn'

const NAV = [
  { value: '/threads', label: '对话', icon: MessagesSquare },
  { value: '/board', label: '看板', icon: LayoutGrid },
  { value: '/todos', label: '待办', icon: ListChecks },
  { value: '/directory', label: '名录', icon: Users },
] as const

function navValueOf(pathname: string): string {
  return NAV.find((n) => pathname.startsWith(n.value))?.value ?? '/threads'
}

/**
 * 舞台 → 玻璃板 的底盘（设计语言 §2）。
 *
 * **注意**：`.pane` 的 ::before 是棱镜边、::after 是高光扫过，两个伪元素都被占了。
 * 千万不要往这里的 Pane 上加 `.glow` / `.runner` —— 棱镜边会被静默顶掉。
 * 语义流光只挂 `.card`（主 agent）与 `.convo`（当前会话）。
 */
export function AppShell({
  children,
  activeThreadId,
}: {
  children: ReactNode
  /** 当前会话，决定 rail 里哪一行带流光 */
  activeThreadId?: string
}) {
  const location = useLocation()
  const navigate = useNavigate()
  const nav = navValueOf(location.pathname)
  const { data: me } = useMe()
  const { data: todos } = useTodos()
  const logout = useLogout()

  const meName = me?.username ?? '管理员'

  return (
    <div className="app min-h-dvh">
      {/* ── 玻璃板 1：左会话栏。<640 收起，640–1023 只剩图标条 ── */}
      <Pane className="hidden shrink-0 flex-col py-[18px] sm:flex sm:w-[86px] lg:w-[278px]">
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
            value={nav}
            onValueChange={(v) => navigate(v)}
            options={NAV.map((n) => ({ value: n.value, label: n.label }))}
          />
        </div>
        {/* 640–1023：侧栏收成图标条 */}
        <div className="flex flex-col items-center gap-2 pb-3 lg:hidden">
          {NAV.map((n) => (
            <Button
              key={n.value}
              variant="gh"
              size="icoSm"
              aria-label={n.label}
              aria-current={nav === n.value ? 'page' : undefined}
              data-active={nav === n.value}
              onClick={() => navigate(n.value)}
            >
              <n.icon size={17} />
            </Button>
          ))}
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
          <span className="lbl px-3 pb-1.5 pt-2">进行中 · {todos?.length ?? 0}</span>
          <div className="flex flex-col gap-0.5">
            {(todos ?? []).map((t) => {
              const active = t.threadId === activeThreadId
              return (
                <Link
                  key={t.threadId}
                  to={`/threads/${t.threadId}`}
                  // 流光只挂当前会话（§1.3）。别的行不许带。
                  className={cn('convo', active && 'glow')}
                  data-active={active}
                  aria-current={active ? 'true' : undefined}
                >
                  <Avatar
                    kind={active ? 'primary' : 'agent'}
                    size="sm"
                    initials={initialsOf(t.primaryAgentName)}
                    online={t.primaryAgentOnline}
                    label={`@${t.primaryAgentName ?? ''}`}
                  />
                  <span className="min-w-0 grow">
                    <span className="block truncate text-[12.5px] font-bold leading-[1.35]">
                      {t.title}
                    </span>
                    <span
                      className="mt-1 block truncate text-[11px] font-medium leading-[1.4]"
                      style={{ color: 'var(--ink3)' }}
                    >
                      @{t.primaryAgentName} · {statusLabel(t.status)}
                    </span>
                  </span>
                  <span className="flex shrink-0 flex-col items-end gap-1.5">
                    <span className="text-[10px] font-medium" style={{ color: 'var(--ink3)' }}>
                      {timeLabel(t.updatedAt ?? t.startedAt)}
                    </span>
                    {!!t.replyCount && (
                      <span
                        className="rounded-pill text-center text-[10px] font-bold leading-[18px]"
                        style={{ width: 18, height: 18, background: 'var(--agent)', color: '#fff' }}
                      >
                        {t.replyCount}
                      </span>
                    )}
                  </span>
                </Link>
              )
            })}
          </div>
        </div>

        <div className="mt-auto flex flex-col gap-2.5 px-4 pt-3">
          <div className="sep hidden lg:block" />
          <div className="flex items-center gap-2.5">
            <Avatar kind="human" size="sm" initials={initialsOf(meName)} label={meName} />
            <div className="hidden min-w-0 lg:block">
              <div className="text-[12px] font-bold leading-none">{meName}</div>
              <div className="mt-1 text-[10px] font-medium" style={{ color: 'var(--human)' }}>
                唯一管理员
              </div>
            </div>
            <Button
              variant="gh"
              size="icoSm"
              className="ml-auto hidden lg:inline-flex"
              aria-label="系统设置"
              onClick={() => navigate('/settings')}
            >
              <SettingsIcon size={14} />
            </Button>
            <Button
              variant="gh"
              size="icoSm"
              className="hidden lg:inline-flex"
              aria-label="退出登录"
              onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login') })}
            >
              <LogOut size={14} />
            </Button>
          </div>
        </div>
      </Pane>

      {/* ── 玻璃板 2：主区 ── */}
      <Pane className="flex min-w-0 grow flex-col pb-16 sm:pb-0">{children}</Pane>

      {/* <640px：底部 tab 取代侧栏 */}
      <nav
        className="pane fixed inset-x-3 bottom-3 z-10 flex items-center justify-around gap-2 rounded-pill px-3 py-2 sm:hidden"
        style={{ paddingBottom: 'max(8px, env(safe-area-inset-bottom))' }}
        aria-label="主导航"
      >
        {NAV.map((n) => (
          <Button
            key={n.value}
            variant="gh"
            className="min-h-11 flex-col gap-1 text-[10px]"
            aria-current={nav === n.value ? 'page' : undefined}
            onClick={() => navigate(n.value)}
          >
            <n.icon size={18} /> {n.label}
          </Button>
        ))}
      </nav>
    </div>
  )
}

/** 主区顶部：标题 + 副标题 + 右侧操作。所有页面共用。 */
export function PageHeader({
  title,
  subtitle,
  actions,
}: {
  title: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="relative z-[3] flex items-center gap-3.5 px-5 pb-4 pt-5 sm:px-6">
      <div className="min-w-0">
        <h1 className="m-0 truncate text-[19px] font-extrabold leading-[1.25] tracking-[-0.03em]">
          {title}
        </h1>
        {subtitle && (
          <div className="mt-1.5 text-[11.5px] font-medium" style={{ color: 'var(--ink3)' }}>
            {subtitle}
          </div>
        )}
      </div>
      <div className="ml-auto flex shrink-0 items-center gap-2.5">
        {actions}
        <ThemeToggle className="lg:hidden" />
      </div>
    </header>
  )
}
