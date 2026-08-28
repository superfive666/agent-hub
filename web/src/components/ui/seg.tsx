import * as React from 'react'
import * as ToggleGroup from '@radix-ui/react-toggle-group'
import { cn } from '@/lib/cn'

export interface SegOption {
  value: string
  label: React.ReactNode
}

export interface SegProps {
  options: SegOption[]
  value: string
  onValueChange: (value: string) => void
  className?: string
  'aria-label'?: string
}

/** 分段器：内板上的胶囊，选中项浮起来变成一小块厚玻璃。 */
export function Seg({ options, value, onValueChange, className, ...rest }: SegProps) {
  return (
    <ToggleGroup.Root
      type="single"
      value={value}
      onValueChange={(v) => v && onValueChange(v)}
      className={cn('seg', className)}
      aria-label={rest['aria-label']}
    >
      {options.map((o) => (
        <ToggleGroup.Item
          key={o.value}
          value={o.value}
          className="seg-item"
          data-state={o.value === value ? 'on' : 'off'}
        >
          {o.label}
        </ToggleGroup.Item>
      ))}
    </ToggleGroup.Root>
  )
}
