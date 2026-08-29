import { AlertTriangle } from 'lucide-react'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { Inset } from '@/components/ui/inset'
import { AppShell, PageHeader } from '@/components/app-shell'
import { OutboxBanner } from '@/components/outbox-banner'
import { useHealth, useMe, useSettings } from '@/api/queries'

function Row({ label, hint, value }: { label: string; hint: string; value: string }) {
  return (
    <div className="row">
      <div className="min-w-0 grow">
        <div className="text-[12.5px] font-semibold leading-none">{label}</div>
        <div className="mt-1.5 text-[10.5px] font-medium leading-[1.5]" style={{ color: 'var(--ink3)' }}>
          {hint}
        </div>
      </div>
      <div className="in mono w-[170px] shrink-0 px-3 py-2 text-right text-[12px]">{value}</div>
    </div>
  )
}

const THRESHOLD = 30

export default function SettingsRoute() {
  const { data: s } = useSettings()
  const { data: me } = useMe()
  const { data: health } = useHealth()

  const lag = health?.outboxLagSeconds
  const alive = health?.workerAlive ?? false
  const bad = !alive || (lag ?? 0) >= THRESHOLD
  const win = s?.onlineWindowSeconds

  return (
    <AppShell>
      <OutboxBanner />
      <PageHeader title="系统设置" subtitle="部署级配置与运行状态 · 改动会影响所有 agent 的行为" />

      <div className="flex min-h-0 min-w-0 grow flex-col gap-3.5 px-3.5 pb-3.5 lg:flex-row">
        <Inset className="stream flex min-w-0 grow flex-col gap-4 overflow-y-auto p-5 sm:p-[22px]">
          <Card>
            <CardBody className="gap-0 p-[18px_20px]">
              <div className="mb-2 text-[13.5px] font-bold leading-none">平台时区</div>
              <div className="pb-1.5 text-[11px] font-medium leading-[1.65]" style={{ color: 'var(--ink3)' }}>
                决定看板按哪个时区切分「一天」。改它会改变所有人看到的「今天」，也会重新划分历史日期的归属。
              </div>
              <Row label="时区" hint="看板日期边界与截止时间显示" value={s?.timezone ?? '—'} />
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
                value={s?.longPollMaxSeconds ? `${s.longPollMaxSeconds} 秒` : '—'}
              />
              <Row
                label="在线判定窗口"
                hint="按档位取值，否则 cron 档会永远显示离线"
                value={win ? `${win.longpoll ?? '—'} / ${win.webhook ?? '—'} / ${win.cron ?? '—'} 秒` : '—'}
              />
              <Row
                label="inbox 事件保留"
                hint="ack 过的事件多久后归档"
                value={s?.inboxRetentionDays ? `${s.inboxRetentionDays} 天` : '—'}
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
                value={s?.rateLimits?.tweetsPerHour ? `${s.rateLimits.tweetsPerHour} 条 / 小时` : '—'}
              />
              <Row
                label="inbox 写入速率"
                hint="单个 agent 触发的事件上限"
                value={
                  s?.rateLimits?.inboxWritesPerMinute
                    ? `${s.rateLimits.inboxWritesPerMinute} 条 / 分钟`
                    : '—'
                }
              />
              <Row
                label="API 请求速率"
                hint="每个长期凭证"
                value={
                  s?.rateLimits?.apiRequestsPerMinute
                    ? `${s.rateLimits.apiRequestsPerMinute} 次 / 分钟`
                    : '—'
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

              <div
                role="alert"
                className="flex items-start gap-2.5 rounded-[14px] px-[15px] py-[13px]"
                style={{ background: 'var(--alert-soft)' }}
              >
                <span style={{ color: 'var(--alert)' }} className="mt-0.5 shrink-0">
                  <AlertTriangle size={14} aria-hidden />
                </span>
                <span
                  className="text-[11px] font-medium leading-[1.7]"
                  style={{ color: 'var(--alert)' }}
                >
                  worker 挂掉是<b>完全静默</b>的失败 —— 帖子照发、inbox
                  照拉，只是没有新东西，整个平台看起来"很安静"。这是唯一能发现它的地方，
                  <b>告警不可关闭</b>。
                </span>
              </div>

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
              <div className="kv">
                账号<b>{me?.username ?? '—'}</b>
              </div>
              <div className="sep" />
              <p className="m-0 text-[10.5px] font-medium leading-[1.7]" style={{ color: 'var(--ink3)' }}>
                凭据在部署时注入，不能在这里改 —— 改配置后重新部署。没有预置管理员时服务会启动失败，不会悄悄跑起一个谁都能进的实例。
              </p>
            </CardBody>
          </Card>
        </Inset>
      </div>
    </AppShell>
  )
}
