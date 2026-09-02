// Server-side observation during the storm: memory, load, oj services.
import { Client } from 'ssh2'
import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const env = Object.fromEntries(
  fs.readFileSync(path.join(here, '.sshenv'), 'utf8')
    .split(/\r?\n/).filter((l) => l.includes('='))
    .map((l) => [l.slice(0, l.indexOf('=')).trim(), l.slice(l.indexOf('=') + 1).trim()]),
)

const durationSec = Number(process.argv[2] ?? 120)
const intervalSec = Number(process.argv[3] ?? 10)

const conn = new Client()
await new Promise((resolve, reject) => {
  conn.on('ready', resolve)
  conn.on('error', reject)
  conn.connect({
    host: env.HOST, port: Number(env.PORT || 22),
    username: env.USER || 'root', password: env.PASSWORD,
    readyTimeout: 20000,
  })
})
console.log(`observing ${durationSec}s every ${intervalSec}s`)

const end = Date.now() + durationSec * 1000
for (let i = 0; Date.now() < end; i++) {
  await new Promise((resolve) => {
    conn.exec('free -m | awk \'NR==2{print "memUsedMB="$3" memAvailMB="$7}\'; uptime | grep -o "load average.*"; ps aux --sort=-rss | awk \'NR==2{print "topRSS="$6"KB "$11}\'', (err, stream) => {
      if (err) return resolve()
      let out = ''
      stream.on('data', (d) => { out += d.toString() })
      stream.on('close', () => { console.log(`[t+${i * intervalSec}s] ${out.trim().split('\n').join(' | ')}`); resolve() })
    })
  })
  await new Promise((r) => setTimeout(r, intervalSec * 1000))
}
conn.end()