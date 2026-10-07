<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Icon from './Icon.vue'
import { icons } from './icons'
import { heard, level, levelSettable } from './sound'

/** The sound of a video, which owner names: its button turns it on or off, its slider sets the level. */
const props = defineProps<{ owner: string }>()

const on = computed(() => heard.value === props.owner)
/** The slider shows the level while the sound is on, 0 otherwise. */
const shown = computed(() => (on.value ? Math.round(level.value * 100) : 0))
/** The slider is left out where the page cannot set a video's level. */
const slider = ref(false)

onMounted(() => {
  slider.value = levelSettable()
})

function toggle() {
  heard.value = on.value ? null : props.owner
}

/** Moving the slider turns the sound on at that level; down to 0, off, keeping the level for the next time. */
function onInput(event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  if (value > 0) {
    level.value = value / 100
    heard.value = props.owner
  } else if (on.value) {
    heard.value = null
  }
}
</script>

<template>
  <div class="sound">
    <button
      type="button"
      class="toggle"
      :aria-label="on ? 'Turn the sound off' : 'Turn the sound on'"
      @click="toggle"
    >
      <Icon :svg="on ? icons.soundOn : icons.soundOff" :size="16" />
    </button>
    <input
      v-if="slider"
      class="level"
      type="range"
      min="0"
      max="100"
      step="5"
      :value="shown"
      :style="{ '--value': `${shown}%` }"
      aria-label="Volume"
      @input="onInput"
    />
  </div>
</template>

<style scoped>
.sound {
  display: inline-flex;
  align-items: center;
  height: var(--sound-size, 36px);
  border: 1px solid var(--nuit-line-3);
  border-radius: var(--nuit-radius-field);
  background: rgb(8 8 14 / 0.72);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  color: var(--nuit-ink);
}

.toggle {
  display: grid;
  place-items: center;
  width: calc(var(--sound-size, 36px) - 2px);
  height: 100%;
  border-radius: var(--nuit-radius-field);
  color: inherit;
  cursor: pointer;
  transition: background-color 160ms var(--nuit-ease);
}

.toggle:hover {
  background: rgb(255 255 255 / 0.08);
}

.toggle:focus-visible,
.level:focus-visible {
  outline: 2px solid var(--nuit-link);
  outline-offset: 2px;
}

.level {
  -webkit-appearance: none;
  appearance: none;
  width: var(--sound-slider, 72px);
  height: 20px;
  margin: 0 12px 0 2px;
  border-radius: 4px;
  background: transparent;
  cursor: pointer;
}

.level::-webkit-slider-runnable-track {
  height: 4px;
  border-radius: 2px;
  background: linear-gradient(
    to right,
    var(--nuit-ink) var(--value),
    var(--nuit-line-3) var(--value)
  );
}

.level::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 12px;
  height: 12px;
  margin-top: -4px;
  border-radius: 50%;
  background: var(--nuit-ink);
}

.level::-moz-range-track {
  height: 4px;
  border-radius: 2px;
  background: var(--nuit-line-3);
}

.level::-moz-range-progress {
  height: 4px;
  border-radius: 2px;
  background: var(--nuit-ink);
}

.level::-moz-range-thumb {
  width: 12px;
  height: 12px;
  border: 0;
  border-radius: 50%;
  background: var(--nuit-ink);
}
</style>
