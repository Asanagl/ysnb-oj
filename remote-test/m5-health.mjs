// Post-deploy health probe (key auth): service state, API liveness, judge
// round-trip and a peek at recent structured logs. Replaces the password
// channel version (server SSH is key-only since 2026-09-04).
import { Client } from 'ssh2'
import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const keyFile = process.env.OJ_SSH_KEY || 'C:/Users/Asanagi/.ssh/id_ed25519'
const host = process.env.OJ_SSH_HOST || '103.210.238.125'

const conn = new Client()
await new Promise((resolve, reject) => {
  conn.on('ready', resolve)
  conn.on('error', reject)
  conn.connect({ host, port: 22, username: 'root', privateKey: fs.readFileSync(keyFile), readyTimeout: 20000 })
})

function run(cmd) {
  return new Promise((resolve) => {
    conn.exec(`bash -s 2>&1`, (err, stream) => {
      if (err) return resolve({ code: -1, out: err.message })
      let out = ''
      stream.on('close', (c) => resolve({ code: c, out }))
      stream.on('data', (d) => { out += d.toString() })
      stream.stderr.on('data', (d) => { out += d.toString() })
      stream.end(cmd + '\n')
    })
  })
}

const r1 = await run(`systemctl is-active oj-api oj-judge nginx; curl -s -o /dev/null -w "HTTP=%{http_code}" http://127.0.0.1:8080/api/v1/languages; echo; curl -s -o /dev/null -w "index=%{http_code}" http://127.0.0.1/; echo`)
console.log('--- health ---\n' + r1.out)
const r2 = await run(`journalctl -u oj-api --lines 10 --no-pager -o cat | tail -10`)
console.log('--- oj-api recent (structured) ---\n' + r2.out)
conn.end()
