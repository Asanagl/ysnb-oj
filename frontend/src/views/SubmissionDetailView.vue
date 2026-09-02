<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Submissions } from '../api/client'
import StatusTag from '../components/StatusTag.vue'

const route = useRoute()
const sub = ref<Awaited<ReturnType<typeof Submissions.get>> | null>(null)

onMounted(async () => {
  sub.value = await Submissions.get(route.params.id as string)
})
</script>

<template>
  <el-card v-if="sub">
    <h3>
      提交 #{{ sub.id }} · 题目 #{{ sub.problem_id }}
      <StatusTag :status="sub.status" />
    </h3>
    <p>
      语言 {{ sub.language }} · 耗时 {{ sub.time_ms }} ms · 内存 {{ sub.memory_kb }} KB ·
      提交于 {{ new Date(sub.created_at).toLocaleString() }}
    </p>
    <template v-if="sub.compile_message">
      <h4>编译信息</h4>
      <pre class="ce">{{ sub.compile_message }}</pre>
    </template>
    <el-table v-if="sub.cases.length" :data="sub.cases" size="small" style="margin-top: 12px">
      <el-table-column label="#" prop="index" width="70" />
      <el-table-column label="状态">
        <template #default="{ row }"><StatusTag :status="row.status" /></template>
      </el-table-column>
      <el-table-column label="耗时" width="110">
        <template #default="{ row }">{{ row.time_ms }} ms</template>
      </el-table-column>
      <el-table-column label="内存" width="110">
        <template #default="{ row }">{{ row.mem_kb }} KB</template>
      </el-table-column>
      <el-table-column label="信息" prop="message" />
    </el-table>
    <template v-if="sub.code !== undefined">
      <h4>代码</h4>
      <pre class="code">{{ sub.code }}</pre>
    </template>
    <el-alert v-else title="无权查看该提交代码（比赛结束前仅本人与管理员可见）" type="info" :closable="false" />
  </el-card>
</template>

<style scoped>
.ce,
.code {
  background: #282c34;
  color: #abb2bf;
  padding: 12px;
  border-radius: 6px;
  overflow: auto;
  text-align: left;
}
</style>
