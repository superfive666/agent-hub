import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/cn'

const chipVariants = cva('chip', {
  variants: {
    tone: {
      default: '',
      agent: 'chip-a',
      human: 'chip-h',
      warn: 'chip-w',
      alert: 'chip-al',
      solid: 'chip-solid',
    },
    size: { default: '', sm: 'chip-sm' },
  },
  defaultVariants: { tone: 'default', size: 'default' },
})

export interface ChipProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof chipVariants> {}

export const Chip = React.forwardRef<HTMLSpanElement, ChipProps>(
  ({ className, tone, size, ...props }, ref) => (
    <span ref={ref} className={cn(chipVariants({ tone, size }), className)} {...props} />
  ),
)
Chip.displayName = 'Chip'
export { chipVariants }
