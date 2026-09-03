<script setup lang="ts">
import type { StandingRow } from '../api/client'

// CF-style scoreboard grid shared by the contest page and the projection
// screen. `large` switches to projection scale (meters-away readable).
//
// mode fork (backend contract, see computeIOIStandings in
// backend/internal/handler/contests.go): in IOI contests penalty_ms carries
// the row's TOTAL SCORE and each cell's solved_ms holds that problem's best
// partial score; in ACM they are penalty time / solve time in milliseconds.
// pending masks post-freeze submissions in both modes — never reveal early.
const props = withDefaults(
  defineProps<{
    rows: StandingRow[]
    problems: { label: string }[]
    large?: boolean
    mode?: 'acm' | 'ioi'
  }>(),
  { mode: 'acm' },
)

function cellClass(cell?: { solved: boolean; attempts: number; pending: number; solved_ms: number }) {
  if (!cell) return ''
  if (cell.solved) return 'ok'
  if (cell.pending) return 'pend'
  if (cell.attempts) return 'fail'
  return ''
}

function rankText(row: StandingRow) {
  if (row.cheated) return '作弊'
  if (row.starred) return '★'
  return String(row.rank)
}

// IOI cells without any judged attempt and without pending show nothing;
// a judged-but-zero-score attempt shows 0 explicitly (partial credit matters).
function ioiCellScore(cell: { attempts: number; solved_ms: number }) {
  return cell.attempts > 0 ? String(cell.solved_ms) : ''
}
</script>

<template>
  <div>
    <div class="b-legendbar">
      <span class="b-legend">
        <span class="b-legend-chip ok" /> {{ props.mode === 'ioi' ? '满分' : 'AC（用时\'）' }}
      </span>
      <span class="b-legend">
        <span class="b-legend-chip fail" /> {{ props.mode === 'ioi' ? '部分分/未过' : '-失败次数' }}
      </span>
      <span class="b-legend"><span class="b-legend-chip pend" /> ?待判/冻结</span>
      <span class="b-legend-note">★ 打星不占名次 · 红名为作弊</span>
    </div>
    <div class="board" :class="{ large }" :style="{ '--n': problems.length }">
      <div class="b-row b-head">
        <div class="b-rank">#</div>
        <div class="b-user">用户</div>
        <div class="b-solved">{{ props.mode === 'ioi' ? '满分题' : '解题' }}</div>
        <div class="b-penalty">{{ props.mode === 'ioi' ? '总分' : '罚时' }}</div>
        <div v-for="p in problems" :key="p.label" class="b-cell">{{ p.label }}</div>
      </div>
      <div
        v-for="row in rows"
        :key="row.user_id"
        class="b-row"
        :class="{ 'b-cheat': row.cheated, 'b-star': row.starred }"
      >
        <div class="b-rank">{{ rankText(row) }}</div>
        <div class="b-user">
          <router-link :to="`/users/${row.user_id}`" class="b-user-link">
            <template v-if="row.cheated">🚩 {{ row.username || row.user_id }}</template>
            <template v-else-if="row.starred">★ {{ row.username || row.user_id }}</template>
            <template v-else>{{ row.username || row.user_id }}</template>
          </router-link>
        </div>
        <div class="b-solved">{{ row.solved }}</div>
        <div class="b-penalty">
          <template v-if="props.mode === 'ioi'">{{ row.penalty_ms }}</template>
          <template v-else>
            {{ Math.round(row.penalty_ms / 60000) }}<span class="b-penalty-unit">分</span>
          </template>
        </div>
        <div
          v-for="p in problems"
          :key="p.label"
          class="b-cell"
          :class="cellClass(row.cells[p.label])"
        >
          <template v-if="row.cells[p.label]">
            <template v-if="props.mode === 'ioi'">
              <span class="b-time">{{ ioiCellScore(row.cells[p.label]) }}</span>
              <span v-if="row.cells[p.label].pending" class="b-pend">
                ?{{ row.cells[p.label].pending }}
              </span>
            </template>
            <template v-else>
              <span v-if="row.cells[p.label].attempts" class="b-att">
                -{{ row.cells[p.label].attempts }}
              </span>
              <span v-if="row.cells[p.label].solved" class="b-time">
                +{{ Math.round(row.cells[p.label].solved_ms / 60000) }}'
              </span>
              <span v-if="row.cells[p.label].pending" class="b-pend">
                ?{{ row.cells[p.label].pending }}
              </span>
            </template>
          </template>
        </div>
      </div>
      <div v-if="rows.length === 0" class="b-empty">暂无有效提交</div>
    </div>
  </div>
</template>

<style scoped>
/* Colors come from the design tokens (style.css) so the board follows the
   light/dark theme; the projection tier (.large) stays fixed dark because it
   targets venue screens, not the page theme. */
