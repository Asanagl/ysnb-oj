// port-scan.mjs — full TCP connect scan, concurrency-limited.
// Usage: node port-scan.mjs <host> [from=1] [to=65535] [concurrency=800] [timeoutMs=1000]
// Only for scanning infrastructure you own.
import net from 'net'

const host = process.argv[2]
const from = Number(process.argv[3] || 1)
const to = Number(process.argv[4] || 65535)
const conc = Number(process.argv[5] || 800)
const timeoutMs = Number(process.argv[6] || 1000)

if (!host) {
  console.error('usage: node port-scan.mjs <host> [from] [to] [concurrency] [timeoutMs]')
  process.exit(2)
}

let next = from
const open = []
const t0 = Date.now()

function tryPort(port) {
  return new Promise((resolve) => {
    const s = new net.Socket()
    let settled = false
    const fin = (isOpen) => {
      if (settled) return
      settled = true
      s.destroy()
      if (isOpen) open.push(port)
      resolve()
    }
    s.setTimeout(timeoutMs, () => fin(false))
    s.once('connect', () => fin(true))
    s.once('error', () => fin(false))
    s.connect(port, host)
  })
}

async function worker() {
  while (next <= to) {
    const port = next++
    await tryPort(port)
    if (port % 8000 === 0) {
      const pct = (((port - from) / (to - from)) * 100).toFixed(1)
      console.error(`[scan] ${port}/${to} (${pct}%) open-so-far=${open.length} ${((Date.now() - t0) / 1000).toFixed(0)}s`)
    }
  }
}

await Promise.all(Array.from({ length: conc }, () => worker()))
open.sort((a, b) => a - b)
console.log('=== SCAN COMPLETE ===')
console.log(`range=${from}-${to} elapsed=${((Date.now() - t0) / 1000).toFixed(0)}s`)
console.log(`OPEN (${open.length}): ${open.join(', ') || 'none'}`)