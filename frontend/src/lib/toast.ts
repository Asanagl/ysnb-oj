// toast 是 vue-sonner 的轻封装。
// 注意：vue-sonner 的 <Toaster />（rich-colors、position="top-center"）须由主代理挂到 App.vue，
// 否则 toast 不会显示。
import { toast as sonner } from 'vue-sonner'

export const toast = {
  success: (msg: string) => sonner.success(msg),
  error: (msg: string) => sonner.error(msg),
  warning: (msg: string) => sonner.warning(msg),
  info: (msg: string) => sonner.info(msg),
}
