<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Contests, errMsg, Problems } from '../api/client'
import { useResponsive } from '../composables/useResponsive'

const contests = ref<Awaited<ReturnType<typeof Contests.list>>>([])
const allProblems = ref<Awaited<ReturnType<typeof Problems.list>>['items']>([])
const selectedId = ref<number | null>(null)
const contestProblems = ref<Awaited<ReturnType<typeof Contests.get>>['problems']>([])
const bankPick = ref<number[]>([])
const { isPhone } = useResponsive()

const form = reactive({
  title: '',
  description: '',
  start_time: '',
  end_time: '',
  problem_ids: [] as number[],
  mode: 'acm',
  freeze_enabled: true,
  freeze_time: '',
  allow_manual_freeze: true,
  require_registration: true,
})

async function load() {
  contests.value = await Contests.list()
  allProblems.value = (await Problems.list({ size: 100 })).items
}
onMounted(load)

async function selectContest(id: number | null) {
  selectedId.value = id
  if (id == null) {
    contestProblems.value = []
    return
  }
  contestProblems.value = (await Contests.get(id)).problems
}

async function create() {
  try {
    const payload: Record<string, unknown> = {
      title: form.title,
      description: form.description,
      start_time: new Date(form.start_time).toISOString(),
      end_time: new Date(form.end_time).toISOString(),
      visibility: 'public',
      mode: form.mode,
      freeze_enabled: form.freeze_enabled,
      no_manual_freeze: !form.allow_manual_freeze,
      require_registration: form.require_registration,
    }
    if (form.freeze_enabled && form.freeze_time) {
      payload.freeze_time = new Date(form.freeze_time).toISOString()
    }
    const contest = await Contests.create(payload)
    if (form.problem_ids && form.problem_ids.length) {
      await Contests.setProblems(contest.id, form.problem_ids)
    }
    ElMessage.success('比赛已创建（题目已复制为独立副本）')
    form.title = ''
    form.problem_ids = []
    await load()
    await selectContest(contest.id)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function attachBank() {
  if (selectedId.value == null || bankPick.value.length === 0) return
  try {
    const r = (await Contests.setProblems(selectedId.value, bankPick.value)) as unknown as {
      copied?: number
    }
    ElMessage.success(`已复制 ${r.copied ?? bankPick.value.length} 道题的独立副本`)
    bankPick.value = []
    await selectContest(selectedId.value)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

const exclusiveDialog = ref(false)
const exForm = reactive({
  title: '',
  statement_md: '',
  time_limit_ms: 1000,
  mem_limit_mb: 256,
  judge_mode: 'default',
})

async function createExclusive() {
  if (selectedId.value == null) return
  try {
    const p = await Problems.create({
      title: exForm.title,
      statement_md: exForm.statement_md,
      time_limit_ms: exForm.time_limit_ms,
      mem_limit_mb: exForm.mem_limit_mb,
      visibility: 'hidden',
      judge_mode: exForm.judge_mode,
      tags: [],
      samples: [],
      contest_id: selectedId.value,
    })
    await Contests.setProblems(selectedId.value, [p.id])
    ElMessage.success('专属题已创建并挂入比赛')
    exclusiveDialog.value = false
    exForm.title = ''
    exForm.statement_md = ''
    await selectContest(selectedId.value)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

async function removeProblem(problemId: number) {
  try {
    await Problems.remove(problemId)
    ElMessage.success('已删除')
    if (selectedId.value != null) await selectContest(selectedId.value)
  } catch (e) {
    ElMessage.error(errMsg(e))
  }
}

const selectedContestTitle = computed(
  () => contests.value.find((c) => c.id === selectedId.value)?.title ?? '',
)
</script>

<template>
  <el-card>
    <h3>比赛管理</h3>
    <el-form label-width="90px" style="max-width: 640px">
      <el-form-item label="名称">
        <el-input v-model="form.title" />
      </el-form-item>
      <el-form-item label="说明">
        <el-input v-model="form.description" type="textarea" :rows="3" />
      </el-form-item>
      <el-form-item label="开始时间">
        <el-input v-model="form.start_time" placeholder="2026-09-01T14:00" />
      </el-form-item>
      <el-form-item label="结束时间">
        <el-input v-model="form.end_time" placeholder="2026-09-01T18:00" />
      </el-form-item>
      <el-row>
        <el-col :span="8">
          <el-form-item label="启用封榜">
            <el-switch v-model="form.freeze_enabled" />
          </el-form-item>
        </el-col>
        <el-col :span="8">
          <el-form-item label="赛时手动封榜">
            <el-switch v-model="form.allow_manual_freeze" />
          </el-form-item>
        </el-col>
        <el-col :span="8">
          <el-form-item label="需要报名">
            <el-switch v-model="form.require_registration" />
          </el-form-item>
        </el-col>
      </el-row>
      <el-form-item label="赛制">
        <el-radio-group v-model="form.mode">
          <el-radio value="acm">ACM（罚时 + 首对即停）</el-radio>
          <el-radio value="ioi">IOI（测试点部分分，按总分排名）</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item v-if="form.freeze_enabled && form.mode === 'acm'" label="封榜时间">
        <el-input v-model="form.freeze_time" placeholder="留空 = 不自动封榜，如 2026-09-01T17:00" />
      </el-form-item>
      <el-form-item label="初始题目">
        <el-select v-model="form.problem_ids" multiple style="width: 100%">
          <el-option v-for="p in allProblems" :key="p.id" :label="`${p.id}. ${p.title}`" :value="p.id" />
        </el-select>
      </el-form-item>
      <el-button type="primary" @click="create">创建（{{ form.mode === 'ioi' ? 'IOI' : 'ACM' }} 赛制）</el-button>
    </el-form>

    <h4 style="margin-top: 20px">题目管理（每场比赛的题目为独立副本）</h4>
    <el-select
      :model-value="selectedId"
      placeholder="选择比赛"
      style="width: 320px"
      @update:model-value="selectContest"
    >
      <el-option v-for="c in contests" :key="c.id" :label="`${c.id}. ${c.title}`" :value="c.id" />
    </el-select>

    <template v-if="selectedId != null">
      <div style="display: flex; gap: 12px; align-items: center; margin: 12px 0; flex-wrap: wrap">
        <el-select
          v-model="bankPick"
          multiple
          placeholder="从题库选择要复制进比赛的题"
          style="width: min(420px, 100%)"
        >
          <el-option v-for="p in allProblems" :key="p.id" :label="`${p.id}. ${p.title}`" :value="p.id" />
        </el-select>
        <el-button type="primary" @click="attachBank">复制挂入（独立副本）</el-button>
        <el-button type="success" @click="exclusiveDialog = true">新建专属题</el-button>
      </div>
      <el-table :data="contestProblems" size="small">
        <el-table-column label="标签" prop="label" width="70" />
        <el-table-column label="题目 ID" prop="id" width="100" />
        <el-table-column label="标题" prop="title" />
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-popconfirm title="删除该题及其测试数据？" @confirm="removeProblem(row.id)">
              <template #reference>
                <el-button size="small" type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
      <p v-if="contestProblems.length === 0" style="color: #909399">
        「{{ selectedContestTitle }}」还没有题目
      </p>
    </template>

    <h4 style="margin-top: 16px">全部比赛</h4>
    <el-table :data="contests" size="small">
      <el-table-column label="#" prop="id" width="70" />
      <el-table-column label="名称" prop="title" />
      <el-table-column label="开始">
        <template #default="{ row }">{{ new Date(row.start_time).toLocaleString() }}</template>
      </el-table-column>
      <el-table-column label="结束">
        <template #default="{ row }">{{ new Date(row.end_time).toLocaleString() }}</template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="exclusiveDialog" title="新建比赛专属题" :width="isPhone ? '95%' : '60%'">
      <el-form :label-width="isPhone ? undefined : '90px'" :label-position="isPhone ? 'top' : 'right'">
        <el-form-item label="标题">
          <el-input v-model="exForm.title" />
        </el-form-item>
        <el-form-item label="题面 MD">
          <el-input v-model="exForm.statement_md" type="textarea" :rows="6" />
        </el-form-item>
        <el-row>
          <el-col :span="8">
            <el-form-item label="时限 ms">
              <el-input-number v-model="exForm.time_limit_ms" :min="100" :step="100" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="内存 MB">
              <el-input-number v-model="exForm.mem_limit_mb" :min="16" :step="16" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="判题模式">
              <el-select v-model="exForm.judge_mode">
                <el-option label="标准比对" value="default" />
                <el-option label="SPJ 特判" value="spj" />
                <el-option label="交互题" value="interactive" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>
        <el-alert
          title="创建后可在题库管理里找到该题（标记为比赛专属）上传测试数据；它不会出现在公共题库"
          type="info"
          :closable="false"
        />
      </el-form>
      <template #footer>
        <el-button @click="exclusiveDialog = false">取消</el-button>
        <el-button type="primary" @click="createExclusive">创建并挂入比赛</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>
