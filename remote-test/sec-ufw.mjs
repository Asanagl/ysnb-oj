// ufw firewall hardening: default deny inbound, allow SSH + HTTP + HTTPS.
// Runs AFTER SSH allow rule is confirmed present (never lock out SSH).
// Idempotent: safe to re-run.
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

const sshPort = Number(env.PORT || 22)
const conn = new Client()
await new Promise((resolve, reject) => {
  conn.on('ready', resolve)
  conn.on('error', reject)
  conn.connect({
    host: env.HOST, port: sshPort,
    username: env.USER || 'root', password: env.PASSWORD, readyTimeout: 20000,
  })
})
console.log('connected')

const stream = await new Promise((resolve, reject) => {
  conn.exec('bash -s 2>&1; echo STEP_EXIT=$?', (err, s) => (err ? reject(err) : resolve(s)))
})
let out = ''
stream.on('data', (d) => process.stdout.write(d))
stream.on('close', (c) => { conn.end(); process.exit(c === 0 ? 0 : 1) })

const runScript = `set -e
apt-get install -y ufw >/dev/null 2>&1 || true
ufw default deny incoming
ufw default allow outgoing
# SSH first — never lock ourselves out (idempotent)
ufw allow ${sshPort}/tcp comment 'SSH'
ufw allow 80/tcp comment 'HTTP'
ufw allow 443/tcp comment 'HTTPS'
# docker bypasses ufw via iptables; pin published services explicitly.
# (api/judge no longer publish ports at all after the compose fix.)
ufw --force enable
ufw status verbose | head -12
`
stream.write(runScript, () => stream.end())