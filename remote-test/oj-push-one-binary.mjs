// Push one binary to the server via the key SSH channel (gzip+base64,
// sha256 verified). Usage: node remote-test/log-push-one.mjs <api|judge>
import { Client } from 'ssh2'
import fs from 'fs'
import path from 'path'
import { execSync } from 'child_process'
import crypto from 'crypto'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const repo = path.resolve(here, '..')
const keyFile = process.env.OJ_SSH_KEY || 'C:/Users/Asanagi/.ssh/id_ed25519'
const host = process.env.OJ_SSH_HOST || '103.210.238.125'
const which = process.argv[2] || 'api'
const local = path.join(repo, 'dist', which === 'judge' ? 'oj-judge-linux' : 'oj-api-linux')
const remote = which === 'judge' ? '/opt/oj/oj-judge.new' : '/opt/oj/oj-api.new'

const gz = `${local}.gz.tmp`
execSync(`gzip -9 -c "${local}" > "${gz}"`)
const b64 = fs.readFileSync(gz).toString('base64')
fs.unlinkSync(gz)
const localSha = crypto.createHash('sha256').update(fs.readFileSync(local)).digest('hex')
console.log(`${path.basename(local)}: ${(b64.length / 1e6).toFixed(1)} MB, sha=${localSha}`)

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
chmod +x ${remote}.bin
REMOTE_SHA=$(sha256sum ${remote}.bin | cut -d' ' -f1)
if [ "$REMOTE_SHA" != "${localSha}" ]; then echo SHA_MISMATCH; exit 9; fi
mv ${remote}.bin ${remote}
echo "PLACED=${remote}"
`)
}
writeNext()
