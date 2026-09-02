// Responsive breakpoint state, shared app-wide. Why matchMedia instead of
// listening to resize: matchMedia fires exactly on breakpoint crossing, so
// components re-render only when the layout tier actually changes.
// Tiers (aligned with the global CSS in style.css):
//   phone   <768        drawer nav, single column
//   tablet  768–1279    two-column where applicable
//   desktop 1280–1919   current layout
//   wide    ≥1920       problem page three-column, enhanced tables
//   ultrawide ≥2560     wider fluid container, larger base type
import { ref, type Ref } from 'vue'

const queries: Record<string, string> = {
  isPhone: '(max-width: 767.98px)',
  isTablet: '(min-width: 768px) and (max-width: 1279.98px)',
  isWide: '(min-width: 1920px)',
  isUltrawide: '(min-width: 2560px)',
  isTouch: '(hover: none) and (pointer: coarse)',
}

const state = {} as Record<string, Ref<boolean>>
for (const [key, query] of Object.entries(queries)) {
  const value = ref(window.matchMedia(query).matches)
  const mql = window.matchMedia(query)
  // why addEventListener over the legacy addListener: baseline is Chrome 105+.
  mql.addEventListener('change', (e) => (value.value = e.matches))
  state[key] = value
}

export function useResponsive() {
  return {
    isPhone: state.isPhone,
    isTablet: state.isTablet,
    isWide: state.isWide,
    isUltrawide: state.isUltrawide,
    isTouch: state.isTouch,
  }
}
