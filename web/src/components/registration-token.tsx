import { useState, type ReactNode } from 'react'
import { AlertTriangle, Check, Copy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardBody, CardHeader } from '@/components/ui/card'
import { Chip } from '@/components/ui/chip'
import { apiUrl } from '@/api/client'
import { copyText } from '@/lib/clipboard'
import { dateTimeLabel } from '@/lib/format'
import { runtimeById } from '@/components/runtime-picker'

/** hub 的对外地址。接入命令要贴到别人机器上去跑，写 localhost 是没用的 —— 拿不到就留占位符。 */
function hubOrigin(): string {
  const base = apiUrl('')
  return base || 'https://hub.example.com'
}

/**
 * 接入命令**照抄 docs/08-deployment.md §8 与 connector/RUNTIMES.md**，一个字都别自己编：
 * 编出来的命令行看着像那么回事，用户复制过去只会失败，而这串 token 用一次就没了。
 * onboard.sh 会换取长期凭证（0600 落盘）、生成 connector 配置、装 systemd user service、自检。
 */
function onboardCommand(token: string, runtime: string): string {
  const opt = runtimeById(runtime)
  const lines = [
    'git clone https://github.com/superfive666/agent-hub.git ~/agent-hub',
    `HUB=${hubOrigin()} REG_TOKEN=${token} RUNTIME=${runtime} \\`,
  ]
  // 常驻服务型必须给 webhook 地址，onboard.sh 缺了它会直接 die。
  // 与其让用户复制一条注定失败的命令，不如把这一行摆在他面前、留个显眼的占位。
  if (opt?.form === 'service') lines.push('  RUNTIME_URL=<对方的 webhook 地址> \\')
  // openclaw 的一次性发消息子命令没有文档化的默认值，猜错的表现是每次唤起都失败、
  // 事件一路重试进死信 —— 很难联想到是命令写错了。所以这里也不替他猜。
  if (runtime === 'openclaw') lines.push("  SUBCOMMAND='message send' \\")
  if (runtime === 'generic-shell') lines.push("  COMMAND='sh /path/wake.sh' \\")
  lines.push('  sh ~/agent-hub/agent-hub-skill/scripts/onboard.sh')
  return lines.join('\n')
}

function CopyButton({ text, label }: { text: string; label: string }) {
  /** idle → ok（复制成功）/ fail（三条路都不通，只能手动选） */
  const [state, setState] = useState<'idle' | 'ok' | 'fail'>('idle')
  return (
    <Button
      variant={state === 'ok' ? 'default' : 'pri'}
      className="shrink-0 px-3.5 py-2 text-[11.5px]"
      aria-label={label}
      onClick={async () => {
        const ok = await copyText(text)
        setState(ok ? 'ok' : 'fail')
        if (ok) window.setTimeout(() => setState('idle'), 2000)
      }}
    >
      {state === 'ok' ? <Check size={13} aria-hidden /> : <Copy size={13} aria-hidden />}
      {state === 'ok' ? '已复制' : state === 'fail' ? '复制不了' : '复制'}
    </Button>
  )
}

export interface RegistrationTokenPanelProps {
  agentName?: string
  token: string
  expiresAt?: string
  /** 拼进接入命令的 RUNTIME=。默认 codex 只是个兜底，调用方应当把用户选的那个传进来 */
  runtime?: string
  /** 「再建一个」「回名录」这类后续动作，由用它的页面决定 */
  footer?: ReactNode
}

/**
 * 注册 token 那一屏 —— 建 agent 成功后和「重新签发」都用它，只有一份实现。
 *
 * 这一屏是整个添加流程的重点，不是一句提示：**明文只在这个响应里出现一次**，
 * 库里只有哈希。关掉页面就再也看不到，只能作废重发。所以：token 要大、要能选中、
 * 要有复制按钮（带降级），过期时间要写出来，接入命令要是真命令。
 */
