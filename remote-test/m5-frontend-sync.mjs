// Frontend dist sync: tar+gzip the local dist, push via the ssh.js-style
// base64+heredoc channel, atomic swap + nginx needs no reload (static root).
import { Client } from 'ssh2'
import fs from 'fs'
import path from 'path'
import { execSync } from 'child_process'
import crypto from 'crypto'
import { fileURLToPath } from 'url'

const here = path.dirname(fileURLToPath(import.meta.url))
const root = path.join(here, '..')
const env = Object.fromEntries(
  fs.readFileSync(path.join(here, '.sshenv'), 'utf8')
    .split(/\r?\n/).filter((l) => l.includes('='))
    .map((l) => [l.slice(0, l.indexOf('=')).trim(), l.slice(l.indexOf('=') + 1).trim()]),
)

const tarball = path.join(here, 'dist.tar.gz')
// Git Bash tar mis-parses C:\ paths as host:port; use POSIX form via cwd.
execSync(`tar -czf dist.tar.gz -C ../frontend dist`, { cwd: here, stdio: 'inherit' })
const b64 = fs.readFileSync(tarball).toString('base64')
const sha = crypto.createHash('sha256').update(fs.readFileSync(tarball)).digest('hex')
console.log(`dist tarball ${(b64.length / 1e6).toFixed(1)} MB base64; sha=${sha.slice(0, 12)}…`)

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
stream.on('data', (d) => process.stdout.write(d))
stream.on('close', (c) => { conn.end(); process.exit(c === 0 ? 0 : 1) })

stream.write(`set -e
rm -rf /opt/oj/web.new
mkdir -p /opt/oj/web.new
base64 -d > /opt/oj/web.tar.gz.new <<'OJB64EOF'
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
REMOTE_SHA=$(sha256sum /opt/oj/web.tar.gz.new | cut -d' ' -f1)
echo "REMOTE_SHA=$REMOTE_SHA"
if [ "$REMOTE_SHA" != "${sha}" ]; then echo SHA_MISMATCH; exit 9; fi
tar -xzf /opt/oj/web.tar.gz.new -C /opt/oj/web.new
rm /opt/oj/web.tar.gz.new
rm -rf /opt/oj/web.old
[ -d /opt/oj/web ] && mv /opt/oj/web /opt/oj/web.old
mv /opt/oj/web.new/dist /opt/oj/web
rmdir /opt/oj/web.new
curl -s -o /dev/null -w "index HTTP=%{http_code}\\n" http://127.0.0.1/
`, resolve))
stream.end()