// Push the freshly cross-compiled oj-api / oj-judge to the server via the
// key-based SSH channel (gzip+base64 through stdin heredoc, sha256 verified
// end to end before swap — same pattern as m5-deploy3, now key auth).
// Usage: node remote-test/log-deploy-push.mjs   (services are already stopped)
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

const jobs = [
  { local: path.join(repo, 'dist', 'oj-api-linux'), remote: '/opt/oj/oj-api.new' },
  { local: path.join(repo, 'dist', 'oj-judge-linux'), remote: '/opt/oj/oj-judge.new' },
]

function connect() {
  return new Promise((resolve, reject) => {
    const conn = new Client()
    conn.on('ready', () => resolve(conn))
    conn.on('error', reject)
    conn.connect({ host, port: 22, username: 'root', privateKey: fs.readFileSync(keyFile), readyTimeout: 20000 })
  })
}

function push(conn, localFile, remotePath) {
  return new Promise((resolve, reject) => {
    execSync(`gzip -9 -c "${localFile}" > "${localFile}.gz.tmp"`)
    const b64 = fs.readFileSync(`${localFile}.gz.tmp`).toString('base64')
    fs.unlinkSync(`${localFile}.gz.tmp`)
    const localSha = crypto.createHash('sha256').update(fs.readFileSync(localFile)).digest('hex')
    console.log(`${path.basename(localFile)}: ${(b64.length / 1e6).toFixed(1)} MB b64, sha=${localSha.slice(0, 16)}…`)

    conn.exec('bash -s 2>&1; echo STEP_EXIT=$?', (err, s) => {
      if (err) return reject(err)
      let out = ''
      s.on('data', (d) => { out += d.toString(); process.stdout.write(d) })
      s.stderr?.on('data', (d) => process.stdout.write(d))
      s.on('close', (code) => (code === 0 ? resolve(localSha) : reject(new Error(`push failed exit=${code}`))))

      const head = `set -e
base64 -d > ${remotePath}.gz <<'OJB64EOF'
`
      s.write(head)
      const CHUNK = 32 * 1024
      let i = 0
      function writeNext() {
        while (i < b64.length) {
          const chunk = b64.slice(i, i + CHUNK)
          i += CHUNK
          if (!s.write(chunk + '\n')) { s.once('drain', writeNext); return }
        }
        // payload done: close heredoc, decode, verify sha, place file
        s.end(`OJB64EOF
gunzip -c ${remotePath}.gz > ${remotePath}.bin && rm ${remotePath}.gz
chmod +x ${remotePath}.bin
REMOTE_SHA=$(sha256sum ${remotePath}.bin | cut -d' ' -f1)
echo "REMOTE_SHA=$REMOTE_SHA"
if [ "$REMOTE_SHA" != "${localSha}" ]; then echo SHA_MISMATCH; exit 9; fi
mv ${remotePath}.bin ${remotePath}
echo "PLACED=${remotePath}"
`)
      }
      writeNext()
    })
  })
}

const conn = await connect()
console.log('connected via key')
try {
  for (const j of jobs) await push(conn, j.local, j.remote)
  console.log('PUSH_ALL_OK')
} finally {
  conn.end()
}
