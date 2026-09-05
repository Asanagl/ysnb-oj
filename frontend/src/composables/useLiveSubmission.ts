// 实时判题状态跟踪（纯前端）：订阅 WS 主题 submission:<id>，后端在入队
// (PENDING) / 开判 (JUDGING) / 终态推送事件；断线自动降级为 1.5s 轮询直到
// 终态（复用旧 pollSubmission 语义）。终态 WS 推送只有摘要字段
//（status/time_ms/memory_kb/score），逐 case 与 compile_message 由调用方
// 在 onFinal 里 refresh() 重拉全量快照。
import { onBeforeUnmount, ref } from 'vue'
import { Submissions, connectWS, type Submission } from '../api/client'

// 非终态集合：其余一切状态（含未来新增的判定码）都按终态处理
const ACTIVE_STATUS = ['PENDING', 'COMPILING', 'JUDGING']

export function isFinalStatus(status: string) {
  return !ACTIVE_STATUS.includes(status)
}

interface StatusPush {
  id?: number
  status?: string
  time_ms?: number
  memory_kb?: number
  score?: number
}

export function useLiveSubmission() {
  const snap = ref<Submission | null>(null)
  const status = ref('')
  const isFinal = isFinalStatus
  let ws: ReturnType<typeof connectWS> | null = null
  let pollTimer: number | null = null
  let currentId: number | null = null
  let onFinalCb: (() => void) | null = null
  let disposed = false

  function stopTracking() {
    ws?.close()
    ws = null
    if (pollTimer !== null) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  function settleFinal() {
    stopTracking()
    const cb = onFinalCb
    onFinalCb = null
    cb?.()
  }

  function applySnapshot(s: Submission) {
    snap.value = s
    status.value = s.status
  }

  function applyPush(p: StatusPush) {
    if (typeof p.status !== 'string') return
    status.value = p.status
    if (snap.value) {
      // 终态推送带摘要指标；merge 进现有快照，卡片免重拉即可显示
      snap.value = {
        ...snap.value,
        status: p.status,
        ...(p.time_ms !== undefined ? { time_ms: p.time_ms } : {}),
        ...(p.memory_kb !== undefined ? { memory_kb: p.memory_kb } : {}),
        ...(p.score !== undefined ? { score: p.score } : {}),
      }
    }
    if (isFinal(p.status)) settleFinal()
  }

  async function refresh() {
    if (currentId === null) return
    try {
      applySnapshot(await Submissions.get(currentId))
    } catch {
      /* 瞬时网络错误：保留现有快照，下一轮兜底轮询重试 */
    }
  }

  // 断线降级：WS onclose 且仍未终态 → 1.5s 轮询兜底
  function startPolling() {
    if (pollTimer !== null || currentId === null) return
    pollTimer = window.setInterval(async () => {
      if (currentId === null) return
      try {
        const s = await Submissions.get(currentId)
        applySnapshot(s)
        if (isFinal(s.status)) settleFinal()
      } catch {
        /* 下一轮重试 */
      }
    }, 1500)
  }

  // 写入快照但不建立连接（详情页加载已终态提交用）
  function set(s: Submission) {
    stopTracking()
    currentId = s.id
    applySnapshot(s)
  }

  // 开始实时跟踪：订阅 WS + 首次 GET 对账（补上订阅建立前的状态跃迁）
  function track(id: number, onFinal?: () => void) {
    stopTracking()
    currentId = id
    onFinalCb = onFinal ?? null
    if (status.value && isFinalStatus(status.value)) {
      settleFinal()
      return
    }
    ws = connectWS([`submission:${id}`], (_topic, data) => applyPush(data as StatusPush))
    ws.onclose = () => {
      if (!disposed && !isFinalStatus(status.value)) startPolling()
    }
    void refresh()
  }

  function stop() {
    disposed = true
    stopTracking()
  }
  onBeforeUnmount(stop)

  return { snap, status, set, track, refresh, stop }
}
