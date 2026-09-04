// Push the hardened nginx.conf to /etc/nginx/sites-available/oj and reload
// (key auth). Backs up first; nginx -t gates the swap; header check after.
// Replaces the password channel version (server SSH is key-only).
import { Client } from 'ssh2'
import fs from 'fs'
import path from 'path'
import crypto from 'crypto'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const keyFile = process.env.OJ_SSH_KEY || 'C:/Users/Asanagi/.ssh/id_ed25519'
const host = process.env.OJ_SSH_HOST || '103.210.238.125'

const local = path.join(here, '..', 'deploy', 'nginx.conf')
const b64 = fs.readFileSync(local).toString('base64')
const sha = crypto.createHash('sha256').update(fs.readFileSync(local)).digest('hex')

const conn = new Client()
await new Promise((resolve, reject) => {
  conn.on('ready', resolve)
  conn.on('error', reject)
  conn.connect({ host, port: 22, username: 'root', privateKey: fs.readFileSync(keyFile), readyTimeout: 20000 })
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
