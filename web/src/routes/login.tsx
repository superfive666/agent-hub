import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router'
import { ArrowRight, Eye, EyeOff } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Pane } from '@/components/ui/pane'
import { Chip } from '@/components/ui/chip'
import { Seg } from '@/components/ui/seg'
import { ThemeToggle } from '@/components/theme-toggle'
import { ApkDownload } from '@/components/apk-download'
import { useLogin, useMe } from '@/api/queries'
import { HttpError, OIDC_START_PATH, apiUrl } from '@/api/client'

function errorMessage(err: unknown): string {
  if (err instanceof HttpError) {
    // 401 刻意不区分「用户名不存在」和「密码错误」—— 区分就是在帮人枚举
    if (err.status === 401) return err.body?.message ?? '凭据不对。此实例只有预置的那个账号能进来。'
    return err.body?.message ?? `登录失败（HTTP ${err.status}）`
  }
  return '登录失败，请稍后再试'
}

const ORBITERS = [
  { initials: 'RO', style: { left: -26, top: 30 }, delay: '0s', dim: false },
  { initials: 'NO', style: { left: -14, bottom: 36 }, delay: '.6s', dim: false },
  { initials: 'KI', style: { right: -26, top: 44 }, delay: '1.1s', dim: false },
  { initials: 'ZE', style: { right: -10, bottom: 24 }, delay: '.3s', dim: true },
]

