<script setup lang="ts">
import { withBase } from 'vitepress'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watchEffect } from 'vue'
import Icon from './Icon.vue'
import SoundControl from './SoundControl.vue'
import { features } from './home'
import { icons } from './icons'
import { heard, level } from './sound'

/** From 13.4 s, each video ends on Polyfin's logo: the viewer moves to the next feature just before. */
const END = 13.2

const current = ref(0)
const video = ref<HTMLVideoElement>()
/** Whether the viewer plays when it is on screen: not with reduced motion, nor once paused. */
const autoplay = ref(false)
const playing = ref(false)
const feature = computed(() => features[current.value])
let visible = false
let progressBar: HTMLElement | undefined
let frame = 0
let observer: IntersectionObserver | undefined

onMounted(() => {
  autoplay.value = !matchMedia('(prefers-reduced-motion: reduce)').matches
  observer = new IntersectionObserver(
    ([entry]) => {
      visible = entry.isIntersecting
      sync()
    },
    { threshold: 0.5 },
  )
  observer.observe(video.value!)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  cancelAnimationFrame(frame)
})

watchEffect(() => {
  const element = video.value
  if (!element) return
  element.muted = heard.value !== 'features'
  element.volume = level.value
})

function play() {
  video.value?.play().catch((error: DOMException) => {
    // A browser that refuses to play muted videos on its own: show the poster and the play button.
    if (error.name === 'NotAllowedError') autoplay.value = false
  })
}

function sync() {
  if (autoplay.value && visible) play()
  else video.value?.pause()
}

/** Shows a feature; it plays if the viewer does. */
function show(index: number) {
  current.value = (index + features.length) % features.length
  void nextTick(sync)
}

/** A feature picked in the list plays, even with reduced motion. */
function choose(index: number) {
  autoplay.value = true
  current.value = index
  void nextTick(play)
}

function togglePlay() {
  autoplay.value = video.value!.paused
  if (autoplay.value) play()
  else video.value!.pause()
}

function setProgressBar(element: unknown) {
  if (element instanceof HTMLElement) progressBar = element
}

function tick() {
  const time = video.value!.currentTime
  if (time >= END) return show(current.value + 1)
  progressBar?.style.setProperty('--progress', String(time / END))
  frame = requestAnimationFrame(tick)
}

function onPlay() {
  playing.value = true
  cancelAnimationFrame(frame)
  frame = requestAnimationFrame(tick)
}

function onPause() {
  playing.value = false
  cancelAnimationFrame(frame)
}
</script>

<template>
  <div class="viewer">
    <ol class="list">
      <li
        v-for="(item, index) in features"
        :key="item.slug"
        :class="{ active: index === current }"
        :style="{ '--accent': item.accent }"
      >
        <button
          type="button"
          class="item"
          :aria-current="index === current ? 'true' : undefined"
          @click="choose(index)"
        >
          <span class="icon"><Icon :svg="item.icon" /></span>
          <span class="name">{{ item.name }}</span>
          <span class="pitch">{{ item.pitch }}</span>
        </button>
        <div v-if="index === current" class="detail">
          <p>{{ item.text }}</p>
          <a :href="withBase(item.docs.link)"
            >{{ item.docs.title }} <Icon :svg="icons.arrowRight" :size="14"
          /></a>
        </div>
        <span v-if="index === current" :ref="setProgressBar" class="progress"></span>
      </li>
    </ol>
    <div class="frame">
      <video
        ref="video"
        :src="withBase(`/videos/polyfin-${feature.slug}.mp4`)"
        :poster="autoplay ? undefined : withBase(`/videos/polyfin-${feature.slug}.jpg`)"
        muted
        playsinline
        preload="metadata"
        :aria-label="`${feature.name}: ${feature.pitch}`"
        @play="onPlay"
        @pause="onPause"
        @ended="show(current + 1)"
      ></video>
      <div class="controls">
        <SoundControl owner="features" />
        <button
          type="button"
          class="icon-button"
          :aria-label="playing ? 'Pause' : 'Play'"
          @click="togglePlay"
        >
          <Icon :svg="playing ? icons.pause : icons.play" :size="16" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.viewer {
  display: grid;
  grid-template-columns: minmax(0, 4fr) minmax(0, 8fr);
  gap: 48px;
  align-items: start;
}

.list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.list li {
  position: relative;
  border-top: 1px solid var(--nuit-line-2);
}

.list li:last-child {
  border-bottom: 1px solid var(--nuit-line-2);
}

.item {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr);
  column-gap: 14px;
  align-items: center;
  width: 100%;
  padding: 16px 0;
  text-align: left;
  cursor: pointer;
}

.item:focus-visible {
  outline: 2px solid var(--nuit-link);
  outline-offset: -2px;
  border-radius: var(--nuit-radius-field);
}

.icon {
  grid-row: span 2;
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  border: 1px solid color-mix(in srgb, var(--accent) 40%, transparent);
  border-radius: 10px;
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  color: var(--accent);
}

.name {
  font-size: 17px;
  font-weight: 600;
  letter-spacing: -0.01em;
  color: var(--nuit-ink-2);
  transition: color 160ms var(--nuit-ease);
}

.pitch {
  font-size: 14px;
  color: var(--nuit-ink-3);
}

.item:hover .name,
.active .name {
  color: var(--nuit-ink);
}

.detail {
  padding: 0 0 20px 54px;
  animation: rise 420ms var(--nuit-ease) both;
}

.detail p {
  margin: 0 0 10px;
  font-size: 15px;
  line-height: 1.55;
  color: var(--nuit-ink-2);
}

.detail a {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  font-weight: 500;
  color: var(--nuit-link);
  text-decoration: none;
}

.detail a:hover {
  color: var(--nuit-link-hover);
}

.progress {
  position: absolute;
  left: 0;
  right: 0;
  bottom: -1px;
  height: 2px;
  background: var(--nuit-brand);
  transform: scaleX(var(--progress, 0));
  transform-origin: left;
}

.frame {
  position: relative;
  aspect-ratio: 16 / 9;
  border-radius: var(--nuit-radius-panel);
  overflow: hidden;
  background: var(--nuit-bg);
  box-shadow: var(--nuit-shadow-pop);
}

.frame video {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.controls {
  position: absolute;
  right: 16px;
  bottom: 16px;
  display: flex;
  gap: 8px;
}

.icon-button {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border: 1px solid var(--nuit-line-3);
  border-radius: var(--nuit-radius-field);
  background: rgb(8 8 14 / 0.72);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  color: var(--nuit-ink);
  cursor: pointer;
  transition:
    background-color 160ms var(--nuit-ease),
    transform 160ms var(--nuit-ease);
}

.icon-button:hover {
  background: rgb(25 25 37 / 0.88);
}

.icon-button:active {
  transform: scale(0.98);
}

.icon-button:focus-visible {
  outline: 2px solid var(--nuit-link);
  outline-offset: 2px;
}

@keyframes rise {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
}

@media (prefers-reduced-motion: reduce) {
  .detail {
    animation: none;
  }
}

@media (max-width: 959px) {
  .viewer {
    grid-template-columns: minmax(0, 1fr);
    gap: 24px;
  }

  .frame {
    order: -1;
  }
}
</style>
