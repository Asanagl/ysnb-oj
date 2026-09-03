<script setup lang="ts">
import { Toaster } from 'vue-sonner'
import 'vue-sonner/style.css'
import { ConfirmHost } from '@/components/ui/confirm-dialog'
import { useTheme } from '@/composables/useTheme'

const { theme } = useTheme()
</script>

<template>
  <router-view />
  <Toaster position="top-center" rich-colors :theme="theme" />
  <ConfirmHost />
</template>
