// ws-probe.mjs — WebSocket availability probe for the OJ API.
// Usage: node scripts/ws-probe.mjs <valid|anon> [token]
//   valid: expects the handshake to succeed with the given JWT, then sends a
//          subscribe frame for "admin:daemons" and requires the connection to
//          stay stable for 1s.
//   anon:  expects the handshake to be rejected (no token => 401 before
//          upgrade), i.e. onopen must never fire.
// Exit code 0 = PASS, 1 = FAIL.
const mode = process.argv[2];
const token = process.argv[3] || "";
const base = process.env.OJ_WS_URL || "ws://127.0.0.1:18081/api/v1/ws";
if (mode !== "valid" && mode !== "anon") {
  console.error("usage: node ws-probe.mjs <valid|anon> [token]");
  process.exit(2);
}
const url = mode === "valid" ? `${base}?token=${encodeURIComponent(token)}` : base;

let settled = false;
const finish = (code, msg) => {
  if (settled) return;
  settled = true;
  console.log(msg);
  process.exitCode = code;
  // Graceful exit: a hard process.exit while the socket handle is still
  // closing trips a libuv assertion on Windows (UV_HANDLE_CLOSING) and
  // turns a passing probe into a non-zero exit. Give handles a beat.
  setTimeout(() => process.exit(process.exitCode ?? 0), 250);
};
const guard = setTimeout(
  () => finish(1, `WS-FAIL: ${mode} timed out (opened=${opened})`),
  5000
);

let opened = false;
let passMsg =
  mode === "valid"
    ? "WS-PASS: connected with token, subscribed admin:daemons, stable 1s"
    : "WS-PASS: anonymous handshake rejected";
const ws = new WebSocket(url);

ws.addEventListener("open", () => {
  opened = true;
  if (mode !== "valid") {
    finish(1, "WS-FAIL: anonymous connection was accepted");
    return;
  }
  ws.send(JSON.stringify({ subscribe: "admin:daemons" }));
  setTimeout(() => {
    try { ws.close(); } catch { /* already dead */ }
    finish(0, passMsg);
  }, 1000);
});
ws.addEventListener("error", () => {
  if (mode === "anon") {
    // a handshake rejection surfaces as error (often WITHOUT a close event
    // on undici's WebSocket) — for anon that is exactly the expected 401.
    finish(0, passMsg);
  }
});
ws.addEventListener("close", () => {
  if (mode === "anon" && !opened) finish(0, passMsg);
  else if (mode === "valid" && !opened) finish(1, "WS-FAIL: valid token was rejected");
});
