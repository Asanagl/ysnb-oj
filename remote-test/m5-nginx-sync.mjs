// Push the hardened nginx.conf to /etc/nginx/sites-available/oj and reload.
// Backs up first; nginx -t gates the swap; health check after reload.
import { Client } from 'ssh2'
import fs from 'fs'
import path from 'path'
import crypto from 'crypto'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const env = Object.fromEntries(
  fs.readFileSync(path.join(here, '.sshenv'), 'utf8')
    .split(/\r?\n/).filter((l) => l.includes('='))
    .map((l) => [l.slice(0, l.indexOf('=')).trim(), l.slice(l.indexOf('=') + 1).trim()]),
)

const local = path.join(here, '..', 'deploy', 'nginx.conf')
const b64 = fs.readFileSync(local).toString('base64')
const sha = crypto.createHash('sha256').update(fs.readFileSync(local)).digest('hex')

const conn = new Client()
await new Promise((resolve, reject) => {
  conn.on('ready', resolve)
  conn.on('error', reject)
  conn.connect({
    host: env.HOST, port: Number(env.PORT || 22),
    username: env.USER || 'root', password: env.PASSWORD, readyTimeout: 20000,
  })
})

const stream = await new Promise((resolve, reject) => {
  conn.exec('bash -s 2>&1; echo STEP_EXIT=$?', (err, s) => (err ? reject(err) : resolve(s)))
})
stream.on('data', (d) => process.stdout.write(d))
stream.on('close', (c) => { conn.end(); process.exit(c === 0 ? 0 : 1) })

stream.write(`set -e
cp /etc/nginx/sites-available/oj /etc/nginx/sites-available/oj.bak-sec
base64 -d > /etc/nginx/sites-available/oj.new <<'OJB64EOF'
`)
const CHUNK = 32 * 1024
let i = 0
await new Promise((resolve) => {
  function writeNext() {
    while (i < b64.length) {
      const chunk = b64.slice(i, i + CHUNK)
      i += CHUNK
      if (!stream.write(chunk + '\n')) {
        stream.once('drain', writeNext)
        return
      }
    }
    resolve()
  }
  writeNext()
})
await new Promise((resolve) => stream.write(`OJB64EOF
REMOTE_SHA=$(sha256sum /etc/nginx/sites-available/oj.new | cut -d' ' -f1)
if [ "$REMOTE_SHA" != "${sha}" ]; then echo SHA_MISMATCH; exit 9; fi
nginx -t && mv /etc/nginx/sites-available/oj.new /etc/nginx/sites-available/oj
systemctl reload nginx
sleep 1
curl -sI http://127.0.0.1/ | grep -iE "HTTP/|x-frame|x-content|referrer|security" | head -6
`, resolve))
stream.end()