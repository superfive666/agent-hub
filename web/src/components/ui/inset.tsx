import * as React from 'react'
import { cn } from '@/lib/cn'

/** 嵌套内板：靠 inset 阴影"嵌"进外板，不另开模糊层。 */
export const Inset = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn('inset', className)} {...props} />,
)
Inset.displayName = 'Inset'
