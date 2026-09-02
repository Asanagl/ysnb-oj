// Minimal SSH driver for the cloud test session.
//
// Security design (per Mimosa review): this driver runs exactly ONE fixed,
// hard-coded remote command — `bash -s` — and feeds it a locally authored
// script file through the exec channel's stdin. No argv data is ever spliced
// into a shell string; SFTP is not used at all. Credentials live in .sshenv.
//
//   node ssh.js runstdin <script.sh>   stream a local script to remote bash -s
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

const [,, cmd, arg] = process.argv

const conn = new Client()
function connect() {
  return new Promise((resolve, reject) => {
    conn.on('ready', resolve)
    conn.on('error', reject)
    conn.connect({
      host: env.HOST,
      port: Number(env.PORT || 22),
      username: env.USER || 'root',
      password: env.PASSWORD,
      readyTimeout: 20000,
    })
  })
}

async function run() {
  await connect()
  if (cmd === 'runstdin') {
    if (!arg) throw new Error('runstdin needs <script.sh>')
    if (!fs.existsSync(arg)) throw new Error(`local script missing: ${arg}`)
    // Fixed literal: the only remote command this driver can ever run.
    conn.exec('bash -s 2>&1; echo STEP_EXIT=$?', (err, stream) => {
      if (err) { console.error('EXEC_ERR', err.message); process.exit(2) }
      stream.on('close', (c) => { conn.end(); process.exit(c ?? 0) })
      stream.on('data', (d) => process.stdout.write(d))
      stream.stderr.on('data', (d) => process.stderr.write(d))
      fs.createReadStream(arg).pipe(stream.stdin)
    })
    return
  }
  console.error('usage: node ssh.js runstdin <script.sh>')
  process.exit(1)
}

run().catch((e) => { console.error('SSH_FAIL', e.message); process.exit(3) })
