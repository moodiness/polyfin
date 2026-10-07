import { ref } from 'vue'

/**
 * The sound of the site's videos: one level for all of them, and one heard
 * at a time. Turning a video's sound on turns the others' off.
 */

/** The level the videos are heard at, from 0 to 1. */
export const level = ref(1)

/** The owner of the video heard (see SoundControl), or null for none. */
export const heard = ref<string | null>(null)

/**
 * Whether the page may set a video's level. iOS plays every video at the
 * device's volume: a level set there reads 1 again.
 */
export function levelSettable(): boolean {
  const probe = document.createElement('video')
  probe.volume = 0.5
  return probe.volume === 0.5
}
