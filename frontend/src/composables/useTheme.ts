import { ref, watchEffect } from 'vue'

export type Theme = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'oj_theme'

// Module-level singleton: one theme state shared by every consumer.
const theme = ref<Theme>((localStorage.getItem(STORAGE_KEY) as Theme | null) ?? 'system')
const media = window.matchMedia('(prefers-color-scheme: dark)')
const systemDark = ref(media.matches)
media.addEventListener('change', (e) => {
  systemDark.value = e.matches
})

watchEffect(() => {
  const dark = theme.value === 'dark' || (theme.value === 'system' && systemDark.value)
  document.documentElement.classList.toggle('dark', dark)
})

export function useTheme() {
  function setTheme(next: Theme) {
    theme.value = next
    localStorage.setItem(STORAGE_KEY, next)
  }
  return { theme, setTheme }
}
