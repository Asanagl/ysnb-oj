<script setup lang="ts">
const props = defineProps<{ status: string }>()

const colorMap: Record<string, string> = {
  AC: '#67c23a',
  WA: '#f56c6c',
  TLE: '#e6a23c',
  MLE: '#e6a23c',
  RE: '#f56c6c',
  CE: '#909399',
  SE: '#909399',
  PENDING: '#909399',
  COMPILING: '#909399',
  JUDGING: '#409eff',
  // 比赛提交首败即停后被跳过的测试点（见 docs/judge-sandbox.md）
  SKIPPED: '#c0c4cc',
}
function color() {
  return colorMap[props.status] ?? '#909399'
}
</script>

<template>
  <el-tag :color="color()" style="color: #fff; border: none">{{ status }}</el-tag>
</template>
