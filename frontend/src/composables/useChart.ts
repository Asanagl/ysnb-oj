// Shared ECharts lifecycle for dashboard-style pages (profile heatmap/trend/
// pie/tags, external-practice heatmap). Centralizes echarts.use registration,
// the charts-array reuse pattern, window resize, disposal on unmount, and
// re-draw on light/dark theme switch (canvas colors can't follow CSS vars
// by themselves — pass cssVar()-derived colors in options and redraw).
import * as echarts from 'echarts/core'
import { BarChart, HeatmapChart, LineChart, PieChart } from 'echarts/charts'
import {
  CalendarComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  VisualMapComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useTheme } from './useTheme'

echarts.use([
  BarChart,
  HeatmapChart,
  LineChart,
  PieChart,
  CalendarComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  VisualMapComponent,
  CanvasRenderer,
])

export function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

export function useChart() {
  const charts: echarts.ECharts[] = []
  // why surfaced in-page: chart init errors would otherwise be a silent blank
  // section; showing them makes remote debugging possible without console.
  const chartErrors = ref<string[]>([])

  function mount(name: string, el: HTMLDivElement | undefined, option: echarts.EChartsCoreOption) {
    if (!el) {
      chartErrors.value.push(`${name}: 容器不存在`)
      return
    }
    try {
      const chart = echarts.init(el)
      chart.setOption(option)
      charts.push(chart)
    } catch (e) {
      chartErrors.value.push(`${name}: ${String(e)}`)
    }
  }

  function disposeAll() {
    for (const c of charts) c.dispose()
    charts.length = 0
  }

  function resizeAll() {
    for (const c of charts) c.resize()
  }

  const resizeHandler = () => resizeAll()
  onMounted(() => window.addEventListener('resize', resizeHandler))
  onBeforeUnmount(() => {
    window.removeEventListener('resize', resizeHandler)
    disposeAll()
  })

  // Theme switch: canvas keeps stale colors unless the caller redraws.
  const { isDark } = useTheme()
  function watchTheme(redraw: () => void) {
    watch(isDark, () => redraw())
  }

  return { chartErrors, mount, disposeAll, resizeAll, watchTheme }
}
