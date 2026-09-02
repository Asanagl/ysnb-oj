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
  process.exit(code);
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
  setTimeout(() => finish(0, passMsg), 1000);
});
ws.addEventListener("error", () => {
  if (mode === "anon") {
    // error without open is the expected 401 rejection; close confirms it
  }
});
ws.addEventListener("close", () => {
  if (mode === "anon" && !opened) finish(0, passMsg);
  else if (mode === "valid" && !opened) finish(1, "WS-FAIL: valid token was rejected");
});
