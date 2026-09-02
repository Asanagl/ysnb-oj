<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Contests, connectWS, type StandingRow } from '../api/client'
import ScoreBoard from '../components/ScoreBoard.vue'

// Fullscreen projection scoreboard for the contest venue big screen:
// giant countdown, dark CF-style board with auto-carousel scrolling, and a
// scrolling announcement ticker. ESC or the corner button exits.
const route = useRoute()
const router = useRouter()
const cid = route.params.id as string

const title = ref('')
const rows = ref<StandingRow[]>([])
const problems = ref<{ label: string }[]>([])
const frozen = ref(false)
const now = ref(Date.now())
const wrap = ref<HTMLDivElement>()
const errorText = ref('')

const remaining = computed(() => {
  if (!dataEnd.value) return ''
  const ms = dataEnd.value - now.value
  if (ms <= 0) return '已结束'
  const h = Math.floor(ms / 3600000)
  const m = Math.floor((ms % 3600000) / 60000)
  const s = Math.floor((ms % 60000) / 1000)
  return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
})
const urgencyClass = computed(() => {
  if (!dataEnd.value || dataEnd.value - now.value <= 0) return 'over'
  if (dataEnd.value - now.value < 5 * 60 * 1000) return 'critical'
  if (dataEnd.value - now.value < 30 * 60 * 1000) return 'warn'
  return ''
})
const dataEnd = ref<number | null>(null)
const notices = ref<Awaited<ReturnType<typeof Contests.notices>>>([])
const noticesText = computed(() =>
  notices.value.map((n) => n.content).join('　◆　'),
)
const frozenShown = computed(() => frozen.value)

let ws: ReturnType<typeof connectWS> | null = null
let tick: ReturnType<typeof setInterval> | null = null
let poll: ReturnType<typeof setInterval> | null = null
let carousel: ReturnType<typeof setInterval> | null = null

async function loadAll() {
  try {
    const d = await Contests.get(cid)
    title.value = d.contest.title
    document.title = `${d.contest.title} · 大屏`
    dataEnd.value = new Date(d.contest.end_time).getTime()
    frozen.value = !!d.contest.manual_frozen
    problems.value = d.problems
    notices.value = await Contests.notices(cid)
    rows.value = (await Contests.standings(cid)).rows
  } catch (e) {
    errorText.value = String(e)
  }
}

// carousel: scrolls the board 2px per tick; pauses at both ends so the top
// of the board stays readable for a while on short guest lists.
function startCarousel() {
  carousel = setInterval(() => {
    const el = wrap.value
    if (!el || el.scrollHeight <= el.clientHeight + 8) return
    if (pauseTicks > 0) {
      pauseTicks--
      return
    }
    el.scrollTop += 2
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 2) {
      el.scrollTop = 0
      pauseTicks = 120
    }
  }, 50)
}
let pauseTicks = 0

function exitBoard() {
  cleanup()
  router.push(`/contests/${cid}`)
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') exitBoard()
}
function cleanup() {
  ws?.close()
  if (tick) clearInterval(tick)
  if (poll) clearInterval(poll)
  if (carousel) clearInterval(carousel)
}

onMounted(async () => {
  document.title = '大屏 · 验收'
  window.addEventListener('keydown', onKey)
  await loadAll()
  ws = connectWS([`contest:${cid}`], () => loadStandingsOnly())
  poll = setInterval(() => loadStandingsOnly(), 15000)
  // full state refresh (time extensions mid-contest) every minute
  setInterval(() => loadAll(), 60000)
  tick = setInterval(() => (now.value = Date.now()), 1000)
  startCarousel()
})
onBeforeUnmount(cleanup)

async function loadStandingsOnly() {
  rows.value = (await Contests.standings(cid)).rows
}
</script>

<template>
  <div class="proj">
    <div class="proj-top">
      <div class="proj-title">{{ title || errorText || '加载中…' }}</div>
      <div class="proj-count" :class="urgencyClass">{{ remaining }}</div>
      <div class="proj-badges">
        <span v-if="frozenShown" class="proj-badge frozen">已封榜</span>
        <button class="proj-exit" @click="exitBoard">退出</button>
      </div>
    </div>

    <div ref="wrap" class="proj-board">
      <ScoreBoard :rows="rows" :problems="problems" large />
    </div>

    <div v-if="noticesText" class="proj-ticker">
      <div class="ticker-track">
        <span class="ticker-item" v-for="i in 2" :key="i">{{ noticesText }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.proj {
  position: fixed;
  inset: 0;
  background: #0d1117;
  color: #e6edf3;
  display: flex;
  flex-direction: column;
  z-index: 2000;
}
.proj-top {
  display: flex;
  align-items: center;
  gap: 28px;
  padding: 20px 36px;
  border-bottom: 1px solid #21262d;
  background: #010409;
}
.proj-title {
  font-size: 40px;
  font-weight: 800;
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.proj-count {
  font-family: 'Consolas', 'Courier New', monospace;
  font-size: 72px;
  font-weight: 800;
  letter-spacing: 4px;
  color: #7ee787;
  font-variant-numeric: tabular-nums;
}
.proj-count.warn {
  color: #d29922;
}
.proj-count.critical {
  color: #f85149;
}
.proj-count.over {
  color: #8b949e;
}
.proj-badges {
  display: flex;
  align-items: center;
  gap: 16px;
}
.proj-badge.frozen {
  background: rgba(210, 153, 34, 0.25);
  color: #d29922;
  font-size: 24px;
  font-weight: 700;
  padding: 8px 20px;
  border-radius: 8px;
}
.proj-exit {
  background: #21262d;
  color: #8b949e;
  border: none;
  border-radius: 8px;
  padding: 10px 18px;
  font-size: 18px;
  cursor: pointer;
}
.proj-board {
  flex: 1;
  overflow: auto;
  padding: 16px 24px;
}
.proj-ticker {
  height: 64px;
  background: #010409;
  border-top: 1px solid #21262d;
  overflow: hidden;
  display: flex;
  align-items: center;
}
.ticker-track {
  display: flex;
  white-space: nowrap;
  animation: ticker-scroll 30s linear infinite;
}
.ticker-item {
  font-size: 26px;
  color: #d29922;
  padding-right: 120px;
}
@keyframes ticker-scroll {
  from {
    transform: translateX(100vw);
  }
  to {
    transform: translateX(-100%);
  }
}
</style>
