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
  height: 40px;
  border-radius: 999px;
  background: rgb(10 11 22 / 0.62);
  color: var(--pf-ink);
}

.toggle {
  display: grid;
  place-items: center;
  width: 40px;
  height: 100%;
  border-radius: 50%;
  color: inherit;
  cursor: pointer;
  transition: background-color 0.3s var(--pf-ease);
}

.toggle:hover {
  background: rgb(10 11 22 / 0.85);
}

.level {
  -webkit-appearance: none;
  appearance: none;
  width: 64px;
  height: 20px;
  margin: 0 14px 0 0;
  border-radius: 4px;
  background: transparent;
  cursor: pointer;
}

.level::-webkit-slider-runnable-track {
  height: 3px;
  border-radius: 2px;
  background: linear-gradient(to right, var(--pf-ink) var(--value), var(--pf-line-3) var(--value));
}

.level::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 12px;
  height: 12px;
  margin-top: -4.5px;
  border-radius: 50%;
  background: var(--pf-ink);
}

.level::-moz-range-track {
  height: 3px;
  border-radius: 2px;
  background: var(--pf-line-3);
}

.level::-moz-range-progress {
  height: 3px;
  border-radius: 2px;
  background: var(--pf-ink);
}

.level::-moz-range-thumb {
  width: 12px;
  height: 12px;
  border: 0;
  border-radius: 50%;
  background: var(--pf-ink);
}

/* Phones have their own buttons for the level. */
@media (max-width: 639px) {
  .level {
    display: none;
  }
}
</style>
