<script setup lang="ts">
import { withBase } from 'vitepress'
import { onMounted, ref, watchEffect } from 'vue'
import Icon from './Icon.vue'
import SoundControl from './SoundControl.vue'
import { icons } from './icons'
import { heard, level } from './sound'

/** The ad's poster is its frame at 3.7 s, Polyfin reaching every source and app: it starts there. */
const POSTER_TIME = 3.7

const teaser = ref<HTMLVideoElement>()
const tour = ref<HTMLVideoElement>()
const dialog = ref<HTMLDialogElement>()
const previewing = ref(false)
let resumePreview = false
/** The video heard before the tour opened, heard again once it closes. */
let heardBefore: string | null = null

onMounted(() => {
  if (!matchMedia('(prefers-reduced-motion: reduce)').matches) playPreview()
})

watchEffect(() => {
  const video = teaser.value
  if (!video) return
  video.muted = heard.value !== 'intro'
  video.volume = level.value
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

/** The tour is the only video heard while it is open, at the level set, which its own controls change. */
function openTour() {
  resumePreview = previewing.value
  teaser.value!.pause()
  heardBefore = heard.value
  heard.value = 'tour'
  dialog.value!.showModal()
  tour.value!.volume = level.value
  tour.value!.play().catch(() => {})
}

function closeTour() {
  tour.value!.pause()
  heard.value = heardBefore
  if (resumePreview) playPreview()
}

function onTourVolume() {
  const video = tour.value!
  if (!video.muted && video.volume > 0) level.value = video.volume
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
      :src="withBase('/videos/polyfin-ad.mp4')"
      :poster="withBase('/videos/polyfin-ad.jpg')"
      muted
      loop
      playsinline
      preload="metadata"
      aria-label="Polyfin in 30 seconds"
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
      <div class="buttons">
        <SoundControl owner="intro" />
        <button
          type="button"
          class="icon-button"
          :aria-label="previewing ? 'Pause the preview' : 'Play the preview'"
          @click="togglePreview"
        >
          <Icon :svg="previewing ? icons.pause : icons.play" :size="14" />
        </button>
      </div>
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
        @volumechange="onTourVolume"
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

.buttons {
  display: flex;
  gap: 8px;
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
  white-space: nowrap;
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
    --sound-size: 32px;
    --sound-slider: 56px;
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

/* The tour's button, the sound's slider and play take 360 px: narrower, the slider is left out, phones having their own buttons for it. */
@media (max-width: 419px) {
  .controls :deep(.level) {
    display: none;
  }
}
</style>
