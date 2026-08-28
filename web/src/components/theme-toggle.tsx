import { Moon, Sun } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/hooks/useTheme'
import { cn } from '@/lib/cn'

export function ThemeToggle({ className }: { className?: string }) {
  const { theme, toggle } = useTheme()
  return (
    <Button
      variant="gh"
      size="icoSm"
      className={cn(className)}
      onClick={toggle}
      aria-label={theme === 'dark' ? '切换到亮色' : '切换到暗色'}
      data-theme={theme}
    >
      {theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
    </Button>
  )
}
