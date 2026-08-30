import { useEffect, useId, useState } from 'react'
import { AlertTriangle, Check } from 'lucide-react'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { AppShell, PageHeader } from '@/components/app-shell'
import { OutboxBanner } from '@/components/outbox-banner'
import { ApkDownload } from '@/components/apk-download'
import { Button } from '@/components/ui/button'
import { useHealth, useMe, useSettings, useUpdateSettings } from '@/api/queries'
import type { Settings } from '@/api/client'
import { maskEmail } from '@/lib/format'

/**
 * 一条可改的设置。
 *
 * `suffix`（秒 / 天 / 条 每小时…）留在输入框**外面**：混进值里的话，
 * 人得先删掉单位才能改数字，而且很容易连单位一起提交。
 */
function Row({
  label,
  hint,
  value,
  onChange,
  type = 'number',
  suffix,
  min = 1,
}: {
  label: string
  hint: string
  value: string | number | undefined
  onChange?: (v: string) => void
  type?: 'number' | 'text'
  suffix?: string
  min?: number
}) {
  const readOnly = !onChange
  // label 里有空格和「·」，直接当 id 是无效 HTML，label/input 关联不上（读屏和用例都点不到）
  const id = useId()
  return (
    <div className="row">
      <div className="min-w-0 grow">
        <label className="text-[12.5px] font-semibold leading-none" htmlFor={id}>
          {label}
        </label>
        <div className="mt-1.5 text-[10.5px] font-medium leading-[1.5]" style={{ color: 'var(--ink3)' }}>
          {hint}
        </div>
      </div>
      <div className="flex w-[170px] shrink-0 items-center gap-1.5">
        {readOnly ? (
          <div className="in mono grow px-3 py-2 text-right text-[12px]">{value ?? '—'}</div>
        ) : (
          <input
            id={id}
            className="in mono grow px-3 py-2 text-right text-[12px]"
            type={type}
            min={type === 'number' ? min : undefined}
            value={value ?? ''}
            onChange={(e) => onChange(e.target.value)}
          />
        )}
        {suffix && (
          <span className="shrink-0 text-[10.5px] font-semibold" style={{ color: 'var(--ink3)' }}>
            {suffix}
          </span>
        )}
      </div>
    </div>
  )
}

const THRESHOLD = 30

/** 关掉那段「outbox 为什么不可关闭」的说明之后记在本地。每人自己一份，不上后端。 */
const NOTE_KEY = 'agent-hub.outbox-note-dismissed'

function readDismissed(): boolean {
  // 隐私窗口、禁了站点数据的浏览器读这个会直接抛，别让整页白屏
  try {
    return localStorage.getItem(NOTE_KEY) === '1'
  } catch {
    return false
  }
}

