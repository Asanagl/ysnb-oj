<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Contests, errMsg, Problems } from '../api/client'
import { toast } from '@/lib/toast'
import { confirmDialog } from '@/lib/confirm'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { NumberInput } from '@/components/ui/number-input'
import { Switch } from '@/components/ui/switch'
import { Checkbox } from '@/components/ui/checkbox'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { FormField } from '@/components/ui/form-field'
import { Alert } from '@/components/ui/alert'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

const contests = ref<Awaited<ReturnType<typeof Contests.list>>>([])
const allProblems = ref<Awaited<ReturnType<typeof Problems.list>>['items']>([])
const selectedId = ref<number | null>(null)
const contestProblems = ref<Awaited<ReturnType<typeof Contests.get>>['problems']>([])
const bankPick = ref<number[]>([])

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

// Select 的 model 不含 null，这里做一层 null ↔ undefined 的桥接
const contestSelect = computed({
  get: () => selectedId.value ?? undefined,
  set: (v: string | number | undefined) => {
    void selectContest(v == null ? null : Number(v))
  },
})

// ui Select 只支持单选，初始题目/题库挑选用复选列表维持 number[] 语义
function togglePick(list: number[], id: number) {
  const i = list.indexOf(id)
  if (i >= 0) list.splice(i, 1)
  else list.push(id)
}

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
    toast.success('比赛已创建（题目已复制为独立副本）')
    form.title = ''
    form.problem_ids = []
    await load()
    await selectContest(contest.id)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function attachBank() {
  if (selectedId.value == null || bankPick.value.length === 0) return
  try {
    const r = (await Contests.setProblems(selectedId.value, bankPick.value)) as unknown as {
      copied?: number
    }
    toast.success(`已复制 ${r.copied ?? bankPick.value.length} 道题的独立副本`)
    bankPick.value = []
    await selectContest(selectedId.value)
  } catch (e) {
    toast.error(errMsg(e))
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
    toast.success('专属题已创建并挂入比赛')
    exclusiveDialog.value = false
    exForm.title = ''
    exForm.statement_md = ''
    await selectContest(selectedId.value)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

async function removeProblem(problemId: number) {
  if (!(await confirmDialog({ title: '删除该题及其测试数据？', danger: true }))) return
  try {
    await Problems.remove(problemId)
    toast.success('已删除')
    if (selectedId.value != null) await selectContest(selectedId.value)
  } catch (e) {
    toast.error(errMsg(e))
  }
}

const selectedContestTitle = computed(
  () => contests.value.find((c) => c.id === selectedId.value)?.title ?? '',
)
</script>

<template>
  <Card>
    <CardContent class="p-6">
      <h3 class="mb-3 text-lg font-semibold">比赛管理</h3>
      <div class="max-w-[640px] space-y-4">
        <FormField label="名称">
          <Input v-model="form.title" />
        </FormField>
        <FormField label="说明">
          <Textarea v-model="form.description" :rows="3" />
        </FormField>
        <FormField label="开始时间">
          <Input v-model="form.start_time" placeholder="2026-09-01T14:00" />
        </FormField>
        <FormField label="结束时间">
          <Input v-model="form.end_time" placeholder="2026-09-01T18:00" />
        </FormField>
        <div class="grid gap-3 sm:grid-cols-3">
          <FormField label="启用封榜">
            <Switch v-model="form.freeze_enabled" />
          </FormField>
          <FormField label="赛时手动封榜">
            <Switch v-model="form.allow_manual_freeze" />
          </FormField>
          <FormField label="需要报名">
            <Switch v-model="form.require_registration" />
          </FormField>
        </div>
        <FormField label="赛制">
          <RadioGroup v-model="form.mode" class="flex gap-4">
            <div class="flex items-center gap-2">
              <RadioGroupItem id="mode-acm" value="acm" />
              <label for="mode-acm" class="cursor-pointer text-sm">ACM（罚时 + 首对即停）</label>
            </div>
            <div class="flex items-center gap-2">
              <RadioGroupItem id="mode-ioi" value="ioi" />
              <label for="mode-ioi" class="cursor-pointer text-sm">IOI（测试点部分分，按总分排名）</label>
            </div>
          </RadioGroup>
        </FormField>
        <FormField v-if="form.freeze_enabled && form.mode === 'acm'" label="封榜时间">
          <Input v-model="form.freeze_time" placeholder="留空 = 不自动封榜，如 2026-09-01T17:00" />
        </FormField>
        <FormField label="初始题目">
          <div class="max-h-48 space-y-1 overflow-auto rounded-md border border-border p-2">
            <label
              v-for="p in allProblems"
              :key="p.id"
              class="flex cursor-pointer items-center gap-2 text-sm"
            >
              <Checkbox
                :model-value="form.problem_ids.includes(p.id)"
                @update:model-value="togglePick(form.problem_ids, p.id)"
              />
              {{ p.id }}. {{ p.title }}
            </label>
          </div>
        </FormField>
        <Button @click="create">创建（{{ form.mode === 'ioi' ? 'IOI' : 'ACM' }} 赛制）</Button>
      </div>

      <h4 class="mb-2 mt-5 text-base font-semibold">题目管理（每场比赛的题目为独立副本）</h4>
      <Select v-model="contestSelect">
        <SelectTrigger class="w-80">
          <SelectValue placeholder="选择比赛" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="c in contests" :key="c.id" :value="c.id">
            {{ c.id }}. {{ c.title }}
          </SelectItem>
        </SelectContent>
      </Select>

      <template v-if="selectedId != null">
        <div class="my-3 flex flex-wrap items-end gap-3">
          <div class="w-full max-w-[420px]">
            <p class="mb-1 text-xs text-muted-foreground">从题库选择要复制进比赛的题</p>
            <div class="max-h-40 space-y-1 overflow-auto rounded-md border border-border p-2">
              <label
                v-for="p in allProblems"
                :key="p.id"
                class="flex cursor-pointer items-center gap-2 text-sm"
              >
                <Checkbox
                  :model-value="bankPick.includes(p.id)"
                  @update:model-value="togglePick(bankPick, p.id)"
                />
                {{ p.id }}. {{ p.title }}
              </label>
            </div>
          </div>
          <Button @click="attachBank">复制挂入（独立副本）</Button>
          <Button variant="secondary" @click="exclusiveDialog = true">新建专属题</Button>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead class="w-[70px]">标签</TableHead>
              <TableHead class="w-[100px]">题目 ID</TableHead>
              <TableHead>标题</TableHead>
              <TableHead class="w-[100px]">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in contestProblems" :key="row.id">
              <TableCell>{{ row.label }}</TableCell>
              <TableCell>{{ row.id }}</TableCell>
              <TableCell>{{ row.title }}</TableCell>
              <TableCell>
                <Button size="sm" variant="destructive" @click="removeProblem(row.id)">删除</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <p v-if="contestProblems.length === 0" class="mt-2 text-sm text-muted-foreground">
          「{{ selectedContestTitle }}」还没有题目
        </p>
      </template>

      <h4 class="mb-2 mt-4 text-base font-semibold">全部比赛</h4>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-[70px]">#</TableHead>
            <TableHead>名称</TableHead>
            <TableHead>开始</TableHead>
            <TableHead>结束</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in contests" :key="row.id">
            <TableCell>{{ row.id }}</TableCell>
            <TableCell>{{ row.title }}</TableCell>
            <TableCell>{{ new Date(row.start_time).toLocaleString() }}</TableCell>
            <TableCell>{{ new Date(row.end_time).toLocaleString() }}</TableCell>
          </TableRow>
        </TableBody>
      </Table>

      <Dialog v-model:open="exclusiveDialog">
        <DialogContent class="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>新建比赛专属题</DialogTitle>
          </DialogHeader>
          <div class="space-y-4">
            <FormField label="标题">
              <Input v-model="exForm.title" />
            </FormField>
            <FormField label="题面 MD">
              <Textarea v-model="exForm.statement_md" :rows="6" />
            </FormField>
            <div class="grid gap-3 sm:grid-cols-3">
              <FormField label="时限 ms">
                <NumberInput v-model="exForm.time_limit_ms" :min="100" :step="100" />
              </FormField>
              <FormField label="内存 MB">
                <NumberInput v-model="exForm.mem_limit_mb" :min="16" :step="16" />
              </FormField>
              <FormField label="判题模式">
                <Select v-model="exForm.judge_mode">
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="default">标准比对</SelectItem>
                    <SelectItem value="spj">SPJ 特判</SelectItem>
                    <SelectItem value="interactive">交互题</SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
            </div>
            <Alert
              variant="info"
              title="创建后可在题库管理里找到该题（标记为比赛专属）上传测试数据；它不会出现在公共题库"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" @click="exclusiveDialog = false">取消</Button>
            <Button @click="createExclusive">创建并挂入比赛</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </CardContent>
  </Card>
</template>