export function RegistrationTokenPanel({
  agentName,
  token,
  expiresAt,
  runtime = 'codex',
  footer,
}: RegistrationTokenPanelProps) {
  const cmd = onboardCommand(token, runtime)
  return (
    <Card data-testid="registration-token">
      <CardHeader>一次性注册 TOKEN{agentName ? ` · @${agentName}` : ''}</CardHeader>
      <CardBody className="gap-4 p-[18px_20px]">
        {/* 警告不是吓唬人，是后端的真实行为：库里只有哈希 */}
        <div
          className="flex items-start gap-2.5 rounded-[14px] px-[15px] py-[13px]"
          style={{ background: 'var(--alert-soft)' }}
        >
          <span style={{ color: 'var(--alert)' }} className="mt-0.5 shrink-0">
            <AlertTriangle size={15} aria-hidden />
          </span>
          <div className="text-[11.5px] font-semibold leading-[1.7]" style={{ color: 'var(--alert)' }}>
            明文只出现这一次。库里只存哈希 —— <b>关掉这个页面就再也看不到了，只能作废重发一张</b>。
            现在就把它交给那台机器。
          </div>
        </div>

        <div>
          <div className="lbl mb-2">token</div>
          <div className="flex items-center gap-2.5">
            {/* 等宽 + 可选中：复制按钮只是省事，手动选中永远是可行的那条路 */}
            <code
              data-testid="token-text"
              className="mono min-w-0 grow select-all break-all rounded-[14px] px-[15px] py-[13px] text-[12.5px] font-semibold leading-[1.6]"
              style={{
                background: 'var(--inset-bg)',
                border: '1px solid var(--inset-bd)',
                boxShadow: 'var(--inset-sh)',
              }}
            >
              {token}
            </code>
            <CopyButton text={token} label="复制注册 token" />
          </div>
          <div className="mt-2.5 flex flex-wrap items-center gap-2">
            <Chip tone="warn" size="sm">
              {expiresAt ? `${dateTimeLabel(expiresAt)} 过期` : '24 小时后过期'}
            </Chip>
            <span className="text-[10.5px] font-medium" style={{ color: 'var(--ink3)' }}>
              短有效期是它安全性的来源之一 · 用过即废，换完长期凭证立即作废
            </span>
          </div>
        </div>

        <div className="sep" />

        <div>
          <div className="lbl mb-2">交给那台机器上的 connector</div>
          <p className="m-0 mb-2.5 text-[11.5px] font-medium leading-[1.75]" style={{ color: 'var(--ink2)' }}>
            token 不是 API 凭证，唯一用途是<b>换长期凭证</b>。在 agent 自己的机器上跑下面这条
            （见 <span className="mono">docs/08-deployment.md §8</span>）：它会换取凭证并 0600 落盘、
            生成 connector 配置、装成 systemd user service、做一次连通性自检。
          </p>
          <div className="flex items-start gap-2.5">
            <pre
              data-testid="onboard-command"
              className="mono min-w-0 grow overflow-x-auto rounded-[14px] px-[15px] py-[13px] text-[11px] leading-[1.8]"
              style={{
                background: 'var(--inset-bg)',
                border: '1px solid var(--inset-bd)',
                boxShadow: 'var(--inset-sh)',
              }}
            >
              {cmd}
            </pre>
            <CopyButton text={cmd} label="复制接入命令" />
          </div>
          <p className="m-0 mt-2.5 text-[10.5px] font-medium leading-[1.7]" style={{ color: 'var(--ink3)' }}>
            命令里的 <span className="mono">RUNTIME={runtime}</span> 就是上面选的那个；
            对方机器上跑的不是它的话，改掉再执行，对照表在
            <span className="mono"> connector/RUNTIMES.md</span>。
            {runtimeById(runtime)?.form === 'service' && (
              <>
                {' '}
                <b style={{ color: 'var(--human)' }}>
                  这一档是常驻服务，必须把 RUNTIME_URL 换成真实的 webhook 地址，
                  否则 onboard.sh 会直接停在那里。
                </b>
              </>
            )}
          </p>
        </div>

        {footer && (
          <>
            <div className="sep" />
            {footer}
          </>
        )}
      </CardBody>
    </Card>
  )
}
