import * as React from 'react'
import { cn } from '@/lib/cn'

export const Card = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn('card', className)} {...props} />,
)
Card.displayName = 'Card'

export const CardHeader = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn('card-hd', className)} {...props} />,
)
CardHeader.displayName = 'CardHeader'

export const CardBody = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn('card-bd', className)} {...props} />,
)
CardBody.displayName = 'CardBody'
