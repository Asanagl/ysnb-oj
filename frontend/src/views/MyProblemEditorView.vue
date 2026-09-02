<script setup lang="ts">
// 全页出题编辑器（用户审核流版）：同一大视窗布局，保存即提交审核。
// 题面用 Markdown 文本 + 右侧预览；无可见性/测试数据（审核通过后配置）。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { MyProblems, errMsg } from '../api/client'
import { renderStatement } from '../utils/markdown'
import MarkdownEditor from '../components/MarkdownEditor.vue'

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
      ElMessage.error('找不到该题目或不是你的题目')
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
    ElMessage.error(errMsg(e))
  } finally {
    loading.value = false
  }
})

async function save() {
  if (!form.title.trim() || !form.statement_md.trim()) {
    ElMessage.warning('标题和题面不能为空')
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
      ElMessage.success('已保存；被驳回的题目将重新进入待审核')
    } else {
      await MyProblems.create(payload)
      ElMessage.success('题目已提交审核，通过后将出现在公开题库')
    }
    router.push('/my/problems')
  } catch (e) {
    ElMessage.error(errMsg(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div v-loading="loading" class="pe-root">
    <div class="pe-header">
      <h3 style="margin: 0">
        {{ problemId ? `编辑我的题目 #${problemId}` : '新建题目（提交审核）' }}
      </h3>
      <span style="flex: 1" />
      <el-button @click="router.back()">返回</el-button>
      <el-button type="primary" :loading="saving" @click="save">保存并提交审核</el-button>
    </div>

    <div class="pe-columns">
      <el-card class="pe-edit" shadow="never">
        <el-form label-position="top">
          <el-form-item label="标题">
            <el-input v-model="form.title" placeholder="题目名称" />
          </el-form-item>
          <el-form-item label="题面（所见即所得，支持代码块/图片/链接；右侧实时预览）">
            <MarkdownEditor
              v-model="form.statement_md"
              placeholder="输入题面内容…"
              class="pe-statement-editor"
            />
          </el-form-item>
          <el-form-item label="输入描述">
            <el-input v-model="form.input_desc" type="textarea" :rows="3" />
          </el-form-item>
          <el-form-item label="输出描述">
            <el-input v-model="form.output_desc" type="textarea" :rows="3" />
          </el-form-item>
          <el-form-item label="提示（可选）">
            <el-input v-model="form.hint" type="textarea" :rows="3" />
          </el-form-item>
          <el-row :gutter="12">
            <el-col :span="8">
              <el-form-item label="时限 ms">
                <el-input-number v-model="form.time_limit_ms" :min="100" :step="100" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="内存 MB">
                <el-input-number v-model="form.mem_limit_mb" :min="16" :step="16" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item label="判题模式">
                <el-select v-model="form.judge_mode">
                  <el-option label="标准比对" value="default" />
                  <el-option label="SPJ 特判" value="spj" />
                  <el-option label="交互题" value="interactive" />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>
          <el-alert
            title="测试数据与样例将在审核通过后由管理员协助配置"
            type="info"
            :closable="false"
          />
        </el-form>
      </el-card>

      <el-card class="pe-preview" shadow="never">
        <h4 style="margin-top: 0">实时预览</h4>
        <h2 style="margin-top: 0">{{ form.title || '（无标题）' }}</h2>
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
      </el-card>
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
