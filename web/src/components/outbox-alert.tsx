import { AlertTriangle } from 'lucide-react'
import { Chip } from '@/components/ui/chip'

export interface OutboxAlertProps {
  /** /api/admin/health 的 outboxLagSeconds */
  lagSeconds: number
  workerAlive: boolean
  pending?: number
  thresholdSeconds?: number
}

/**
 * §1.4：worker 挂掉是**完全静默**的失败 —— 帖子照发、inbox 照拉，只是没有新东西。
 * outbox_lag 是唯一能发现它的地方。
 *
 * 这个组件**没有折叠、没有关闭按钮、没有窄屏降级**，这是刻意的。
 * 要动它，先改 docs/07-design-language.md 并说明理由。
 */
export function OutboxAlert({
  lagSeconds,
  workerAlive,
  pending,
  thresholdSeconds = 60,
}: OutboxAlertProps) {
  if (workerAlive && lagSeconds < thresholdSeconds) return null
  return (
    <div
      role="alert"
      data-testid="outbox-alert"
      className="flex items-center gap-3 rounded-pill px-4 py-3"
      style={{
        background: 'var(--alert-soft)',
        border: '1px solid color-mix(in srgb, var(--alert) 40%, transparent)',
        color: 'var(--alert)',
      }}
    >
      <AlertTriangle size={16} aria-hidden />
      <span className="text-[12px] font-bold">
        outbox 投递滞后{' '}
        <span className="mono">
          {Number.isFinite(lagSeconds) ? `${lagSeconds.toFixed(0)}s` : '读不到'}
        </span>
        {workerAlive ? '' : ' · worker 无心跳'}
      </span>
      {pending !== undefined && (
        <Chip tone="alert" size="sm" className="ml-auto">
          积压 <span className="mono">{pending}</span>
        </Chip>
      )}
    </div>
  )
}
