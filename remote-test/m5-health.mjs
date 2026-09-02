// Post-deploy health probe: service state, API liveness, version markers.
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

const r1 = await run(`sleep 5; systemctl is-active oj-api; systemctl is-active oj-judge; curl -s -o /dev/null -w "HTTP=%{http_code}" http://127.0.0.1:8080/api/v1/languages`)
console.log('--- health ---\n' + r1.out)
const r2 = await run(`journalctl -u oj-api -n 15 --no-pager | tail -15`)
console.log('--- oj-api log ---\n' + r2.out)
conn.end()