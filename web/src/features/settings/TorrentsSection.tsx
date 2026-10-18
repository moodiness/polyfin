import { useState } from 'react'
import { useI18n } from '@/i18n'
import { NumberInput, SecretField, Select, TextInput } from '@/ui'
import { FieldRow, SettingRow, SettingsGroup, SwitchRow, TextRow } from './parts'
import type { SectionFormApi } from './SectionForm'

type Route = 'interface' | 'proxy'

/**
 * Settings › Torrents: playing the torrents addons list, over one route alone, a network interface
 * (Linux only) or a SOCKS5 proxy, and the limits of their traffic.
 */
export default function TorrentsSection({
  form,
  update,
  error,
  number,
  range,
  limits,
}: SectionFormApi) {
  const { t } = useI18n()
  const text = t.settings.torrents
  // Where the interface is not offered, the proxy is the only route.
  const [route, setRoute] = useState<Route>(
    form.interfaceRoutes && form.torrentInterface !== '' ? 'interface' : 'proxy',
  )
  const choose = (next: Route) => {
    setRoute(next)
    // One route alone: the other is cleared.
    update(next === 'interface' ? { torrentProxy: '' } : { torrentInterface: '' })
  }
  return (
    <SettingsGroup>
      <SwitchRow
        anchor="torrent-streams"
        label={text.play}
        help={text.playHelp}
        checked={form.torrentStreams}
        onChange={(torrentStreams) => update({ torrentStreams })}
      />
      {form.interfaceRoutes ? (
        <FieldRow
          anchor="torrent-route"
          label={text.route}
          help={text.routeHelp}
          error={error('torrent-route')}
        >
          <Select
            value={route}
            options={(['interface', 'proxy'] as const).map((value) => ({
              value,
              label: text.routes[value],
            }))}
            onValue={choose}
            className="max-w-xs"
          />
        </FieldRow>
      ) : (
        <TextRow anchor="torrent-route">{text.routeHelpProxyOnly}</TextRow>
      )}
      {route === 'interface' ? (
        <FieldRow
          anchor="torrent-interface"
          label={text.interface}
          help={text.interfaceHelp}
          error={error('torrent-interface')}
        >
          <TextInput
            value={form.torrentInterface}
            onValue={(torrentInterface) => update({ torrentInterface })}
            placeholder="wg0"
            maxLength={limits('torrentInterface').max}
            autoComplete="off"
            spellCheck={false}
            mono
            className="max-w-xs"
          />
        </FieldRow>
      ) : (
        <>
          <FieldRow
            anchor="torrent-proxy"
            label={text.proxy}
            help={text.proxyHelp}
            error={error('torrent-proxy')}
          >
            <TextInput
              value={form.torrentProxy}
              onValue={(torrentProxy) => update({ torrentProxy })}
              placeholder="10.0.0.2:1080"
              maxLength={limits('torrentProxy').max}
              autoComplete="off"
              spellCheck={false}
              mono
              className="max-w-sm"
            />
          </FieldRow>
          <FieldRow
            anchor="torrent-proxy-user"
            label={text.proxyUser}
            help={text.proxyUserHelp}
            error={error('torrent-proxy-user')}
          >
            <TextInput
              value={form.torrentProxyUser}
              onValue={(torrentProxyUser) => update({ torrentProxyUser })}
              maxLength={limits('torrentProxyUser').max}
              autoComplete="off"
              spellCheck={false}
              className="max-w-sm"
            />
          </FieldRow>
          <SettingRow
            anchor="torrent-proxy-password"
            className="[&_label]:text-[15px] [&_label]:tracking-[-0.01em]"
          >
            <SecretField
              label={text.proxyPassword}
              help={text.proxyPasswordHelp}
              saved={form.torrentProxyPasswordSet}
              value={form.torrentProxyPassword}
              onValue={(torrentProxyPassword) => update({ torrentProxyPassword })}
            />
          </SettingRow>
        </>
      )}
      <FieldRow
        anchor="torrent-download-limit"
        label={text.downloadLimit}
        help={text.downloadLimitHelp(range('torrentDownloadMbps'))}
        error={error('torrent-download-limit')}
      >
        <NumberInput
          {...limits('torrentDownloadMbps')}
          step={1}
          {...number('torrent-download-limit', form.torrentDownloadMbps, (value) =>
            update({ torrentDownloadMbps: Math.trunc(value) }),
          )}
          className="max-w-[10rem]"
        />
      </FieldRow>
      <FieldRow
        anchor="torrent-upload-limit"
        label={text.uploadLimit}
        help={text.uploadLimitHelp(range('torrentUploadMbps'))}
        error={error('torrent-upload-limit')}
      >
        <NumberInput
          {...limits('torrentUploadMbps')}
          step={1}
          {...number('torrent-upload-limit', form.torrentUploadMbps, (value) =>
            update({ torrentUploadMbps: Math.trunc(value) }),
          )}
          className="max-w-[10rem]"
        />
      </FieldRow>
    </SettingsGroup>
  )
}
