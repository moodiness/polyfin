<script setup lang="ts">
// A ```mermaid block of a docs page (config.mts turns each into this component). The page shows
// the diagram's text until Mermaid, loaded on the first page that has a diagram, draws it.
import { nextTick, onMounted, ref, useId } from 'vue'
import { renderDiagram } from './diagrams'

const props = defineProps<{ code: string }>()
const source = decodeURIComponent(props.code)
const id = `pf-mermaid-${useId()}`
const svg = ref('')
const box = ref<HTMLElement>()

/** The smallest text a diagram may shrink to, in CSS pixels: past it, the box scrolls sideways. */
const smallestText = 13

onMounted(async () => {
  svg.value = await renderDiagram(id, source)
  await nextTick()
  const drawing = box.value?.querySelector('svg')
  if (!drawing) return
  const natural = drawing.viewBox.baseVal.width
  const fonts = [...drawing.querySelectorAll('text, foreignObject div, foreignObject span')]
    .filter((label) => label.textContent?.trim())
    .map((label) => parseFloat(getComputedStyle(label).fontSize))
  const smallest = Math.min(...fonts)
  if (natural > 0 && smallest > 0) {
    drawing.style.minWidth = `${Math.ceil(natural * Math.min(1, smallestText / smallest))}px`
  }
})
</script>

<template>
  <div v-if="svg" ref="box" class="pf-mermaid" v-html="svg" />
  <pre v-else class="pf-mermaid pf-mermaid-source"><code>{{ source }}</code></pre>
</template>
