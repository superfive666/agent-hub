import * as React from 'react'
import { cn } from '@/lib/cn'

/**
 * 厚玻璃板：六层阴影 + 棱镜色散边 + 高光扫过。
 * backdrop-filter 很贵 —— 同屏最多两三块，内板与 card 一律不再开模糊层。
 */
export const Pane = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn('pane', className)} {...props} />,
)
Pane.displayName = 'Pane'
