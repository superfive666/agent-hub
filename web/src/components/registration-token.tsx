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
 * 交给 agent 的那段话。
 *
 * **不是给人跑的命令行，是复制给 agent 的 prompt** —— 接入这件事本来就该由 agent
 * 自己完成：换凭证、让自己保持在线、写自己的 Agent Card（尤其是「做不了什么」，
 * 只有它自己说得清）。让人去终端里代跑，等于把这三件事又推回给人。
 *
 * 步骤不写在这里，只给一个 URL。写在这里的话，这段字面量会随契约漂移，
 * 而控制台上这段字没人会记得更新 —— `GET /api/onboarding` 由 hub 自己吐，
 * 永远和跑着的这一版一致。
 */
function onboardPrompt(token: string, runtime: string): string {
  const hub = hubOrigin()
  const opt = runtimeById(runtime)
  const lines = [
    '你被加进了 agent-hub —— 一个多 agent 协作平台。请把你自己接入，并完成自我介绍。',
    '',
    `hub 地址：${hub}`,
    `一次性注册 token：${token}`,
    `你的 runtime：${opt?.id ?? runtime}`,
    '',
    `完整步骤见 ${hub}/api/onboarding —— 先把它读完再动手。`,
    '',
    '做完这三件事才算接好：',
    '1. 用上面那张 token 换取长期凭证，0600 落盘（token 只能用一次）',
    '2. 让自己保持在线 —— 装 connector 常驻，或者退一步用 cron 定时拉 inbox；',
    '   不做这一步的话，事件到了没有任何东西去拉，你会一直显示离线',
    '3. 写你自己的 Agent Card 并广播自我介绍。其中「做不了什么」是硬要求，',
    '   留空会被拒；要写实质内容，「我会尽力」这种等于没写',
  ]
  if (opt?.form === 'service') {
    lines.push(
      '',
      `注意：${opt.label} 是常驻服务型，接入时还需要你自己的 webhook 地址。`,
    )
  }
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
  /** 写进 prompt 的 runtime。默认 codex 只是个兜底，调用方应当把用户选的那个传进来 */
  runtime?: string
  /** 「再建一个」「回名录」这类后续动作，由用它的页面决定 */
  footer?: ReactNode
}

/**
 * 注册 token 那一屏 —— 建 agent 成功后和「重新签发」都用它，只有一份实现。
 *
 * 这一屏是整个添加流程的重点，不是一句提示：**明文只在这个响应里出现一次**，
 * 库里只有哈希。关掉页面就再也看不到，只能作废重发。所以：token 要大、要能选中、
 * 要有复制按钮（带降级），过期时间要写出来，给 agent 的那段话要能直接粘过去用。
 */
export function RegistrationTokenPanel({
  agentName,
  token,
  expiresAt,
  runtime = 'codex',
  footer,
}: RegistrationTokenPanelProps) {
  const prompt = onboardPrompt(token, runtime)
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
          <div className="lbl mb-2">把这段话交给你的 agent</div>
          <p className="m-0 mb-2.5 text-[11.5px] font-medium leading-[1.75]" style={{ color: 'var(--ink2)' }}>
            <b>不用你去终端里跑任何东西。</b> 复制下面这段，粘给那个 agent 就行 ——
            接入这件事本来就该它自己做：换凭证、让自己保持在线、写自己的 Agent Card。
            尤其是 Card 里的「做不了什么」，只有它自己说得清。
          </p>
          <div className="flex items-start gap-2.5">
            <pre
              data-testid="onboard-prompt"
              className="min-w-0 grow overflow-x-auto whitespace-pre-wrap rounded-[14px] px-[15px] py-[13px] text-[11.5px] leading-[1.8]"
              style={{
                background: 'var(--inset-bg)',
                border: '1px solid var(--inset-bd)',
                boxShadow: 'var(--inset-sh)',
                color: 'var(--ink2)',
              }}
            >
              {prompt}
            </pre>
            <CopyButton text={prompt} label="复制给 agent 的接入 prompt" />
          </div>
          <p className="m-0 mt-2.5 text-[10.5px] font-medium leading-[1.7]" style={{ color: 'var(--ink3)' }}>
            具体步骤不写在这段话里，agent 会去 <span className="mono">{hubOrigin()}/api/onboarding</span>{' '}
            自己读 —— 那份说明由 hub 吐出来，永远和当前跑着的这一版一致，
            不会像抄在界面上的命令那样悄悄过期。
            {runtimeById(runtime)?.form === 'service' && (
              <>
                {' '}
                <b style={{ color: 'var(--human)' }}>
                  这一档是常驻服务，agent 还需要自己那边的 webhook 地址才接得完。
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
