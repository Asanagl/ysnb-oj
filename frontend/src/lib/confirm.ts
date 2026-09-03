// confirmDialog 是 Promise 式确认框：await confirmDialog({ title, ... }) → boolean。
// 注意：ConfirmHost（components/ui/confirm-dialog/ConfirmHost.vue）必须挂到 App.vue，
// 由主代理完成挂载，否则对话框不会渲染、Promise 不会 resolve。
import { reactive } from 'vue'

export interface ConfirmOptions {
  title: string
  description?: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
}

interface ConfirmState {
  open: boolean
  title: string
  description?: string
  confirmText?: string
  cancelText?: string
  danger: boolean
  resolve: ((value: boolean) => void) | null
}

export const confirmState = reactive<ConfirmState>({
  open: false,
  title: '',
  danger: false,
  resolve: null,
})

export function confirmDialog(opts: ConfirmOptions): Promise<boolean> {
  confirmState.resolve?.(false)
  return new Promise<boolean>((resolve) => {
    confirmState.title = opts.title
    confirmState.description = opts.description
    confirmState.confirmText = opts.confirmText
    confirmState.cancelText = opts.cancelText
    confirmState.danger = opts.danger ?? false
    confirmState.resolve = resolve
    confirmState.open = true
  })
}

export function settleConfirm(value: boolean) {
  const resolve = confirmState.resolve
  confirmState.resolve = null
  confirmState.open = false
  resolve?.(value)
}
