<script setup lang="ts">
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { confirmState, settleConfirm } from '@/lib/confirm'

function onOpenChange(open: boolean) {
  if (!open && confirmState.resolve) settleConfirm(false)
}
</script>

<template>
  <Dialog :open="confirmState.open" @update:open="onOpenChange">
    <DialogContent class="max-w-md">
      <DialogHeader>
        <DialogTitle>{{ confirmState.title }}</DialogTitle>
        <DialogDescription v-if="confirmState.description">
          {{ confirmState.description }}
        </DialogDescription>
      </DialogHeader>
      <DialogFooter>
        <Button variant="outline" @click="settleConfirm(false)">
          {{ confirmState.cancelText ?? '取消' }}
        </Button>
        <Button :variant="confirmState.danger ? 'destructive' : 'default'" @click="settleConfirm(true)">
          {{ confirmState.confirmText ?? '确定' }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
