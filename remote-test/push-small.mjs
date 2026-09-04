// Push a small local file to the server (gzip+base64+sha256).
// Usage: node remote-test/push-small.mjs <local> <remote>
import { Client } from 'ssh2'
import fs from 'fs'
import path from 'path'
import { execSync } from 'child_process'
import crypto from 'crypto'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const keyFile = process.env.OJ_SSH_KEY || 'C:/Users/Asanagi/.ssh/id_ed25519'
const host = process.env.OJ_SSH_HOST || '103.210.238.125'
const [, , local, remote] = process.argv
if (!local || !remote) { console.error('usage: node push-small.mjs <local> <remote>'); process.exit(2) }

const gz = `${path.resolve(local)}.gz.tmp`
execSync(`gzip -9 -c "${path.resolve(local)}" > "${gz}"`)
const b64 = fs.readFileSync(gz).toString('base64')
fs.unlinkSync(gz)
const localSha = crypto.createHash('sha256').update(fs.readFileSync(local)).digest('hex')
console.log(`${local}: ${(b64.length / 1e6).toFixed(2)} MB, sha=${localSha.slice(0, 16)}…`)

const conn = new Client()
await new Promise((resolve, reject) => {
  conn.on('ready', resolve)
  conn.on('error', reject)
  conn.connect({ host, port: 22, username: 'root', privateKey: fs.readFileSync(keyFile), readyTimeout: 20000 })
})
const stream = await new Promise((r, j) => conn.exec('bash -s 2>&1; echo STEP_EXIT=$?', (e, s) => (e ? j(e) : r(s))))
stream.on('data', (d) => process.stdout.write(d))
stream.on('close', (c) => { conn.end(); process.exit(c === 0 ? 0 : 1) })
stream.write(`set -e
base64 -d > ${remote}.gz <<'OJB64EOF'
`)
const CHUNK = 32 * 1024
let i = 0
function writeNext() {
  while (i < b64.length) {
    const chunk = b64.slice(i, i + CHUNK)
    i += CHUNK
    if (!stream.write(chunk + '\n')) { stream.once('drain', writeNext); return }
  }
  stream.end(`OJB64EOF
gunzip -c ${remote}.gz > ${remote}.bin && rm ${remote}.gz
REMOTE_SHA=$(sha256sum ${remote}.bin | cut -d' ' -f1)
if [ "$REMOTE_SHA" != "${localSha}" ]; then echo SHA_MISMATCH; exit 9; fi
mv ${remote}.bin ${remote}
echo "PLACED=${remote}"
`)
}
writeNext()
