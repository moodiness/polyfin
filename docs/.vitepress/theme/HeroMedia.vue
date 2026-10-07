<script setup lang="ts">
import { withBase } from 'vitepress'
import { onMounted, ref } from 'vue'
import Icon from './Icon.vue'
import { icons } from './icons'

/** The teaser's poster is its frame at 6 s, the diagram of sources, Polyfin and apps: it starts there. */
const POSTER_TIME = 6

const teaser = ref<HTMLVideoElement>()
const tour = ref<HTMLVideoElement>()
const dialog = ref<HTMLDialogElement>()
const previewing = ref(false)
let resumePreview = false

onMounted(() => {
  if (!matchMedia('(prefers-reduced-motion: reduce)').matches) playPreview()
})

function playPreview() {
  const video = teaser.value!
  if (video.currentTime === 0) video.currentTime = POSTER_TIME
  video.play().catch(() => {})
}

function togglePreview() {
  if (previewing.value) teaser.value!.pause()
  else playPreview()
}

function openTour() {
  resumePreview = previewing.value
  teaser.value!.pause()
  dialog.value!.showModal()
  tour.value!.play().catch(() => {})
}

function closeTour() {
  tour.value!.pause()
  if (resumePreview) playPreview()
}

/** A click on the backdrop reaches the dialog itself. */
function onDialogClick(event: MouseEvent) {
  if (event.target === dialog.value) dialog.value?.close()
}
</script>

<template>
  <div class="media">
    <video
      ref="teaser"
      class="teaser"
      :src="withBase('/videos/polyfin-intro.mp4')"
      :poster="withBase('/videos/polyfin-intro.jpg')"
      muted
      loop
      playsinline
      preload="metadata"
      aria-label="Preview of the Polyfin tour"
      @play="previewing = true"
      @pause="previewing = false"
    ></video>
    <div class="controls">
      <a
        class="tour-button"
        :href="withBase('/videos/polyfin-showcase.mp4')"
        @click.prevent="openTour"
      >
        <span class="play"><Icon :svg="icons.play" :size="12" /></span>
        Watch the tour
        <span class="duration">1:18</span>
      </a>
      <button
        type="button"
        class="icon-button"
        :aria-label="previewing ? 'Pause the preview' : 'Play the preview'"
        @click="togglePreview"
      >
        <Icon :svg="previewing ? icons.pause : icons.play" :size="14" />
      </button>
    </div>
    <dialog
      ref="dialog"
      class="pf-tour"
      aria-label="Polyfin tour"
      @close="closeTour"
      @click="onDialogClick"
    >
      <video
        ref="tour"
        :src="withBase('/videos/polyfin-showcase.mp4')"
        controls
        playsinline
        preload="none"
      ></video>
      <button type="button" class="icon-button close" aria-label="Close" @click="dialog?.close()">
        <Icon :svg="icons.x" :size="18" />
      </button>
    </dialog>
  </div>
</template>

<style scoped>
.media {
  position: relative;
  aspect-ratio: 16 / 9;
  border-radius: var(--nuit-radius-panel);
  overflow: hidden;
  background: var(--nuit-bg);
  box-shadow: var(--nuit-shadow-pop);
}

.teaser {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.controls {
  position: absolute;
  inset: auto 16px 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  pointer-events: none;
}

.controls > * {
  pointer-events: auto;
}

.tour-button,
.icon-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--nuit-line-3);
  border-radius: var(--nuit-radius-field);
  background: rgb(8 8 14 / 0.72);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  color: var(--nuit-ink);
  transition:
    background-color 160ms var(--nuit-ease),
    transform 160ms var(--nuit-ease);
}

.tour-button {
  gap: 10px;
  height: 44px;
  padding: 0 16px 0 10px;
  font-size: 15px;
  font-weight: 500;
  text-decoration: none;
}

.icon-button {
  width: 36px;
  height: 36px;
  cursor: pointer;
}

.tour-button:hover,
.icon-button:hover {
  background: rgb(25 25 37 / 0.88);
}

.tour-button:active,
.icon-button:active {
  transform: scale(0.98);
}

.tour-button:focus-visible,
.icon-button:focus-visible {
  outline: 2px solid var(--nuit-link);
  outline-offset: 2px;
}

.play {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--nuit-accent);
  color: #fff;
}

.duration {
  font-family: var(--vp-font-family-mono);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  color: var(--nuit-ink-2);
}

.pf-tour {
  width: min(1280px, calc(100vw - 48px), calc((100dvh - 128px) * 16 / 9));
  max-width: none;
  max-height: none;
  padding: 0;
  border: 0;
  background: transparent;
  overflow: visible;
}

.pf-tour::backdrop {
  background: rgb(4 4 10 / 0.86);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.pf-tour video {
  display: block;
  width: 100%;
  aspect-ratio: 16 / 9;
  border-radius: var(--nuit-radius-panel);
  background: var(--nuit-bg);
  box-shadow: var(--nuit-shadow-pop);
}

.close {
  position: absolute;
  top: -48px;
  right: 0;
}

@media (max-width: 639px) {
  .controls {
    inset: auto 10px 10px;
  }
  .tour-button {
    gap: 8px;
    height: 36px;
    padding: 0 12px 0 6px;
    font-size: 13px;
  }
  .play {
    width: 24px;
    height: 24px;
  }
  .duration {
    font-size: 12px;
  }
  .icon-button {
    width: 32px;
    height: 32px;
  }
}
</style>
