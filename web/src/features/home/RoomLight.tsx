import { cx } from '@/ui'
import { posterUrl } from './NowPlaying'

/**
 * The room light of Home: the lit playback's poster, blurred far and at 13 %, tinting the top of
 * the page so that the room takes a little of the colour of what is playing. Its hue is held
 * toward the room's own indigo (the glow of `body`, index.css), so that a warm poster lights the
 * room without turning it amber. Each poster fades in and out as the light moves between tiles.
 * Drawn behind the page, from the top of the document, so it must sit outside any transformed
 * block (the page's entrance).
 */
export function RoomLight({ posters, lit }: { posters: string[]; lit: string | null }) {
  if (posters.length === 0) return null
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none absolute inset-x-0 top-0 -z-1 h-[36rem] overflow-hidden [mask-image:linear-gradient(180deg,#000_0%,rgb(0_0_0/0.5)_45%,transparent_100%)] max-md:h-[28rem]"
    >
      {/* One isolated layer: the indigo takes the hue of the posters under it only. */}
      <div className="absolute inset-0 isolate opacity-13">
        {posters.map((poster) => (
          <img
            key={poster}
            src={posterUrl(poster, 64)}
            alt=""
            decoding="async"
            className={cx(
              'absolute -top-[30%] left-1/2 h-[160%] w-[120%] max-w-none -translate-x-1/2 object-cover blur-[90px] transition-opacity duration-1200 ease-nuit motion-reduce:transition-none',
              poster === lit ? 'opacity-100' : 'opacity-0',
            )}
          />
        ))}
        <div className="absolute inset-0 bg-[rgb(76_63_217)] opacity-65 mix-blend-color" />
      </div>
    </div>
  )
}
