// Repair: the previous heredoc-based push corrupted /opt/oj/oj-api (the
// base64 payload passed through the same stdin the heredoc consumed, so the
// decode truncated). Re-push with the payload fed AFTER the heredoc marker
// inside a single stdin stream, then verify sha256 end to end before swap.
import { Client } from 'ssh2'
import fs from 'fs'
import path from 'path'
import { execSync } from 'child_process'
import crypto from 'crypto'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const env = Object.fromEntries(
  fs.readFileSync(path.join(here, '.sshenv'), 'utf8')
    .split(/\r?\n/).filter((l) => l.includes('='))
    .map((l) => [l.slice(0, l.indexOf('=')).trim(), l.slice(l.indexOf('=') + 1).trim()]),
)

const target = process.argv[2] ?? 'api'
const localFile = target === 'api' ? 'oj-api-m5' : 'oj-judge-m5'
const remote = target === 'api' ? '/opt/oj/oj-api' : '/opt/oj/oj-judge'
const service = target === 'api' ? 'oj-api' : 'oj-judge'

const gz = path.join(here, `.deploy-${target}.gz`)
execSync(`gzip -9 -c ${localFile} > ${gz}`, { cwd: here })
const b64 = fs.readFileSync(gz).toString('base64')
fs.unlinkSync(gz)
const localSha = crypto.createHash('sha256').update(fs.readFileSync(path.join(here, localFile))).digest('hex')
console.log(`payload ${(b64.length / 1e6).toFixed(1)} MB; local sha256=${localSha.slice(0, 16)}…`)

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
console.log('connected')

const stream = await new Promise((resolve, reject) => {
  conn.exec('bash -s 2>&1; echo STEP_EXIT=$?', (err, s) => (err ? reject(err) : resolve(s)))
})
let exitLine = ''
stream.on('data', (d) => process.stdout.write(d))
stream.on('close', (c) => { conn.end(); process.exit(c === 0 ? 0 : 1) })

// stdin order: heredoc opener, payload, closer, then decode+verify+swap.
// base64 -d reads ONLY the heredoc; everything after is shell script.
const head = `set -e
base64 -d > ${remote}.gz.new <<'OJB64EOF'
`
stream.write(head)
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
gunzip -c ${remote}.gz.new > ${remote}.bin && rm ${remote}.gz.new
chmod +x ${remote}.bin
REMOTE_SHA=$(sha256sum ${remote}.bin | cut -d' ' -f1)
echo "REMOTE_SHA=$REMOTE_SHA"
if [ "$REMOTE_SHA" != "${localSha}" ]; then echo SHA_MISMATCH; exit 9; fi
mv ${remote}.bin ${remote}
systemctl restart ${service}
sleep 4
systemctl is-active ${service}
`, resolve))
stream.end()