import type { HDRPath } from '@/api'
import type { Messages } from '@/i18n'

type HDRPathMessages = Messages['settings']['conversion']['hdrPath']

/**
 * Where HDR titles are converted to SDR, and why, as Settings › Conversion and System › Health
 * tell it, with the speeds in the language's numbers. With `gpuToneMapping` off, the processor
 * converts them whatever the timings.
 */
export function hdrPathText(
  hdr: HDRPath,
  m: HDRPathMessages,
  language: string,
  gpuToneMapping = true,
): { where: string; why: string } {
  if (!gpuToneMapping || hdr.reason === 'turnedOff') return { where: m.processor, why: m.turnedOff }
  const speed = (value: number) =>
    value > 0
      ? `${value.toLocaleString(language, { minimumFractionDigits: 1, maximumFractionDigits: 1 })}×`
      : '–'
  const processor = hdr.processorSpeed > 0 ? speed(hdr.processorSpeed) : null
  const where =
    hdr.path === 'processor' ? m.processor : hdr.height > 0 ? m.gpuUpTo(hdr.height) : m.gpu
  switch (hdr.reason) {
    case 'notTimed':
      return { where, why: m.notTimed }
    case 'keepsUp':
      return { where, why: m.keepsUp(speed(hdr.gpuSpeed)) }
    case 'processorFaster':
      return { where, why: m.processorFaster(speed(hdr.gpuSpeed), speed(hdr.processorSpeed)) }
    case 'lowerHeight':
      return { where, why: m.lowerHeight(speed(hdr.gpuSpeed), speed(hdr.lowSpeed), processor) }
    case 'noneKeepsUp':
      return { where, why: m.noneKeepsUp(speed(hdr.gpuSpeed), speed(hdr.lowSpeed), processor) }
  }
}
