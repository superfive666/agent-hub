import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/cn'

/** 全部胶囊化圆角；变体名沿用设计稿里的 pri / gh / ico。 */
const buttonVariants = cva('btn', {
  variants: {
    variant: {
      default: '',
      pri: 'btn-pri',
      gh: 'btn-gh',
    },
    size: {
      default: '',
      ico: 'btn-ico',
      icoSm: 'btn-ico btn-ico-sm',
      block: 'justify-center w-full',
    },
  },
  defaultVariants: { variant: 'default', size: 'default' },
})

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, type = 'button', ...props }, ref) => {
    const Comp = asChild ? Slot : 'button'
    return (
      <Comp
        ref={ref}
        type={asChild ? undefined : type}
        className={cn(buttonVariants({ variant, size }), className)}
        {...props}
      />
    )
  },
)
Button.displayName = 'Button'
export { buttonVariants }
