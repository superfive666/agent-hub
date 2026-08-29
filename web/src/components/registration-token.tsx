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
 * 交给 agent 的那**一句话**。
 *
 * token 和 runtime 走 query，所以 agent 拉到的 JOIN.md 里命令是可以直接跑的 ——
 * 没有「把 <你的token> 换成真值」这种需要它自己填的占位符，也就少一个出错的地方。
 *
 * 英文：它和 agent 接下来读到的 JOIN.md 是同一串上下文，中英混排只会让指代变糊。
 * 控制台自己的界面语言不受这里影响。
 *
 * ⚠️ token 在 URL 里，**反向代理的 access log 会记下它**。可接受的前提是它一次性、
 * 24 小时过期、而且本来就明文显示在这一屏上；服务端那边加了 no-store。
 */
function joinPrompt(token: string, runtime: string): string {
  const q = new URLSearchParams({ token, runtime })
  return `Join agent-hub: read ${hubOrigin()}/api/join?${q} and follow it end to end.`
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
  const prompt = joinPrompt(token, runtime)
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
          <div className="lbl mb-2">把这句话交给你的 agent</div>
          <p className="m-0 mb-2.5 text-[11.5px] font-medium leading-[1.75]" style={{ color: 'var(--ink2)' }}>
            <b>不用你去终端里跑任何东西。</b> 复制这一句，粘给那个 agent 就行 ——
            接入这件事本来就该它自己做：换凭证、让自己保持在线、写自己的 Agent Card。
            尤其是 Card 里的「做不了什么」，只有它自己说得清。
          </p>
          <div className="flex items-start gap-2.5">
            <pre
              data-testid="join-prompt"
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
            <CopyButton text={prompt} label="复制给 agent 的接入指令" />
          </div>
          <p className="m-0 mt-2.5 text-[10.5px] font-medium leading-[1.7]" style={{ color: 'var(--ink3)' }}>
            步骤全在仓库根的 <span className="mono">JOIN.md</span> 里，由 hub 自己吐给它；
            token 和 runtime 在 URL 里，所以文档里的命令 agent 拿到就能直接跑，
            没有需要它自己填的占位符。
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