.board {
  --cell: 56px;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 10px;
  overflow-x: auto;
}
.b-row {
  display: grid;
  grid-template-columns: 56px minmax(120px, 1.6fr) 64px 72px repeat(var(--n), var(--cell));
  gap: 4px;
  align-items: center;
  min-width: 620px;
  padding: 3px 4px;
  border-radius: 6px;
}
.b-row:nth-child(odd):not(.b-head) {
  background: var(--muted);
}
.b-head {
  color: var(--muted-foreground);
  font-size: 12px;
  text-transform: uppercase;
}
.b-rank,
.b-solved,
.b-penalty,
.b-cell {
  text-align: center;
}
.b-solved {
  color: var(--ac);
  font-size: 18px;
  font-weight: 700;
}
.b-penalty {
  color: var(--foreground);
  font-size: 16px;
  font-weight: 600;
  font-family: var(--font-mono);
}
.b-penalty-unit {
  font-size: 11px;
  color: var(--muted-foreground);
  margin-left: 2px;
}
.b-user-link {
  color: var(--foreground);
  text-decoration: none;
}
.b-user-link:hover {
  color: var(--primary);
}
.b-cheat .b-user-link {
  color: var(--wa);
}
.b-star .b-user-link {
  color: var(--tle);
}
.b-cell {
  min-height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  border-radius: 5px;
  background: var(--muted);
  color: var(--foreground);
  font-size: 13px;
  font-weight: 600;
  font-family: var(--font-mono);
}
.b-cell.ok {
  background: var(--ac);
  color: #fff;
}
.b-cell.fail {
  background: var(--wa-bg);
  color: var(--wa);
}
.b-cell.pend {
  background: var(--tle-bg);
  color: var(--tle);
}
.b-att {
  font-size: 11px;
  opacity: 0.85;
}
.b-empty {
  color: var(--muted-foreground);
  text-align: center;
  padding: 16px;
}
.b-legendbar {
  display: flex;
  gap: 16px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 8px;
}
.b-legend {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--muted-foreground);
}
.b-legend-note {
  font-size: 12px;
  color: var(--muted-foreground);
}
.b-legend-chip {
  width: 14px;
  height: 14px;
  border-radius: 3px;
  display: inline-block;
}
.b-legend-chip.ok {
  background: var(--ac);
}
.b-legend-chip.fail {
  background: var(--wa-bg);
  border: 1px solid var(--wa);
}
.b-legend-chip.pend {
  background: var(--tle-bg);
  border: 1px solid var(--tle);
}
@media (max-width: 767.98px) {
  .board {
    --cell: 44px;
  }
  .b-row {
    min-width: 560px;
  }
}

/* Projection tier: dark, meters-away readability (theme-independent). */
.board.large {
  --cell: 96px;
  background: #0d1117;
  border: none;
  padding: 20px;
  border-radius: 12px;
}
.board.large .b-row {
  grid-template-columns: 110px minmax(200px, 1.6fr) 110px 130px repeat(var(--n), var(--cell));
  gap: 8px;
  min-width: 900px;
  padding: 8px 10px;
}
.board.large .b-row:nth-child(odd):not(.b-head) {
  background: #10161f;
}
.board.large .b-head {
  color: #8b949e;
  font-size: 22px;
}
.board.large .b-rank {
  font-size: 30px;
  font-weight: 700;
  color: #e6edf3;
}
.board.large .b-solved {
  font-size: 40px;
  color: #7ee787;
}
.board.large .b-penalty {
  font-size: 32px;
  color: #e6edf3;
}
.board.large .b-penalty-unit {
  font-size: 18px;
  color: #8b949e;
}
.board.large .b-user-link {
  font-size: 26px;
  font-weight: 600;
  color: #e6edf3;
}
.board.large .b-user-link:hover {
  color: #58a6ff;
}
.board.large .b-cheat .b-user-link {
  color: #f85149;
}
.board.large .b-star .b-user-link {
  color: #d29922;
}
.board.large .b-cell {
  min-height: 68px;
  font-size: 22px;
  border-radius: 8px;
  background: #161b22;
  color: #e6edf3;
}
.board.large .b-cell.ok {
  background: #1f6f43;
  color: #fff;
}
.board.large .b-cell.fail {
  background: rgba(248, 81, 73, 0.18);
  color: #f85149;
}
.board.large .b-cell.pend {
  background: rgba(210, 153, 34, 0.25);
  color: #d29922;
}
.board.large .b-att {
  font-size: 16px;
}
.board.large .b-time {
  font-size: 22px;
}
.board.large .b-empty {
  font-size: 28px;
  color: #8b949e;
}
.board.large .b-legend {
  color: #8b949e;
  font-size: 16px;
}
.board.large .b-legend-note {
  color: #8b949e;
  font-size: 16px;
}
.board.large .b-legend-chip.ok {
  background: #1f6f43;
}
.board.large .b-legend-chip.fail {
  background: rgba(248, 81, 73, 0.3);
}
.board.large .b-legend-chip.pend {
  background: rgba(210, 153, 34, 0.35);
}
</style>