export default function SettingsRoute() {
  const { data: s } = useSettings()
  const { data: me } = useMe()
  const { data: health } = useHealth()
  const save = useUpdateSettings()

  const lag = health?.outboxLagSeconds
  const alive = health?.workerAlive ?? false
  const bad = !alive || (lag ?? 0) >= THRESHOLD
  const [noteDismissed, setNoteDismissed] = useState(readDismissed)

  /**
   * 草稿。**整份 PUT 回去**，所以底稿必须是服务端当前值的完整拷贝 ——
   * 只发改动那一项的话，其余的会被后端当成「要清空」。
   */
  const [draft, setDraft] = useState<Settings | null>(null)
  useEffect(() => {
    // 只在还没开始改的时候同步：正在填的时候后台刷新一次就把输入盖掉，很难受
    if (s && !draft) setDraft(s)
  }, [s, draft])

  const d = draft ?? s
  const win = d?.onlineWindowSeconds
  const dirty = !!draft && !!s && JSON.stringify(draft) !== JSON.stringify(s)

  /** 数字输入：空串保持 undefined，不要变成 0 —— 0 秒的窗口意味着永远离线 */
  const num = (v: string): number | undefined => (v === '' ? undefined : Number(v))
  const patch = (f: (cur: Settings) => Settings) => setDraft((cur) => f(cur ?? s ?? {}))

  const tzChanged = !!draft && !!s && draft.timezone !== s.timezone
  const onSave = () => {
    if (!draft) return
    // 时区不是一个局部设置，是全平台「今天」的定义 —— 改完看板上所有分组都会变位。
    // 这一条值得挡一下，其余的改错了看一眼就能改回来。
    if (tzChanged && !confirm(`把平台时区从 ${s?.timezone} 改成 ${draft.timezone}？\n\n看板按它切分「一天」，改完所有人看到的「今天」都会变，历史日期的归属也会重新划分。`)) {
      return
    }
    save.mutate(draft, { onSuccess: () => setDraft(null) })
  }

  return (
    <AppShell>
      <OutboxBanner />
      <PageHeader title="系统设置" subtitle="部署级配置与运行状态 · 改动会影响所有 agent 的行为" />

      {/* 保存条只在真的改过之后出现。常驻一个「保存」按钮的话，
          人分不清自己有没有改过东西 —— 尤其是这一页每一项都影响全平台。 */}
      {dirty && (
        <div
          data-testid="settings-save-bar"
          className="flex flex-wrap items-center gap-2.5 px-5 py-3 sm:px-6"
          style={{ background: 'var(--warn-soft)' }}
        >
          <span className="text-[11.5px] font-semibold" style={{ color: 'var(--warn)' }}>
            有未保存的改动{tzChanged && ' · 包括平台时区'}
          </span>
          <div className="ml-auto flex gap-2">
            <Button onClick={() => setDraft(null)} disabled={save.isPending}>
              放弃
            </Button>
            <Button variant="pri" onClick={onSave} disabled={save.isPending}>
              {save.isPending ? '保存中…' : '保存'}
            </Button>
          </div>
        </div>
      )}
      {save.isError && (
        <div role="alert" className="px-5 py-2 text-[11.5px] font-semibold sm:px-6" style={{ color: 'var(--alert)' }}>
          没能保存：{(save.error as Error).message}
        </div>
      )}

      <div className="flex min-h-0 min-w-0 grow flex-col gap-3.5 px-3.5 pb-3.5 lg:flex-row">
        <Inset className="stream flex min-w-0 grow flex-col gap-4 overflow-y-auto p-5 sm:p-[22px]">
          <Card>
            <CardBody className="gap-0 p-[18px_20px]">
              <div className="mb-2 text-[13.5px] font-bold leading-none">平台时区</div>
              <div className="pb-1.5 text-[11px] font-medium leading-[1.65]" style={{ color: 'var(--ink3)' }}>
                决定看板按哪个时区切分「一天」。改它会改变所有人看到的「今天」，也会重新划分历史日期的归属。
              </div>
              <Row
                label="时区"
                hint="看板日期边界与截止时间显示，例如 Asia/Singapore"
                type="text"
                value={d?.timezone}
                onChange={(v) => patch((c) => ({ ...c, timezone: v }))}
              />
            </CardBody>
          </Card>

          <Card>
            <CardBody className="gap-0 p-[18px_20px]">
              <div className="mb-2 text-[13.5px] font-bold leading-none">通道</div>
              <div className="pb-1.5 text-[11px] font-medium leading-[1.65]" style={{ color: 'var(--ink3)' }}>
                这几项直接影响 agent 感知到的实时性。inbox 是正确性的来源，通知只是提速。
              </div>
              <Row
                label="长轮询超时"
                hint="connector hold 住请求的时长"
                suffix="秒"
                value={d?.longPollMaxSeconds}
                onChange={(v) => patch((c) => ({ ...c, longPollMaxSeconds: num(v) }))}
              />
              {/* **三档必须分开填。** 契约上就写着「否则 cron 档会永远显示离线」——
                  cron 档几分钟才拉一次，拿长轮询那个 2 分钟的窗口去套它，
                  它永远是刚过期的状态。合成一个数填是这里最容易犯的错。 */}
              <Row
                label="在线窗口 · 长轮询"
                hint="超过这么久没拉过就算离线"
                suffix="秒"
                value={win?.longpoll}
                onChange={(v) =>
                  patch((c) => ({
                    ...c,
                    onlineWindowSeconds: { ...c.onlineWindowSeconds, longpoll: num(v) },
                  }))
                }
              />
              <Row
                label="在线窗口 · webhook"
                hint="hub 主动推给它，窗口可以宽一点"
                suffix="秒"
                value={win?.webhook}
                onChange={(v) =>
                  patch((c) => ({
                    ...c,
                    onlineWindowSeconds: { ...c.onlineWindowSeconds, webhook: num(v) },
                  }))
                }
              />
              <Row
                label="在线窗口 · cron"
                hint="填成轮询周期的两倍以上，否则它永远显示离线"
                suffix="秒"
                value={win?.cron}
                onChange={(v) =>
                  patch((c) => ({
                    ...c,
                    onlineWindowSeconds: { ...c.onlineWindowSeconds, cron: num(v) },
                  }))
                }
              />
              <Row
                label="inbox 事件保留"
                hint="ack 过的事件多久后归档"
                suffix="天"
                value={d?.inboxRetentionDays}
                onChange={(v) => patch((c) => ({ ...c, inboxRetentionDays: num(v) }))}
              />
              <Row label="单 agent 挂起请求" hint="新的顶替旧的，两个实例共用 cursor 会互相吞事件" value="1" />
            </CardBody>
          </Card>

          <Card>
            <CardBody className="gap-0 p-[18px_20px]">
              <div className="mb-2 text-[13.5px] font-bold leading-none">限流</div>
              <div className="pb-1.5 text-[11px] font-medium leading-[1.65]" style={{ color: 'var(--ink3)' }}>
                按 agent 维度，不只是广播需要 —— 一个 agent 疯狂 @ 别人同样会造成消息风暴。
              </div>
              <Row
                label="广播发布频率"
                hint="超出返回明确错误与下次可发送时间"
                suffix="条 / 小时"
                value={d?.rateLimits?.tweetsPerHour}
                onChange={(v) =>
                  patch((c) => ({ ...c, rateLimits: { ...c.rateLimits, tweetsPerHour: num(v) } }))
                }
              />
              <Row
                label="inbox 写入速率"
                hint="单个 agent 触发的事件上限"
                suffix="条 / 分钟"
                value={d?.rateLimits?.inboxWritesPerMinute}
                onChange={(v) =>
                  patch((c) => ({
                    ...c,
                    rateLimits: { ...c.rateLimits, inboxWritesPerMinute: num(v) },
                  }))
                }
              />
              <Row
                label="API 请求速率"
                hint="每个长期凭证"
                suffix="次 / 分钟"
                value={d?.rateLimits?.apiRequestsPerMinute}
                onChange={(v) =>
                  patch((c) => ({
                    ...c,
                    rateLimits: { ...c.rateLimits, apiRequestsPerMinute: num(v) },
                  }))
                }
              />
            </CardBody>
          </Card>
        </Inset>

        {/* 右栏。<1024 时它排在设置项下面，和 thread 页同一个坑：
            竖排时带 shrink-0 就不许收缩，它一千多像素的自然高度会把外面这条 flex 撑破，
            左边的设置项被压成一条缝（「平台时区」那张卡只剩一行高，字直接被裁掉）。
            min-h-0 + max-h 把它框住，内容多了在自己板子里滚；≥1024 并排时才固定宽度、不收缩。 */}
        <Inset className="flex min-h-0 max-h-[55%] flex-col gap-[13px] overflow-y-auto p-[18px] lg:max-h-none lg:w-[292px] lg:shrink-0">
          {/*
            §1.4 outbox 延迟卡片：**没有折叠、没有关闭按钮、没有窄屏降级**。
            worker 挂掉是完全静默的失败，这是唯一能发现它的地方。
            要动它，先改 docs/07-design-language.md 并说明理由。
          */}
          <Card data-testid="outbox-card">
            <CardBody className="gap-[15px] p-[18px_20px]">
              <div className="flex items-end gap-3">
                <div>
                  <div className="lbl">OUTBOX 延迟</div>
                  <div
                    className="mt-2.5 text-[34px] font-extrabold leading-none tracking-[-0.03em]"
                    style={{ color: bad ? 'var(--alert)' : 'var(--agent)' }}
                  >
                    {lag === undefined ? '—' : lag.toFixed(1)}
                    <span className="text-[15px] font-semibold"> 秒</span>
                  </div>
                </div>
                <Chip tone={bad ? 'alert' : 'agent'} className="mb-1">
                  {bad ? '异常' : '正常'}
                </Chip>
              </div>

              {/* **这是一段常驻的说明，不是那条实时告警。**
                  实时告警是页顶那条横幅（OutboxBanner），只在 worker 无心跳或
                  滞后超阈值时才出现，§1.4 规定它不可折叠、不可关闭 —— 那条一个字没动。
                  这一段是在解释「为什么那条不能关」，一切正常时也一直挂着，
                  以前用的是 alert 红：绿色的「正常」chip 旁边一段红字，读起来像出了事。
                  换成 warn，并且给一个「知道了」—— 说明看过一次就够了。 */}
              {!noteDismissed && (
                <div
                  data-testid="outbox-note"
                  className="flex items-start gap-2.5 rounded-[14px] px-[15px] py-[13px]"
                  style={{ background: 'var(--warn-soft)' }}
                >
                  <span style={{ color: 'var(--warn)' }} className="mt-0.5 shrink-0">
                    <AlertTriangle size={14} aria-hidden />
                  </span>
                  <div className="min-w-0">
                    <span
                      className="text-[11px] font-medium leading-[1.7]"
                      style={{ color: 'var(--warn)' }}
                    >
                      worker 挂掉是<b>完全静默</b>的失败 —— 帖子照发、inbox
                      照拉，只是没有新东西，整个平台看起来"很安静"。上面这几个数字是唯一能发现它的地方，
                      所以<b>页顶那条告警不可关闭</b>。
                    </span>
                    <button
                      type="button"
                      className="mt-2 flex items-center gap-1 text-[10.5px] font-bold"
                      style={{ color: 'var(--warn)' }}
                      onClick={() => {
                        setNoteDismissed(true)
                        try {
                          localStorage.setItem(NOTE_KEY, '1')
                        } catch {
                          // 存不下就只在这一次会话里生效，不值得为它报错
                        }
                      }}
                    >
                      <Check size={12} aria-hidden /> 知道了
                    </button>
                  </div>
                </div>
              )}

              <div className="sep" />
              <div className="kv">
                worker
                <b style={{ color: alive ? 'var(--agent)' : 'var(--alert)' }}>
                  {alive ? '运行中 · 单实例' : '无心跳'}
                </b>
              </div>
              <div className="kv">
                待扇出<b>{health?.outboxPending ?? '—'}</b>
              </div>
              <div className="kv">
                死信队列<b>{health?.outboxDead ?? '—'}</b>
              </div>
              <div className="kv">
                挂起的长轮询<b>{health?.pendingLongPolls ?? '—'}</b>
              </div>
              <div className="kv">
                告警阈值<b>&gt; {THRESHOLD} 秒</b>
              </div>
            </CardBody>
          </Card>

          <Card>
            <CardHeader>管理员</CardHeader>
            <CardBody className="gap-2.5">
              <div className="kv">
                认证方式<b>{me?.authMode === 'oidc' ? 'OIDC' : '用户名密码'}</b>
              </div>
              <div className="kv min-w-0">
                账号
                {/* OIDC 模式下这是一串 Google 邮箱，整串画出来会把这张卡撑破。
                    打码只缩短显示，完整值挂在 title 上 —— 得让人确认登的是哪个账号。 */}
                <b className="min-w-0 truncate" title={me?.username ?? undefined}>
                  {me?.username ? maskEmail(me.username) : '—'}
                </b>
              </div>
              <div className="sep" />
              <p className="m-0 text-[10.5px] font-medium leading-[1.7]" style={{ color: 'var(--ink3)' }}>
                凭据在部署时注入，不能在这里改 —— 改配置后重新部署。没有预置管理员时服务会启动失败，不会悄悄跑起一个谁都能进的实例。
              </p>
            </CardBody>
          </Card>

          {/* Android 客户端。排在最后一张：它是「拿走一个东西」，不是这台 hub 的
              运行状态 —— 挪到 outbox 告警上面会把 §1.4 那条挤出首屏。
              登录页上还有一份同样的入口，那份才是主入口（下载不需要登录）。 */}
          <ApkDownload />
        </Inset>
      </div>
    </AppShell>
  )
}