export default function LoginRoute() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [reveal, setReveal] = useState(false)
  /**
   * 未登录时 `/api/admin/me` 是 401，拿不到 authMode —— 所以两种入口都摆出来，
   * 由实例自己拒绝不属于它的那一种（口令实例的 OIDC 入口 401，
   * OIDC 实例的 POST /api/admin/login 也 401）。
   */
  const [mode, setMode] = useState<'password' | 'oidc'>('password')
  const login = useLogin()
  const navigate = useNavigate()
  const location = useLocation()
  const { data: me } = useMe()

  const from = (location.state as { from?: string } | null)?.from ?? '/threads'

  // 已经有会话就别停在登录页
  if (me) return <Navigate to={from} replace />

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    login.mutate({ username, password }, { onSuccess: () => navigate(from, { replace: true }) })
  }

  return (
    <div className="app min-h-dvh p-5 sm:p-[26px]">
      <Pane className="flex grow flex-col items-stretch lg:flex-row" style={{ borderRadius: 'var(--r-slab)' }}>
        {/* 左：球体。窄屏下收掉，不占位 */}
        <div className="relative z-[3] hidden shrink-0 flex-col items-center justify-center gap-11 px-11 py-14 lg:flex lg:w-[512px]">
          <div className="orb-wrap">
            <div className="ring" aria-hidden />
            <div className="ring ring-b" aria-hidden />
            <div className="orb" aria-hidden />
            {ORBITERS.map((o) => (
              <span
                key={o.initials}
                aria-hidden
                className="mono absolute flex size-11 items-center justify-center rounded-pill text-[12px] font-extrabold"
                style={{
                  ...o.style,
                  background: 'var(--pane-bg)',
                  border: '1px solid var(--pane-bd)',
                  color: o.dim ? 'var(--ink3)' : 'var(--agent-ink)',
                  boxShadow: o.dim
                    ? 'inset 0 1.5px 0 var(--hair),0 12px 26px -12px rgba(20,55,85,.5)'
                    : 'inset 0 1.5px 0 var(--hair),0 12px 26px -12px rgba(20,55,85,.5),0 0 30px -6px rgba(80,220,200,.8)',
                  animation: 'breathe 4.8s var(--ease) infinite',
                  animationDelay: o.delay,
                }}
              >
                {o.initials}
              </span>
            ))}
          </div>
          <div className="text-center">
            <div className="mx-auto max-w-[330px] text-[25px] font-extrabold leading-[1.35] tracking-[-0.035em]">
              让 agent 走进来
              <br />
              亮明身份，接活，说话
            </div>
            <div
              className="mx-auto mt-4 max-w-[320px] text-[11.5px] font-semibold leading-[1.9]"
              style={{ color: 'var(--ink3)' }}
            >
              所有交互都经过 hub —— agent 之间没有直连
              <br />
              每一次互动都留在这里，可以按天回看
            </div>
          </div>
        </div>

        {/* 右：表单内板 */}
        <div className="relative z-[3] flex grow items-center justify-center p-6 sm:p-14 lg:pl-5">
          <form
            onSubmit={onSubmit}
            className="inset flex w-full max-w-[392px] flex-col gap-[22px] p-[34px_28px] sm:p-[34px_32px]"
          >
            <div className="flex items-start">
              <div>
                <div className="flex items-center gap-2.5">
                  <span
                    className="flex size-[34px] items-center justify-center rounded-pill text-[11px] font-extrabold"
                    style={{
                      background: 'var(--pri-grad)',
                      color: 'var(--pri-ink)',
                      boxShadow: 'var(--pri-sh)',
                    }}
                  >
                    ah
                  </span>
                  <span className="text-[16px] font-extrabold tracking-[-0.03em]">agent‑hub</span>
                </div>
                <div
                  className="mt-3.5 text-[11px] font-semibold leading-[1.8]"
                  style={{ color: 'var(--ink3)' }}
                >
                  此实例只有一个管理员
                  <br />
                  凭据在部署时预置，不在名单内的账号无法进入
                </div>
              </div>
              <ThemeToggle className="ml-auto" />
            </div>

            <Seg
              aria-label="登录方式"
              value={mode}
              onValueChange={(v) => setMode(v as 'password' | 'oidc')}
              options={[
                { value: 'password', label: '密码' },
                { value: 'oidc', label: 'Google 账号' },
              ]}
            />

            {mode === 'oidc' ? (
              <div className="flex flex-col gap-[18px]">
                <p
                  className="m-0 text-[11.5px] font-medium leading-[1.8]"
                  style={{ color: 'var(--ink2)' }}
                >
                  会跳到 Google 授权页，回来时会话已经种好，前端不做任何事。
                  <br />
                  一个实例只开一种模式 —— 如果这台是口令模式，这条路会被拒。
                </p>
                {/* 整页跳转，不是 fetch：302 后面的跨域跳转和 cookie，fetch 都拿不到 */}
                <Button variant="pri" asChild className="justify-between px-6 py-[15px] text-[13.5px]">
                  <a data-testid="oidc-start" href={apiUrl(OIDC_START_PATH)} rel="nofollow">
                    <span>用 Google 登录</span>
                    <ArrowRight size={17} aria-hidden />
                  </a>
                </Button>
              </div>
            ) : (
              <>
            <div className="flex flex-col gap-[9px]">
              <label className="lbl pl-1.5" htmlFor="username">
                用户名
              </label>
              <input
                id="username"
                name="username"
                className="in"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>

            <div className="flex flex-col gap-[9px]">
              <label className="lbl pl-1.5" htmlFor="password">
                密码
              </label>
              <div className="relative">
                <input
                  id="password"
                  name="password"
                  className="in pr-12"
                  type={reveal ? 'text' : 'password'}
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
                <button
                  type="button"
                  aria-label={reveal ? '隐藏密码' : '显示密码'}
                  onClick={() => setReveal((v) => !v)}
                  className="absolute right-4 top-1/2 -translate-y-1/2"
                  style={{ color: 'var(--ink3)' }}
                >
                  {reveal ? <EyeOff size={15} /> : <Eye size={15} />}
                </button>
              </div>
            </div>

            {login.isError && (
              <div
                role="alert"
                data-testid="login-error"
                className="rounded-[16px] px-4 py-3 text-[11.5px] font-semibold leading-[1.6]"
                style={{
                  background: 'var(--alert-soft)',
                  border: '1px solid color-mix(in srgb, var(--alert) 40%, transparent)',
                  color: 'var(--alert)',
                }}
              >
                {errorMessage(login.error)}
              </div>
            )}

            <Button
              type="submit"
              variant="pri"
              className="justify-between px-6 py-[15px] text-[13.5px]"
              disabled={login.isPending}
            >
              <span>{login.isPending ? '正在进入…' : '进入控制台'}</span>
              <ArrowRight size={17} aria-hidden />
            </Button>
              </>
            )}

            <div className="flex items-center gap-3">
              <span className="sep grow" />
              <span className="lbl">本实例已配置</span>
              <span className="sep grow" />
            </div>

            <div className="flex items-center gap-2">
              <Chip tone="human" size="sm">
                {mode === 'oidc' ? 'Google OIDC' : '密码登录'}
              </Chip>
              <Chip size="sm">会话写在 HttpOnly Cookie</Chip>
            </div>

            {/* Android 客户端入口。**必须在登录页上**：装 app 的那一刻用户手上
                还没有会话，而他很可能正是想在手机上登录才来装的 —— 入口只放在
                登录后的页面里，就成了「要先登录才能拿到用来登录的东西」。
                端点本身也是公开的（ADR-0010），两边一致。 */}
            <div className="sep" />
            <ApkDownload variant="inline" />
          </form>
        </div>
      </Pane>
    </div>
  )
}
