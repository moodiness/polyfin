import type en from '../en/livetv'

const livetv: typeof en = {
  livetv: {
    title: 'TV ao Vivo',
    description:
      'Os catálogos de TV vêm das suas fontes. Abra os guias de um catálogo para dar uma programação aos canais dele.',
    recordings: 'Configurações de gravação',
    settings: 'Configurações da TV ao Vivo',
    catalogsTitle: 'Catálogos de TV',
    catalogsHelp:
      'Um catálogo de TV exibido como biblioteca coloca os canais dele na TV ao Vivo dos aplicativos Jellyfin. Os guias dele dão a programação aos canais.',
    catalogsLabel: 'Catálogos de TV',
    catalogsLoading: 'Carregando catálogos de TV…',
    catalogsEmptyTitle: 'Nenhum catálogo de TV ainda',
    catalogsEmpty:
      'Instale um complemento que tenha um catálogo de TV, ou adicione uma fonte IPTV, em Fontes: os canais dele são listados aqui.',
    addSource: 'Adicionar uma fonte',
    kinds: { stremio: 'Complemento Stremio', iptv: 'Fonte IPTV' },
    guides: (count: number) => (count === 1 ? '1 guia' : `${count} guias`),
    guideNone: 'Nenhum guia de programação',
    failing: (count: number) =>
      count === 1
        ? 'O último download de 1 guia falhou.'
        : `O último download de ${count} guias falhou.`,
    mapped: (mapped: string, channels: string) => `${mapped} de ${channels} canais com guia`,
    guideLine: (position: number, message: string) => `Guia ${position}: ${message}`,
    manage: 'Guias e mapeamento',
    lineup: 'Lista de canais e guias',
    lineupLabel: (name: string) => `Lista de canais e guias de ${name}`,
    manageLabel: (name: string) => `Guias e mapeamento de ${name}`,
    notShownHelp:
      'Exiba-o como biblioteca para colocar os canais dele na TV ao Vivo e dar guias a ele.',
    openLibraries: 'Abrir bibliotecas',
    states: {
      notShown: 'Fora da TV ao Vivo',
      addonOff: 'Complemento desativado',
      missing: 'Não está mais disponível',
      failed: 'Falha no download do guia',
      noGuide: 'Sem guia',
      notFetched: 'Ainda não obtido',
      incomplete: 'Guia incompleto',
      complete: 'Guia completo',
    },
    iptvTitle: 'Canais IPTV',
    iptvHelp: 'Os canais ao vivo de cada fonte IPTV, conforme a lista de canais dela os mantém.',
    iptvLoading: 'Carregando fontes IPTV…',
    iptvEmptyTitle: 'Nenhuma fonte IPTV ainda',
    iptvEmpty:
      'Adicione uma lista de reprodução M3U ou uma conta Xtream Codes em Fontes: os canais dela são contados aqui.',
    kindM3u: 'Lista de reprodução M3U',
    kindXtream: 'Conta Xtream Codes',
    kindTuner: 'Sintonizador HDHomeRun',
    turnedOff: 'Desativada',
    lastDownload: 'Último download',
    nextDownload: 'próximo',
    never: 'nunca',
    figuresLabel: (name: string) => `Canais de ${name}`,
    figures: {
      entries: 'Entradas ao vivo',
      enabled: 'Canais ativados',
      shown: 'Exibidos nos aplicativos',
      mapped: 'Com guia',
      unmapped: 'Sem guia',
    },
    noLiveTv: 'Esta fonte não importa nenhum canal ao vivo.',
    importOptions: 'Opções de importação',
    channels: 'Canais',
    channelsLabel: (name: string) => `Canais de ${name}`,
    openMapping: 'Abrir o mapeamento de guias',
    recordingsTitle: 'Gravações',
    recordingsLoading: 'Carregando gravações…',
    recordingsOffTitle: 'A gravação está desativada',
    recordingsOff:
      'Ative-a e escolha as margens e por quanto tempo as gravações são mantidas em Configurações › Gravações.',
    noTimersTitle: 'Nenhuma gravação agendada',
    noTimers:
      'As gravações são agendadas a partir do guia de programação nos aplicativos Jellyfin. Elas aparecem aqui até terminarem.',
    recordingNow: 'Gravando',
    scheduled: 'Agendada',
    failed: 'Falhou',
    series: 'Série',
    scheduledBy: (name: string) => `Agendada por ${name}`,
    unknownChannel: 'Canal não encontrado',
    tunersShort: 'Sintonizadores insuficientes',
    tunersShortHelp:
      'Mais gravações nos canais deste sintonizador do que ele tem sintonizadores acontecem ao mesmo tempo: uma delas pode falhar.',
    guidesOf: (name: string) => `Guias de ${name}`,
    spaceSaved: (count: number, saved: string) =>
      count === 1
        ? `1 gravação convertida para HEVC: ${saved} economizados.`
        : `${count} gravações convertidas para HEVC: ${saved} economizados.`,
  },
}

export default livetv
