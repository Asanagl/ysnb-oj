<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { EditorView, basicSetup } from 'codemirror'
import { cpp } from '@codemirror/lang-cpp'
import { python } from '@codemirror/lang-python'
import { java } from '@codemirror/lang-java'
import { oneDark } from '@codemirror/theme-one-dark'
import { useTheme } from '@/composables/useTheme'

const props = defineProps<{ modelValue: string; language?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const host = ref<HTMLDivElement>()
let view: EditorView | null = null

const { isDark } = useTheme()

const langExt = computed(() => {
  switch (props.language) {
    case 'python3':
      return [python()]
    case 'java':
      return [java()]
    default:
      return [cpp()]
  }
})

function build(value: string) {
  return new EditorView({
    doc: value,
    extensions: [
      basicSetup,
      ...langExt.value,
      ...(isDark.value ? [oneDark] : []),
      EditorView.updateListener.of((u) => {
        if (u.docChanged) emit('update:modelValue', u.state.doc.toString())
      }),
    ],
    parent: host.value,
  })
}

// why rebuild on language switch: the language extension is fixed at
// EditorView construction; a full swap is simpler than dynamic reconfigure.
// Same for the theme: oneDark is a construction-time extension.
watch([langExt, isDark], () => {
  if (!view) return
  const doc = view.state.doc.toString()
  view.destroy()
  view = build(doc)
})

watch(
  () => props.modelValue,
  (v) => {
    if (view && v !== view.state.doc.toString()) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } })
    }
  },
)

onMounted(() => {
  if (host.value && !view) view = build(props.modelValue)
})
</script>

<template>
  <div ref="host" class="code-editor" />
</template>

<style>
.code-editor {
  border: 1px solid var(--input);
  border-radius: 6px;
  overflow: hidden;
}
.code-editor .cm-editor {
  min-height: 320px;
  max-height: 480px;
  text-align: left;
  background: var(--card);
  color: var(--foreground);
}
/* Phones: a shorter editor keeps the submit button above the fold. */
@media (max-width: 767.98px) {
  .code-editor .cm-editor {
    min-height: 200px;
    max-height: 300px;
  }
}
</style>
