<script setup lang="ts">
/** An icon of icons.ts, decorative: what it stands for is always in words beside it or on its button. */
defineProps<{ svg: string; size?: number }>()
</script>

<template>
  <span
    class="pf-icon"
    :style="{ '--size': `${size ?? 20}px` }"
    aria-hidden="true"
    v-html="svg"
  ></span>
</template>

<style scoped>
.pf-icon {
  display: inline-flex;
  flex: none;
}

.pf-icon :deep(svg) {
  width: var(--size);
  height: var(--size);
}
</style>
