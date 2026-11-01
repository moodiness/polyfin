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
    skipLocal: 'Omitir os títulos que as pastas locais têm',
    skipLocalHelp:
      'Um filme ou série que a biblioteca de uma pasta local lista, vinculado ao mesmo identificador, aparece só lá. Suas versões e o estado de assistido continuam os mesmos, e a busca o encontra uma vez. Um usuário que não vê a biblioteca da pasta continua a encontrá-lo aqui.',
    skippedLocal: 'Sem os títulos das pastas locais',
    showAsTitles: 'Mostrar como filmes e séries',
    showAsTitlesHelp:
      'Para os aplicativos Jellyfin que listam só filmes e séries: um título com vários vídeos, ou com um numerado como episódio, vira uma série, seus vídeos seus episódios; um título sem vídeos, ou com um só arquivo, um filme. As marcações de assistido, os pontos de retomada e os favoritos os acompanham ao salvar.',
    shownAsTitles: 'Mostrada como filmes e séries',
    remove: 'Remover',
    removeLabel: (name: string) => `Remover ${name}`,
    moreActions: (name: string) => `Mais ações para ${name}`,
    userOrders: {
      help: 'Um usuário que ordenou as bibliotecas em Configurações › Início do player web mantém essa ordem, até “Aplicar esta ordem a todos os usuários” ou o seu próprio “Redefinir minhas preferências”.',
      apply: 'Aplicar esta ordem a todos os usuários',
      saveFirst: 'Salve a lista primeiro: a ordem aplicada é a salva.',
      title: 'Aplicar esta ordem a todos os usuários?',
      counting: 'Contando os usuários com uma ordem própria…',
      none: 'Todos os usuários já seguem esta ordem: nenhum ordenou as bibliotecas em Configurações › Início do player web.',
      some: (count: number) =>
        `${count === 1 ? '1 usuário ordenou' : `${count} usuários ordenaram`} as bibliotecas em Configurações › Início do player web. A ordem deles é esquecida, e os aplicativos deles listam as bibliotecas nesta ordem, a salva; o player web, em até um minuto. As outras configurações deles continuam.`,
      applied: (count: number) =>
        count === 0
          ? 'Todos os usuários seguem esta ordem.'
          : count === 1
            ? 'Esta ordem agora vale para todos: a ordem própria de 1 usuário foi esquecida.'
            : `Esta ordem agora vale para todos: a ordem própria de ${count} usuários foi esquecida.`,
    },
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
    exclude: 'Exclusões',
    excludeHelp:
      'Os títulos que atendem a estas regras ficam de fora, seja o que for que as regras acima digam. Sem exclusões, nenhum fica.',
    excludeMatch: 'Deixar de fora os títulos que atendem a',
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
      released: 'Data de lançamento',
      homeRelease: 'Lançamento em casa',
      votes: 'Votos no IMDb',
      runtime: 'Duração',
      contentRating: 'Classificação indicativa',
      person: 'Ator ou diretor',
      studio: 'Estúdio ou emissora',
    } as Record<string, string>,
    negation: 'Corresponde ou não',
    is: 'é',
    isNot: 'não é',
    genre: 'Gênero',
    from: 'De',
    to: 'Até',
    language: 'Idioma',
    rankMax: 'Entre os primeiros',
    minutes: 'minutos',
    atMost: 'No máximo',
    releasedWhen: 'Quando',
    releasedWithin: { last: 'Nos últimos', next: 'Nos próximos' } as Record<string, string>,
    homeReleaseWithin: {
      released: 'Lançado nos últimos',
      upcoming: 'Lança nos próximos',
      both: 'Em até',
    } as Record<string, string>,
    days: 'dias',
    namePlaceholder: { person: 'O nome', studio: 'O nome' } as Record<string, string>,
    types: { movie: 'Um filme', series: 'Uma série' } as Record<string, string>,
    states: {
      played: 'Reproduzido',
      unplayed: 'Não reproduzido',
      started: 'Iniciado, não concluído',
      favorite: 'Favorito',
    } as Record<string, string>,
    stateHelp:
      'As regras de reproduzido, iniciado e favorito seguem o que cada usuário fez: cada um vê a sua própria lista.',
    describedHelp:
      'O idioma de um título, e sua duração, classificação indicativa, pessoas e data de lançamento quando o catálogo não os informa, vêm com a sua descrição, que o Polyfin lê em segundo plano: um título ainda não descrito fica de fora até que seja. Um título a que falta o que uma regra olha não atende nem à regra nem ao seu contrário.',
    votesHelp:
      'Os votos no IMDb vêm do MDBList, com a chave da API do MDBList para avaliações em Configurações › Conteúdo, consultados em segundo plano. Sem essa chave, nenhum título tem votos.',
    studioHelp:
      'Os estúdios e as emissoras vêm do TMDB, com a chave da API do TMDB em Configurações › Conteúdo, consultados em segundo plano e guardados: a lista cresce à medida que chegam. Sem essa chave, contam apenas os estúdios que um administrador definiu nos metadados de um título.',
    homeReleaseHelp:
      'O lançamento em casa de um filme é seu primeiro lançamento digital ou físico, ou então 90 dias após a estreia nos cinemas, segundo o TMDB, com a chave da API do TMDB em Configurações › Conteúdo, consultado em segundo plano e guardado: a lista cresce à medida que chegam. O de uma série é sua data de estreia. Com Aguardar o lançamento digital ativado, um filme ainda não lançado só aparece nas listas com uma regra de títulos futuros.',
    sort: 'Ordenação',
    sorts: {
      catalog: 'Como os catálogos os listam, um de cada por vez',
      premiere: 'Data de estreia, mais recentes primeiro',
      latestEpisode: 'Último episódio exibido, mais recentes primeiro',
      seasonPremiere: 'Última estreia de temporada, mais recentes primeiro',
      rating: 'Avaliação IMDb, maiores primeiro',
      name: 'Nome',
      random: 'Aleatória, sorteada novamente a cada dia',
    } as Record<string, string>,
    episodeSortHelp:
      'As séries vêm pelo último episódio regular exibido, ou pela estreia da última temporada, como diz a descrição, sem especiais nem episódios por vir; os filmes pela data de lançamento. Uma série cuja descrição ainda não foi lida vem por último até que seja.',
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
        ? '1 título ainda espera o que as suas regras olham: a descrição, os votos ou os estúdios. A lista cresce à medida que chegam.'
        : `${count} títulos ainda esperam o que as suas regras olham: a descrição, os votos ou os estúdios. A lista cresce à medida que chegam.`,
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
  /** Episode orders: the order a series shows its seasons and episodes in, for every user. */
  episodeOrders: {
    title: 'Ordens de episódios',
    help: 'Uma série mostra suas temporadas e episódios como o complemento os numera, na ordem de exibição. Mostre uma em outra ordem do TMDB, como DVD, absoluta ou arco narrativo, para todos os usuários, como faz a ordem de exibição do editor de metadados do jellyfin-web. Seus episódios mantêm a própria temporada e número para reprodução, histórico e acompanhamento.',
    loading: 'Carregando as ordens de episódios…',
    emptyTitle: 'Todas as séries aparecem na ordem de exibição',
    emptyHow: 'Encontre uma série abaixo para mostrá-la em outra ordem.',
    search: 'Encontrar uma série',
    searchPlaceholder: 'Nome da série',
    searchButton: 'Buscar',
    searching: 'Buscando…',
    results: 'Séries encontradas',
    noSeries: 'Nenhuma série tem esse nome.',
    editTitle: (name: string) => `Ordem dos episódios de ${name}`,
    order: 'Ordem',
    orderHelp:
      'Os grupos de episódios que o TMDB dá para esta série. As partes de um grupo viram as temporadas dele.',
    aired: 'Ordem de exibição',
    types: {
      originalAirDate: 'Data de exibição original',
      absolute: 'Absoluta',
      dvd: 'DVD',
      digital: 'Digital',
      storyArc: 'Arco narrativo',
      production: 'Produção',
      tv: 'TV',
    },
    counts: (seasons: number, episodes: number) =>
      `${seasons === 1 ? '1 temporada' : `${seasons} temporadas`}, ${episodes === 1 ? '1 episódio' : `${episodes} episódios`}`,
    lookup: {
      no_key:
        'Adicione uma chave do TMDB em Configurações › Conteúdo para listar as ordens desta série.',
      unknown: 'O TMDB não conhece esta série: ela aparece na ordem de exibição.',
      failed:
        'Não foi possível acessar o TMDB para listar as ordens desta série. Tente de novo em instantes.',
    },
    specials: 'Especiais',
    specialsHelp:
      'Onde a temporada 0 aparece entre as temporadas. Especiais ocultos continuam tocando a partir de Continuar assistindo ou de um link.',
    specialsFirst: 'Primeiro',
    specialsLast: 'Por último',
    specialsHidden: 'Ocultos',
    specialsSummary: { last: 'Especiais por último', hidden: 'Especiais ocultos' },
    saved: (name: string) => `${name}: ordem dos episódios salva.`,
  },
}

export default libraries
