import { useCallback, useEffect, useState } from 'react'

export type Theme = 'light' | 'dark'
export type ThemePref = Theme | 'system'

export const THEME_STORAGE_KEY = 'agent-hub-theme'

function readStored(): ThemePref {
  try {
    const v = localStorage.getItem(THEME_STORAGE_KEY)
    return v === 'light' || v === 'dark' ? v : 'system'
  } catch {
    return 'system'
  }
}

function systemTheme(): Theme {
  return typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

export function resolveTheme(pref: ThemePref): Theme {
  return pref === 'system' ? systemTheme() : pref
}

/**
 * `.dark` 挂在 <html> 上；偏好存 localStorage，默认跟随 prefers-color-scheme。
 */
export function useTheme() {
  const [pref, setPref] = useState<ThemePref>(readStored)
  const [theme, setTheme] = useState<Theme>(() => resolveTheme(readStored()))

  useEffect(() => {
    const next = resolveTheme(pref)
    setTheme(next)
    document.documentElement.classList.toggle('dark', next === 'dark')
  }, [pref])

  // 跟随系统：只有在 pref === 'system' 时才响应系统变化
  useEffect(() => {
    if (pref !== 'system' || typeof matchMedia !== 'function') return
    const mq = matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => {
      const next: Theme = mq.matches ? 'dark' : 'light'
      setTheme(next)
      document.documentElement.classList.toggle('dark', next === 'dark')
    }
    mq.addEventListener?.('change', onChange)
    return () => mq.removeEventListener?.('change', onChange)
  }, [pref])

  const setPreference = useCallback((next: ThemePref) => {
    setPref(next)
    try {
      if (next === 'system') localStorage.removeItem(THEME_STORAGE_KEY)
      else localStorage.setItem(THEME_STORAGE_KEY, next)
    } catch {
      /* 隐私模式下写不进去也不该崩 */
    }
  }, [])

  const toggle = useCallback(() => {
    setPreference(resolveTheme(pref) === 'dark' ? 'light' : 'dark')
  }, [pref, setPreference])

  return { theme, preference: pref, setPreference, toggle }
}
