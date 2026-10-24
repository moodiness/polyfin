import type en from '../en/libraries'

const libraries: typeof en = {
  libraries: {
    title: 'Bibliotecas',
    description:
      'Escolha quais catálogos dos complementos do servidor aparecem como bibliotecas nos aplicativos Jellyfin, para todos os usuários.',
    shownTitle: 'Exibidas nos aplicativos Jellyfin',
    shownHelp:
      'As bibliotecas aparecem nesta ordem. Deixe um nome vazio para usar o nome do catálogo. Bibliotecas com o mesmo nome recebem o tipo nos aplicativos, por exemplo “Populares (Filmes)”.',
    shownHelpMine:
      'As bibliotecas aparecem nesta ordem, depois das bibliotecas do servidor, se você as usar. Deixe um nome vazio para usar o nome do catálogo. Bibliotecas com o mesmo nome recebem o tipo nos aplicativos, por exemplo “Populares (Filmes)”.',
    defaultHelp:
      'Quando um complemento é instalado, seus catálogos de coleções se tornam bibliotecas: cada uma exibe suas coleções (por exemplo, gêneros ou décadas), que abrem seus filmes e séries. Um complemento sem coleções tem seus catálogos de filmes, séries e TV transformados em bibliotecas, até 20. Isso é apenas um ponto de partida: ative quantos catálogos quiser. Cada biblioteca adiciona uma linha à tela inicial dos aplicativos Jellyfin, exceto os catálogos de TV, cujos canais aparecem na TV ao Vivo.',
    manyWarning:
      'Mais de 20 bibliotecas podem deixar a tela inicial dos aplicativos Jellyfin pesada.',
    noAddons: 'Adicione uma fonte primeiro: os catálogos dela serão listados aqui.',
    shownEmpty: 'Nenhuma biblioteca: os aplicativos Jellyfin não exibem nada desses complementos.',
    name: 'Nome nos aplicativos',
    appName: (name: string) => `Exibida nos aplicativos como “${name}”.`,
    liveTv: 'Os canais aparecem na TV ao Vivo dos aplicativos Jellyfin.',
    iptvVod: 'De uma fonte IPTV.',
    iptvVodLink: 'Opções de importação',
    addonOff: 'Complemento desativado',
    missing: 'Não está mais disponível',
    hideInMenus: 'Ocultar da barra superior do player web',
    hideInMenusHelp:
      'Ela mantém sua linha na tela inicial. O menu lateral do player web também a omite; outros aplicativos Jellyfin continuam a listá-la. Aplica-se na próxima vez que o player web for carregado.',
    hiddenInMenus: 'Oculta da barra superior',
    remove: 'Remover',
    removeLabel: (name: string) => `Remover ${name}`,
    moreActions: (name: string) => `Mais ações para ${name}`,
    refresh: {
      start: 'Atualizar as descrições',
      stop: 'Parar a atualização',
      queued: 'Aguardando para atualizar as descrições',
      running: (done: string, titles: string) => `Atualizando as descrições: ${done} de ${titles}`,
      done: (done: string, failed: number, figure: string) =>
        failed === 0
          ? `Descrições atualizadas: ${done}`
          : `Descrições atualizadas: ${done}, ${figure} sem descrição`,
      stopped: (done: string, titles: string) => `Atualização parada em ${done} de ${titles}`,
      failed: 'Não foi possível atualizar as descrições',
      started: (name: string) => `${name}: as descrições são lidas de novo, 30 por minuto.`,
      progress: (name: string) => `Atualização de ${name}`,
    },
    removedLive: (name: string) => `${name} removido das bibliotecas.`,
    dragHandle: (name: string) => `Arrastar ${name}`,
    availableTitle: 'Catálogos disponíveis',
    availableHelp: 'Adicionar um catálogo o exibe como biblioteca, no final da lista acima.',
    filter: 'Filtrar',
    filterPlaceholder: 'Nome do catálogo ou do complemento',
    type: 'Tipo',
    allTypes: 'Todos os tipos',
    add: 'Adicionar',
    addLabel: (name: string) => `Adicionar ${name}`,
    addedLive: (name: string, position: number) =>
      `${name} adicionado como biblioteca ${position}.`,
    notBrowsable: 'Indisponível',
    notBrowsableHelp: 'Requer uma pesquisa ou outro valor, por isso não pode ser uma biblioteca.',
    availableEmpty: 'Todos os catálogos já estão exibidos.',
    noMatch: 'Nenhum catálogo corresponde a este filtro.',
    reset: 'Redefinir',
    saved: 'Bibliotecas salvas.',
    guideAfterSave: 'Salve as bibliotecas para adicionar um guia de programação a este catálogo.',
    guideFetched: 'Última obtenção',
    guideNever: 'Ainda não obtido.',
    loading: 'Carregando bibliotecas…',
    listLabel: 'Bibliotecas, na ordem em que os aplicativos as exibem',
    columnCatalog: 'Catálogo',
    shownEmptyTitle: 'Nenhuma biblioteca ainda',
    shownEmptyHow: 'Adicione catálogos da lista abaixo: cada um se torna uma biblioteca.',
    noAddonsTitle: 'Nenhum catálogo ainda',
    addSource: 'Adicionar uma fonte',
    defaultTitle: 'Como as primeiras bibliotecas são escolhidas',
    availableLabel: (addon: string) => `Catálogos de ${addon}`,
    music: {
      library: {
        music: 'Biblioteca de músicas',
        audiobook: 'Biblioteca de livros',
        podcast: 'Biblioteca de músicas',
      } as Record<string, string>,
      help: {
        music: 'Os itens aparecem na seção de músicas dos aplicativos Jellyfin.',
        audiobook: 'Os audiolivros aparecem em uma biblioteca de livros dos aplicativos Jellyfin.',
        podcast: 'Os episódios aparecem na seção de músicas dos aplicativos Jellyfin.',
      } as Record<string, string>,
    },
    image: {
      label: 'Imagem nos aplicativos',
      edit: (name: string, state: string) => `Imagem de ${name}: ${state}`,
      choices: {
        none: 'Nenhuma',
        automatic: 'Automática',
        generated: 'Gerada',
        custom: 'Personalizada',
      },
      help: {
        none: 'Os aplicativos exibem esta biblioteca sem imagem, como antes.',
        automatic:
          'O Polyfin a obtém do catálogo: a imagem de fundo de um dos primeiros títulos, ou então um pôster.',
        generated:
          'O Polyfin a cria com os pôsteres dos primeiros títulos do catálogo, ou das primeiras coleções de uma biblioteca de coleções, junto com o nome da biblioteca, depois de lê-los.',
        custom:
          'Uma imagem que você envia, ou que o Polyfin baixa uma vez de um endereço. Imagens largas em 16:9 se encaixam melhor nos blocos das bibliotecas. JPEG, PNG ou WebP, no máximo 10 MB.',
      },
      parental: 'Usuários sob controle parental ou que bloqueiam gêneros não a veem.',
      notFound: {
        automatic:
          'Nada encontrado neste catálogo por enquanto, então os aplicativos não exibem imagem.',
        generated:
          'Nenhum pôster lido deste catálogo ainda, então os aplicativos não exibem imagem. O Polyfin a cria quando a biblioteca for aberta em um aplicativo.',
      },
      noImage: 'Sem imagem',
      upload: 'Enviar uma imagem',
      uploadAnother: 'Enviar outra imagem',
      address: 'Ou um endereço de imagem',
      addressHelp: 'O Polyfin a baixa uma vez e guarda uma cópia.',
      useAddress: 'Usar este endereço',
      remove: 'Remover a imagem',
      removeTitle: (name: string) => `Remover a imagem de ${name}?`,
      removeToNone:
        'A imagem enviada será excluída. Os aplicativos exibirão esta biblioteca sem imagem.',
      removeToAutomatic:
        'A imagem enviada será excluída. Os aplicativos exibirão a imagem encontrada no catálogo.',
      removeToGenerated:
        'A imagem enviada será excluída. Os aplicativos exibirão a imagem criada pelo Polyfin.',
      removeConfirm: 'Remover',
      saved: 'Imagem salva.',
      removed: 'Imagem removida.',
      atOnce: 'As alterações de imagem são salvas imediatamente.',
      afterSave: 'Salve as bibliotecas primeiro e depois escolha a imagem desta biblioteca.',
      close: (name: string) => `Fechar a imagem de ${name}`,
    },
    narrow: {
      title: 'Gênero e máximo',
      edit: (name: string) => `Gênero e máximo de ${name}`,
      close: (name: string) => `Fechar o gênero e o máximo de ${name}`,
      staged: 'Salvos com o botão Salvar, assim como os nomes.',
      genre: 'Gênero',
      allGenres: 'Todos os gêneros',
      genreHelp: 'Exibe apenas os títulos deste gênero.',
      maxItems: 'Máximo de títulos',
      noMax: 'Sem máximo',
      maxHelp: (max: string, where: string) =>
        `Exibe no máximo esta quantidade de títulos na biblioteca, e a mesma quantidade em cada uma de suas coleções, no lugar do limite do catálogo em ${where}. De 1 a ${max}. Vazio: esse limite se aplica.`,
      maxHelpMine: (max: string) =>
        `Exibe no máximo esta quantidade de títulos na biblioteca, e a mesma quantidade em cada uma de suas coleções, no lugar do limite do servidor. De 1 a ${max}. Vazio: o limite do servidor se aplica.`,
      maxRange: (max: string) => `Digite um número inteiro de 1 a ${max}, ou deixe vazio.`,
      maxBadge: (count: string) => `máx. ${count}`,
    },
  },
  smartLists: {
    title: 'Listas inteligentes',
    help: 'Listas criadas a partir de regras sobre os títulos dos catálogos do servidor, como “Filmes franceses dos anos 1990 ainda não assistidos”. Todos os usuários as veem, dentro das bibliotecas que podem ver.',
    helpMine:
      'Listas criadas a partir de regras sobre os títulos dos seus catálogos, e dos do servidor enquanto você os usa. Só você as vê.',
    add: 'Adicionar lista inteligente',
    emptyTitle: 'Nenhuma lista inteligente ainda',
    emptyHow:
      'Uma lista inteligente mantém os títulos dos catálogos escolhidos que atendem às suas regras e acompanha esses catálogos conforme eles mudam.',
    loading: 'Carregando listas inteligentes…',
    shownAsLibrary: 'Biblioteca',
    shownAsCollection: 'Coleção',
    catalogCount: (count: number) => (count === 1 ? '1 catálogo' : `${count} catálogos`),
    ruleCount: (count: number) => (count === 1 ? '1 regra' : `${count} regras`),
    maxBadge: (count: string) => `${count} no máx.`,
    newTitle: 'Nova lista inteligente',
    back: 'Bibliotecas',
    backMine: 'Minhas fontes',
    lede: 'Escolha os catálogos de onde ela tira os títulos e as regras que eles devem atender. A prévia mostra os títulos como estão agora, antes de salvar.',
    notFound: 'Esta lista inteligente não existe.',
    name: 'Nome',
    nameHelp: 'O nome exibido pelos aplicativos Jellyfin.',
    shownAs: 'Exibida como',
    shownAsChoices: { library: 'Uma biblioteca', collection: 'Uma coleção' },
    shownAsHelp: {
      library:
        'Uma biblioteca própria nos aplicativos Jellyfin, com sua fileira na tela inicial: uma biblioteca de filmes ou de séries quando todos os seus catálogos são de um mesmo tipo.',
      collection:
        'Uma coleção na visualização Coleções, ao lado das criadas pelos usuários. Os aplicativos não podem adicionar nem remover títulos dela.',
    },
    catalogs: 'Catálogos',
    catalogsHelp:
      'Os títulos vêm destes catálogos, cada um lido até o seu máximo: o da própria biblioteca quando o catálogo é uma das suas bibliotecas, senão Títulos lidos por catálogo de filmes e séries.',
    catalogFilter: 'Filtrar catálogos',
    catalogFilterPlaceholder: 'Nome do catálogo ou do complemento',
    serverBadge: 'Servidor',
    noCatalogs: 'Nenhum catálogo disponível ainda: adicione uma fonte primeiro.',
    noCatalogMatch: 'Nenhum catálogo corresponde a este filtro.',
    rules: 'Regras',
    rulesHelp: 'Sem regras, a lista mantém todos os títulos dos seus catálogos.',
    match: 'Os títulos atendem a',
    matchChoices: { all: 'Todas as regras', any: 'Qualquer uma das regras' },
    groupMatch: 'Este grupo atende a',
    group: 'Grupo',
    addRule: 'Adicionar regra',
    addGroup: 'Adicionar grupo',
    removeRule: 'Remover esta regra',
    removeGroup: 'Remover este grupo',
    field: 'Analisa',
    fields: {
      genre: 'Gênero',
      year: 'Ano',
      rating: 'Avaliação IMDb',
      language: 'Idioma',
      type: 'Filme ou série',
      rank: 'Posição no catálogo',
      state: 'Estado de reprodução',
    } as Record<string, string>,
    negation: 'Corresponde ou não',
    is: 'é',
    isNot: 'não é',
    genre: 'Gênero',
    from: 'De',
    to: 'Até',
    language: 'Idioma',
    rankMax: 'Entre os primeiros',
    types: { movie: 'Um filme', series: 'Uma série' } as Record<string, string>,
    states: {
      played: 'Reproduzido',
      unplayed: 'Não reproduzido',
      started: 'Iniciado, não concluído',
      favorite: 'Favorito',
    } as Record<string, string>,
    stateHelp:
      'As regras de reproduzido, iniciado e favorito seguem o que cada usuário fez: cada um vê a sua própria lista.',
    languageHelp:
      'O idioma vem com a descrição de um título, que o Polyfin lê em segundo plano: um título ainda não descrito fica de fora até ser descrito.',
    sort: 'Ordenação',
    sorts: {
      catalog: 'Como os catálogos os listam, um de cada por vez',
      premiere: 'Data de estreia, mais recentes primeiro',
      rating: 'Avaliação IMDb, maiores primeiro',
      name: 'Nome',
      random: 'Aleatória, sorteada novamente a cada dia',
    } as Record<string, string>,
    maxItems: 'Máximo de títulos',
    maxHelp: (max: string) => `De 1 a ${max}. Vazio: sem máximo.`,
    noMax: 'Sem máximo',
    create: 'Criar lista',
    save: 'Salvar',
    created: 'Lista inteligente criada.',
    saved: 'Lista inteligente salva.',
    delete: 'Excluir',
    deleteTitle: (name: string) => `Excluir ${name}?`,
    deleteBody:
      'Os aplicativos Jellyfin deixam de exibi-la. Os títulos dela continuam nos catálogos.',
    deleted: 'Lista inteligente excluída.',
    preview: 'Pré-visualização',
    previewHelp:
      'Os títulos que a lista mostra para você agora. Cada usuário a vê com os próprios títulos reproduzidos.',
    refresh: 'Atualizar',
    listed: (count: string) => (count === '1' ? '1 título' : `${count} títulos`),
    firstOf: (shown: string, listed: string) => `Os primeiros ${shown} de ${listed}.`,
    unknown: (count: string) =>
      count === '1'
        ? '1 título ainda aguarda a descrição, que informa o idioma: a lista cresce à medida que as descrições chegam.'
        : `${count} títulos ainda aguardam a descrição, que informa o idioma: a lista cresce à medida que as descrições chegam.`,
    incomplete:
      'Alguns catálogos não responderam a tempo: a lista mostra o que foi lido até agora e lê o restante.',
    previewEmpty: 'Nenhum título corresponde a estas regras por enquanto.',
    previewNeedsCatalog: 'Escolha pelo menos um catálogo para ver a lista.',
    previewLabel: 'Títulos da lista',
    columns: {
      title: 'Título',
      type: 'Tipo',
      year: 'Ano',
      rating: 'IMDb',
      genres: 'Gêneros',
      languages: 'Idioma',
    },
  },
}

export default libraries
