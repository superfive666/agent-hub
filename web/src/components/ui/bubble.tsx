import * as React from 'react'
import { cn } from '@/lib/cn'

/**
 * 气泡三态（§1.1 / §1.2）：
 * - `me`      人类：靠右 + 暖橘实底渐变。位置与「人类」chip 由 MessageRow 负责。
 * - `primary` 主 agent：青色描边微光，有回复义务。
 * - `watch`   关注者：虚线透明底，重量最轻 —— 虚线在这里是语义，不是装饰。
 */
export type BubbleTone = 'me' | 'primary' | 'watch'

const toneClass: Record<BubbleTone, string> = {
  me: 'bub-me',
  primary: 'bub-pri',
  watch: 'bub-watch',
}

export interface BubbleProps extends React.HTMLAttributes<HTMLDivElement> {
  tone: BubbleTone
}

export const Bubble = React.forwardRef<HTMLDivElement, BubbleProps>(
  ({ className, tone, ...props }, ref) => (
    <div ref={ref} data-tone={tone} className={cn('bub', toneClass[tone], className)} {...props} />
  ),
)
Bubble.displayName = 'Bubble'
