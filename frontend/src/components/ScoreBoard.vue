<script setup lang="ts">
import type { StandingRow } from '../api/client'

// CF-style scoreboard grid shared by the contest page and the projection
// screen. `large` switches to projection scale (meters-away readable).
defineProps<{
  rows: StandingRow[]
  problems: { label: string }[]
  large?: boolean
}>()

function cellClass(cell?: { solved: boolean; attempts: number; pending: number }) {
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
</script>

<template>
  <div>
    <div class="b-legendbar">
      <span class="b-legend"><span class="b-legend-chip ok" /> AC（用时'）</span>
      <span class="b-legend"><span class="b-legend-chip fail" /> -失败次数</span>
      <span class="b-legend"><span class="b-legend-chip pend" /> ?待判/冻结</span>
      <span class="b-legend-note">★ 打星不占名次 · 红名为作弊</span>
    </div>
    <div class="board" :class="{ large }" :style="{ '--n': problems.length }">
    <div class="b-row b-head">
      <div class="b-rank">#</div>
      <div class="b-user">用户</div>
      <div class="b-solved">解题</div>
      <div class="b-penalty">罚时</div>
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
        {{ Math.round(row.penalty_ms / 60000) }}<span class="b-penalty-unit">分</span>
      </div>
      <div
        v-for="p in problems"
        :key="p.label"
        class="b-cell"
        :class="cellClass(row.cells[p.label])"
      >
        <template v-if="row.cells[p.label]">
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
      </div>
    </div>
    <div v-if="rows.length === 0" class="b-empty">暂无有效提交</div>
    </div>
  </div>
</template>

<style scoped>
/* Light theme (default): matches the Element Plus palette used site-wide.
   The projection tier (.large) re-covers with the dark high-contrast board. */
.board {
  --cell: 56px;
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 8px;
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
  background: #fafafa;
}
.b-head {
  color: #909399;
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
  color: #67c23a;
  font-size: 18px;
  font-weight: 700;
}
.b-penalty {
  color: #303133;
  font-size: 16px;
  font-weight: 600;
}
.b-penalty-unit {
  font-size: 11px;
  color: #909399;
  margin-left: 2px;
}
.b-user-link {
  color: #303133;
  text-decoration: none;
}
.b-user-link:hover {
  color: #409eff;
}
.b-cheat .b-user-link {
  color: #f56c6c;
}
.b-star .b-user-link {
  color: #e6a23c;
}
.b-cell {
  min-height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  border-radius: 5px;
  background: #f5f7fa;
  font-size: 13px;
  font-weight: 600;
}
.b-cell.ok {
  background: #67c23a;
  color: #fff;
}
.b-cell.fail {
  background: rgba(245, 108, 108, 0.14);
  color: #f56c6c;
}
.b-cell.pend {
  background: rgba(230, 162, 60, 0.18);
  color: #e6a23c;
}
.b-att {
  font-size: 11px;
  opacity: 0.85;
}
.b-empty {
  color: #909399;
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
  color: #909399;
}
.b-legend-note {
  font-size: 12px;
  color: #909399;
}
.b-legend-chip {
  width: 14px;
  height: 14px;
  border-radius: 3px;
  display: inline-block;
}
.b-legend-chip.ok {
  background: #67c23a;
}
.b-legend-chip.fail {
  background: rgba(245, 108, 108, 0.4);
}
.b-legend-chip.pend {
  background: rgba(230, 162, 60, 0.45);
}
@media (max-width: 767.98px) {
  .board {
    --cell: 44px;
  }
  .b-row {
    min-width: 560px;
  }
}

/* Projection tier: dark, meters-away readability. */
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
