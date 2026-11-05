<script setup lang="ts">
/** An icon of icons.ts, decorative: what it stands for is always in words beside it or on its button. */
const props = defineProps<{ svg: string; size?: number | string }>()
const length = typeof props.size === 'string' ? props.size : `${props.size ?? 20}px`
</script>

<template>
  <span class="pf-icon" :style="{ '--size': length }" aria-hidden="true" v-html="svg"></span>
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
