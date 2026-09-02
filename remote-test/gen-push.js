// Generates a self-contained push script: base64 payload + heredoc that the
// remote `bash -s` decodes to the target path. Remote targets are an internal
// table (never argv) so MSYS path conversion cannot mangle them.
//   node gen-push.js <local-file> <target-key> <out-script>
import fs from 'fs'

const TARGETS = {
  seccprobe: '/root/ojtest/seccprobe',
  judge: '/opt/oj/oj-judge',
  api: '/opt/oj/oj-api',
  testdata: '/root/ojtest/upload.zip',
  web: '/opt/oj/web.zip',
}

const [,, local, key, out] = process.argv
const remote = TARGETS[key]
if (!local || !remote || !out) {
  console.error(`usage: node gen-push.js <local-file> <${Object.keys(TARGETS).join('|')}> <out-script>`)
  process.exit(1)
}
const b64 = fs.readFileSync(local).toString('base64')
// chunk to 76 columns like standard base64 output
const lines = b64.match(/.{1,76}/g) ?? []
const script = `set -e
mkdir -p "$(dirname "${remote}")"
base64 -d > "${remote}" <<'OJB64EOF'
${lines.join('\n')}
OJB64EOF
echo "PUSHED $(wc -c < "${remote}") bytes -> ${remote}"
`
fs.writeFileSync(out, script)
console.log('WROTE', out, `${(script.length / 1024 / 1024).toFixed(1)} MB -> ${remote}`)
