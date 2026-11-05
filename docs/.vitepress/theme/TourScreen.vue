<script setup lang="ts">
import { withBase } from 'vitepress'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from './Icon.vue'
import { icons } from './icons'
import { heard, level } from './sound'

/**
 * The tour in one clean frame: a muted preview while the frame is on screen, and the whole tour
 * with sound and controls in the theater, which open() also opens (the hero's link).
 */
const preview = ref<HTMLVideoElement>()
const tour = ref<HTMLVideoElement>()
const theater = ref<HTMLDialogElement>()
let visible = false
let still = false
let observer: IntersectionObserver | undefined
/** The video heard before the tour opened, heard again once it closes. */
let heardBefore: string | null = null

onMounted(() => {
  still = matchMedia('(prefers-reduced-motion: reduce)').matches
  observer = new IntersectionObserver(
    ([entry]) => {
      visible = entry.isIntersecting
      sync()
    },
    { threshold: 0.35 },
  )
  observer.observe(preview.value!)
})

onBeforeUnmount(() => observer?.disconnect())

function sync() {
  const video = preview.value!
  if (visible && !still && !theater.value?.open) video.play().catch(() => {})
  else video.pause()
}

/** The tour is the only video heard while it is open, at the level set, which its controls change. */
function open() {
  preview.value!.pause()
  heardBefore = heard.value
  heard.value = 'tour'
  theater.value!.showModal()
  const video = tour.value!
  video.currentTime = 0
  video.volume = level.value
  video.muted = false
  video.play().catch(() => {})
}

function onClose() {
  tour.value!.pause()
  heard.value = heardBefore
  sync()
}

function onTourVolume() {
  const video = tour.value!
  if (!video.muted && video.volume > 0) level.value = video.volume
}

/** A click on the backdrop reaches the dialog itself. */
function onTheaterClick(event: MouseEvent) {
  if (event.target === theater.value) theater.value?.close()
}

defineExpose({ open })
</script>

<template>
  <figure class="pf-media screen">
    <span class="facet-back" aria-hidden="true"></span>
    <div class="clip">
      <video
        ref="preview"
        :src="withBase('/videos/polyfin-showcase.mp4')"
        :poster="withBase('/videos/polyfin-showcase.jpg')"
        muted
        loop
        playsinline
        preload="none"
        aria-label="The Polyfin tour, muted"
      ></video>
      <span class="facet-tab" aria-hidden="true"></span>
      <div class="bar">
        <button type="button" class="pf-button primary" @click="open">
          <span class="knob"><Icon :svg="icons.play" :size="14" /></span>
          Watch with sound
          <span class="duration">1:23</span>
        </button>
        <span class="note">Muted preview</span>
      </div>
    </div>
    <dialog
      ref="theater"
      class="pf-theater"
      aria-label="Polyfin tour"
      @close="onClose"
      @click="onTheaterClick"
    >
      <video
        ref="tour"
        :src="withBase('/videos/polyfin-showcase.mp4')"
        :poster="withBase('/videos/polyfin-showcase.jpg')"
        controls
        playsinline
        preload="none"
        @volumechange="onTourVolume"
      ></video>
      <button type="button" class="close" aria-label="Close the tour" @click="theater?.close()">
        <Icon :svg="icons.x" :size="19" />
      </button>
    </dialog>
  </figure>
</template>

<style scoped>
.screen {
  --facet: var(--pf-violet);
  --facet-back: var(--pf-cyan);
  margin: 0;
}

.bar {
  position: absolute;
  left: 22px;
  right: 22px;
  bottom: 22px;
  z-index: 3;
  display: flex;
  align-items: center;
  gap: 14px;
}

.bar .knob {
  order: -1;
  margin-left: -14px;
}

.duration {
  padding-right: 12px;
  font: 500 12.5px/1 var(--vp-font-family-mono);
  color: rgb(7 16 25 / 0.62);
}

.note {
  padding: 8px 11px;
  border-radius: 999px;
  background: rgb(10 11 22 / 0.62);
  font: 500 12px/1 var(--vp-font-family-mono);
  color: rgb(238 240 250 / 0.85);
}

.pf-theater {
  width: min(1200px, calc(100vw - 48px), calc((100dvh - 128px) * 16 / 9));
  max-width: none;
  max-height: none;
  padding: 0;
  border: 0;
  background: transparent;
  overflow: visible;
}

.pf-theater::backdrop {
  background: rgb(5 6 14 / 0.9);
}

.pf-theater video {
  display: block;
  width: 100%;
  aspect-ratio: 16 / 9;
  border-radius: 16px;
  background: #000;
}

.close {
  position: absolute;
  top: -56px;
  right: 0;
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  border-radius: 50%;
  background: var(--pf-s3);
  color: var(--pf-ink);
  cursor: pointer;
}

.close:hover {
  background: var(--pf-s4);
}

@media (max-width: 639px) {
  .bar {
    left: 12px;
    right: 12px;
    bottom: 12px;
  }

  .bar .pf-button {
    height: 40px;
    font-size: 14px;
  }

  .note {
    display: none;
  }

  .pf-theater {
    width: 100vw;
  }

  .pf-theater video {
    border-radius: 0;
  }

  .close {
    right: 12px;
  }
}
</style>
