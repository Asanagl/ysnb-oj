<script setup lang="ts">
// 全页出题编辑器（用户审核流版）：同一大视窗布局，保存即提交审核。
// 题面用 Markdown 文本 + 右侧预览；无可见性/测试数据（审核通过后配置）。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MyProblems, errMsg } from '../api/client'
import { renderStatement } from '../utils/markdown'
import MarkdownEditor from '../components/MarkdownEditor.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { NumberInput } from '@/components/ui/number-input'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { FormField } from '@/components/ui/form-field'
import { Alert } from '@/components/ui/alert'
import { toast } from '@/lib/toast'

const route = useRoute()
const router = useRouter()
const problemId = computed(() => (route.params.id ? Number(route.params.id) : 0))
const loading = ref(false)
const saving = ref(false)

const form = reactive({
  title: '',
  statement_md: '',
  input_desc: '',
  output_desc: '',
  hint: '',
  time_limit_ms: 1000,
  mem_limit_mb: 256,
  judge_mode: 'default',
})

const statementHTML = computed(() => renderStatement(form.statement_md))

onMounted(async () => {
  if (!problemId.value) return
  loading.value = true
  try {
    const mine = await MyProblems.list()
    const p = mine.find((x) => x.id === problemId.value)
    if (!p) {
      toast.error('找不到该题目或不是你的题目')
      router.replace('/my/problems')
      return
    }
    Object.assign(form, {
      title: p.title ?? '', statement_md: p.statement_md ?? '',
      input_desc: p.input_desc ?? '', output_desc: p.output_desc ?? '',
      hint: p.hint ?? '', time_limit_ms: p.time_limit_ms ?? 1000,
      mem_limit_mb: p.mem_limit_mb ?? 256, judge_mode: p.judge_mode ?? 'default',
    })
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    loading.value = false
  }
})

async function save() {
  if (!form.title.trim() || !form.statement_md.trim()) {
    toast.warning('标题和题面不能为空')
    return
  }
  saving.value = true
  try {
    const payload = {
      title: form.title,
      statement_md: form.statement_md,
      input_desc: form.input_desc,
      output_desc: form.output_desc,
      hint: form.hint,
      time_limit_ms: form.time_limit_ms,
      mem_limit_mb: form.mem_limit_mb,
      judge_mode: form.judge_mode,
      tags: [],
      samples: [],
    }
    if (problemId.value) {
      await MyProblems.update(problemId.value, payload)
      toast.success('已保存；被驳回的题目将重新进入待审核')
    } else {
      await MyProblems.create(payload)
      toast.success('题目已提交审核，通过后将出现在公开题库')
    }
    router.push('/my/problems')
  } catch (e) {
    toast.error(errMsg(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="pe-root relative">
    <div
      v-if="loading"
      class="absolute inset-0 z-10 flex items-center justify-center bg-background/60 text-muted-foreground"
    >
      加载中…
    </div>
    <div class="pe-header">
      <h3 class="m-0">
        {{ problemId ? `编辑我的题目 #${problemId}` : '新建题目（提交审核）' }}
      </h3>
      <span class="flex-1" />
      <Button variant="outline" @click="router.back()">返回</Button>
      <Button :disabled="saving" @click="save">
        {{ saving ? '保存中…' : '保存并提交审核' }}
      </Button>
    </div>

    <div class="pe-columns">
      <Card class="pe-edit">
        <CardContent class="flex flex-col gap-4 p-6">
          <FormField label="标题">
            <Input v-model="form.title" placeholder="题目名称" />
          </FormField>
          <FormField label="题面（所见即所得，支持代码块/图片/链接；右侧实时预览）">
            <MarkdownEditor
              v-model="form.statement_md"
              placeholder="输入题面内容…"
              class="pe-statement-editor"
            />
          </FormField>
          <FormField label="输入描述">
            <Textarea v-model="form.input_desc" :rows="3" />
          </FormField>
          <FormField label="输出描述">
            <Textarea v-model="form.output_desc" :rows="3" />
          </FormField>
          <FormField label="提示（可选）">
            <Textarea v-model="form.hint" :rows="3" />
          </FormField>
          <div class="grid gap-3 sm:grid-cols-3">
            <FormField label="时限 ms">
              <NumberInput v-model="form.time_limit_ms" :min="100" :step="100" class="w-full" />
            </FormField>
            <FormField label="内存 MB">
              <NumberInput v-model="form.mem_limit_mb" :min="16" :step="16" class="w-full" />
            </FormField>
            <FormField label="判题模式">
              <Select v-model="form.judge_mode">
                <SelectTrigger>
                  <SelectValue placeholder="判题模式" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default">标准比对</SelectItem>
                  <SelectItem value="spj">SPJ 特判</SelectItem>
                  <SelectItem value="interactive">交互题</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
          </div>
          <Alert variant="info" title="测试数据与样例将在审核通过后由管理员协助配置" />
        </CardContent>
      </Card>

      <Card class="pe-preview">
        <CardContent class="p-6">
          <h4 class="mt-0">实时预览</h4>
          <h2 class="mt-0">{{ form.title || '（无标题）' }}</h2>
          <div class="pe-section" v-html="statementHTML" />
          <template v-if="form.input_desc || form.output_desc">
            <h4>输入格式</h4>
            <div class="pe-section" v-html="renderStatement(form.input_desc)" />
            <h4>输出格式</h4>
            <div class="pe-section" v-html="renderStatement(form.output_desc)" />
          </template>
          <template v-if="form.hint">
            <h4>提示</h4>
            <div class="pe-section" v-html="renderStatement(form.hint)" />
          </template>
        </CardContent>
      </Card>
    </div>
  </div>
</template>

<style scoped>
.pe-root {
  max-width: 1800px;
  margin: 0 auto;
}
.pe-header {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 12px;
}
.pe-columns {
  display: grid;
  grid-template-columns: minmax(0, 7fr) minmax(0, 5fr);
  gap: 16px;
  align-items: start;
}
.pe-edit {
  min-width: 0;
}
.pe-statement-editor {
  width: 100%;
}
.pe-statement-editor :deep(.wysiwyg-content) {
  min-height: 320px;
}
.pe-preview {
  position: sticky;
  top: 16px;
  max-height: calc(100vh - 90px);
  overflow: auto;
}
@media (max-width: 1279.98px) {
  .pe-columns {
    grid-template-columns: 1fr;
  }
  .pe-preview {
    position: static;
    max-height: none;
    order: -1;
  }
}
</style>
