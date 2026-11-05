<script setup lang="ts">
import { withBase } from 'vitepress'

/** Install, at the end of the navigation bar, and of the phone menu (screen). */
defineProps<{ screen?: boolean }>()
</script>

<template>
  <a
    class="pf-button primary"
    :class="screen ? 'pf-screen-install' : 'pf-nav-install'"
    :href="withBase('/#install')"
    >Install</a
  >
</template>
