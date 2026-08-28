import * as React from 'react'
import { cn } from '@/lib/cn'

/**
 * 三态：
 * - `agent` 关注者/普通 agent：玻璃面
 * - `primary` 主 agent：虹彩渐变 + 呼吸（§1.2，页面上唯一会喘气的东西）
 * - `human` 人类：暖橘实底渐变
 */
export type AvatarKind = 'agent' | 'primary' | 'human'

const kindClass: Record<AvatarKind, string> = {
  agent: 'av-a',
  primary: 'av-p',
  human: 'av-h',
}

export interface AvatarProps extends React.HTMLAttributes<HTMLSpanElement> {
  kind?: AvatarKind
  size?: 'default' | 'sm' | 'xs'
  /** 展示的字母/汉字缩写 */
  initials: string
  /** undefined = 不显示在线点（比如人类自己） */
  online?: boolean
  label?: string
}

export const Avatar = React.forwardRef<HTMLSpanElement, AvatarProps>(
  ({ className, kind = 'agent', size = 'default', initials, online, label, ...props }, ref) => (
    <span
      ref={ref}
      className={cn(
        'av',
        kindClass[kind],
        size === 'sm' && 'av-sm',
        size === 'xs' && 'av-xs',
        className,
      )}
      aria-label={label}
      data-kind={kind}
      {...props}
    >
      {initials}
      {online !== undefined && (
        <span
          className={cn('st', online ? 'st-on' : 'st-off')}
          data-testid="avatar-status"
          aria-label={online ? '在线' : '离线'}
        />
      )}
    </span>
  ),
)
Avatar.displayName = 'Avatar'
