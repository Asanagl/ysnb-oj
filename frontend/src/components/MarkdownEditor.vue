<script setup lang="ts">
import { onBeforeUnmount, onMounted, shallowRef, watch } from 'vue'
import { Editor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Link from '@tiptap/extension-link'
import Image from '@tiptap/extension-image'

// 富文本所见即所得 editor bound to a model string (HTML). Toolbar commands
// live in script scope (not template scope) so window/prompt resolve cleanly.
const props = defineProps<{ modelValue: string; placeholder?: string }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()

const editorRef = shallowRef<Editor | undefined>(undefined)

onMounted(() => {
  editorRef.value = new Editor({
    content: props.modelValue || '<p></p>',
    extensions: [
      StarterKit.configure({ heading: { levels: [1, 2, 3] } }),
      Link.configure({ openOnClick: false, autolink: true }),
      Image.configure({ inline: false }),
    ],
    editorProps: {
      attributes: {
        class: 'wysiwyg-content',
        'data-placeholder': props.placeholder ?? '输入题解内容…',
      },
    },
    onUpdate: ({ editor: e }) => {
      emit('update:modelValue', e.getHTML())
    },
  })
})

watch(
  () => props.modelValue,
  (v) => {
    if (editorRef.value && v !== editorRef.value.getHTML()) {
      editorRef.value.commands.setContent(v || '<p></p>')
    }
  },
)

onBeforeUnmount(() => editorRef.value?.destroy())

function cmd(fn: (e: Editor) => void) {
  if (editorRef.value) fn(editorRef.value)
}

function setLink() {
  if (!editorRef.value) return
  const url = window.prompt('链接地址：')
  if (url) cmd((e) => e.chain().focus().setLink({ href: url }).run())
}

function setImage() {
  if (!editorRef.value) return
  const url = window.prompt('图片地址：')
  if (url) cmd((e) => e.chain().focus().setImage({ src: url }).run())
}

const buttons = [
  { label: 'B', title: '加粗', active: () => editorRef.value?.isActive('bold') ?? false, run: () => cmd((e) => e.chain().focus().toggleBold().run()) },
  { label: 'I', title: '斜体', active: () => editorRef.value?.isActive('italic') ?? false, run: () => cmd((e) => e.chain().focus().toggleItalic().run()) },
  { label: 'H', title: '标题', active: () => editorRef.value?.isActive('heading') ?? false, run: () => cmd((e) => e.chain().focus().toggleHeading({ level: 2 }).run()) },
  { label: '“”', title: '引用', active: () => editorRef.value?.isActive('blockquote') ?? false, run: () => cmd((e) => e.chain().focus().toggleBlockquote().run()) },
  { label: '• 列表', title: '无序列表', active: () => editorRef.value?.isActive('bulletList') ?? false, run: () => cmd((e) => e.chain().focus().toggleBulletList().run()) },
  { label: '1. 列表', title: '有序列表', active: () => editorRef.value?.isActive('orderedList') ?? false, run: () => cmd((e) => e.chain().focus().toggleOrderedList().run()) },
  { label: '</>', title: '代码块', active: () => editorRef.value?.isActive('codeBlock') ?? false, run: () => cmd((e) => e.chain().focus().toggleCodeBlock().run()) },
  { label: '—', title: '分割线', active: () => false, run: () => cmd((e) => e.chain().focus().setHorizontalRule().run()) },
]
</script>

<template>
  <div class="wysiwyg">
    <div class="wysiwyg-toolbar">
      <button
        v-for="b in buttons"
        :key="b.title"
        type="button"
        class="wys-btn"
        :class="{ on: b.active() }"
        :title="b.title"
        @click.prevent="b.run"
      >
        {{ b.label }}
      </button>
      <button type="button" class="wys-btn" title="链接" @click.prevent="setLink">🔗</button>
      <button type="button" class="wys-btn" title="图片" @click.prevent="setImage">🖼</button>
      <button type="button" class="wys-btn" title="撤销" @click.prevent="cmd((e) => e.chain().focus().undo().run())">↶</button>
      <button type="button" class="wys-btn" title="重做" @click.prevent="cmd((e) => e.chain().focus().redo().run())">↷</button>
    </div>
    <editor-content v-if="editorRef" :editor="editorRef" class="wysiwyg-body" />
  </div>
</template>

<style scoped>
.wysiwyg {
  border: 1px solid var(--input);
  border-radius: 8px;
  overflow: hidden;
  background: var(--card);
}
.wysiwyg-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 6px;
  background: var(--muted);
  border-bottom: 1px solid var(--border);
}
.wys-btn {
  border: 1px solid transparent;
  background: transparent;
  border-radius: 4px;
  padding: 4px 10px;
  cursor: pointer;
  font-size: 13px;
  color: var(--muted-foreground);
}
.wys-btn:hover {
  background: var(--accent-soft);
  color: var(--primary);
}
.wys-btn.on {
  background: var(--primary);
  color: var(--primary-foreground);
}
.wysiwyg-body :deep(.wysiwyg-content) {
  min-height: 220px;
  padding: 12px 16px;
  outline: none;
  font-size: 14px;
  line-height: 1.7;
  color: var(--foreground);
}
.wysiwyg-body :deep(.wysiwyg-content p.is-editor-empty:first-child)::before {
  content: attr(data-placeholder);
  color: var(--muted-foreground);
  float: left;
  height: 0;
  pointer-events: none;
}
.wysiwyg-body :deep(.wysiwyg-content pre) {
  background: #282c34;
  color: #abb2bf;
  padding: 10px;
  border-radius: 4px;
}
.wysiwyg-body :deep(.wysiwyg-content img) {
  max-width: 100%;
}
.wysiwyg-body :deep(.wysiwyg-content blockquote) {
  border-left: 3px solid var(--primary);
  padding-left: 12px;
  color: var(--muted-foreground);
  margin-left: 0;
}
</style>
