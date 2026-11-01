import type en from '../en/users'

const users: typeof en = {
  users: {
    title: 'Usuários',
    description: 'Crie e gerencie as contas autorizadas a usar este servidor.',
    listTitle: 'Contas',
    empty: 'Nenhum usuário ainda.',
    administrator: 'Administrador',
    hidden: 'Oculto na tela de login',
    disabled: 'Desativado',
    you: 'Você',
    createTitle: 'Criar um usuário',
    name: 'Nome',
    password: 'Senha',
    isAdministrator: 'Administrador',
    isAdministratorHelp: 'Administradores podem gerenciar usuários e as configurações do servidor.',
    showOnSignIn: 'Mostrar na tela de login',
    showOnSignInHelp:
      'Usuários visíveis aparecem na tela de login dos aplicativos Jellyfin. Usuários ocultos digitam o próprio nome, como no padrão do Jellyfin.',
    create: 'Criar usuário',
    creating: 'Criando…',
    created: (name: string) => `${name} foi criado.`,
    edit: 'Editar',
    close: 'Fechar',
    rename: 'Renomear',
    renamed: 'Nome alterado.',
    resetPassword: 'Redefinir senha',
    newPassword: 'Nova senha',
    resetPasswordHelp: 'Alterar a senha desconecta todos os dispositivos Jellyfin deste usuário.',
    passwordReset: 'Senha redefinida.',
    accessTitle: 'Acesso',
    isDisabled: 'Desativado',
    isDisabledHelp:
      'Um usuário desativado não consegue entrar, e todos os seus dispositivos são desconectados.',
    providerOnly: 'Somente provedor de identidade',
    providerOnlyHelp:
      'O usuário entra pelo provedor de identidade configurado em Configurações › Segurança, nunca com uma senha: a senha é recusada, e uma conta criada assim não precisa de uma. Os aplicativos Jellyfin que não conseguem abrir o provedor entram com o Quick Connect.',
    ended: 'Encerrada',
    endsOn: (date: string) => `Encerra em ${date}`,
    endedOn: (date: string) => `Encerrada em ${date}`,
    endTitle: 'Encerra em',
    endHelp:
      'A partir desta data, no fuso horário do servidor, a conta é recusada como uma conta desativada: ela não consegue entrar, e seus dispositivos são recusados. Tudo o que ela tem é mantido, e uma data posterior, ou “Nunca”, a reativa.',
    endAdministratorHelp:
      'A conta de um administrador nunca é encerrada: tornar um usuário administrador apaga a data de encerramento dele.',
    endDate: 'Data de encerramento',
    saveEnd: 'Salvar data',
    endNever: 'Nunca',
    canTranscode: 'Pode usar conversão (transcodificação)',
    canTranscodeHelp:
      'Permite que o servidor recodifique vídeo e áudio quando o aplicativo deste usuário não consegue reproduzir um arquivo como ele está. Sem isso, os arquivos são reproduzidos como estão ou apenas reempacotados.',
    canDownload: 'Pode baixar',
    canDownloadHelp:
      'Permite que este usuário salve títulos nos aplicativos Jellyfin para assistir offline.',
    canPlayTorrents: 'Pode reproduzir streams de torrent',
    canPlayTorrentsHelp:
      'Oferece a este usuário a lista de torrents dos complementos, enquanto Configurações › Torrents os reproduz. Desativado para todos, inclusive administradores, até ser concedido.',
    noTranscoding: 'Sem conversão',
    noDownloads: 'Sem downloads',
    turnOffDownloads: 'Desativar downloads para todos',
    turnOffDownloadsConfirm:
      'Todos os usuários e todos os grupos perdem a permissão de baixar, inclusive nos aplicativos Jellyfin já conectados. Você pode concedê-la de novo a um usuário ou grupo na página dele, em Acesso.',
    turningOffDownloads: 'Desativando…',
    downloadsTurnedOff: (count: number) =>
      count === 0
        ? 'Ninguém podia baixar.'
        : count === 1
          ? '1 usuário não pode mais baixar.'
          : `${count} usuários não podem mais baixar.`,
    lastAdminHelp: 'O último administrador ativo não pode ser excluído, rebaixado nem desativado.',
    updated: 'Alterações salvas.',
    metadataLanguageTitle: 'Idioma dos metadados',
    metadataLanguageHelp:
      'O idioma dos nomes, sinopses e imagens dos títulos nos aplicativos deste usuário, quando um dos complementos do servidor os descreve nesse idioma; títulos que nenhum complemento descreve nele mantêm a descrição do servidor. Gêneros, classificações etárias e pessoas continuam iguais para todos, então o controle parental e os gêneros bloqueados avaliam todos os usuários da mesma forma. Os nomes que o próprio Polyfin cria seguem esse idioma quando o Polyfin o fala: inglês, francês, alemão, espanhol, italiano ou português.',
    metadataLanguageNone:
      'Nenhum complemento do servidor tem idioma ainda. Defina um idioma para um complemento na página dele, em Conteúdo › Fontes.',
    webThemeTitle: 'Tema do player web',
    webThemeHelp:
      'A aparência do player web para este usuário, em cada navegador em que ele entra: o tema do servidor, LumaaGlaass, a aparência do jellyfin-web, mais leve, para TVs e celulares antigos, ou o tema personalizado enquanto ele tiver um texto. Navegadores antigos demais para o LumaaGlaass recebem sozinhos a aparência do jellyfin-web.',
    webThemeChange: 'Permitir que ele mude',
    webThemeChangeHelp:
      'Em Minha conta › Tema do player web. Desativado, o tema escolhido aqui continua sendo o dele.',
    audioRegionTitle: 'Região do áudio',
    audioRegionHelp:
      'A dublagem que este usuário prefere quando um idioma é dublado à parte para uma região, como o francês do Québec. Ela vale para o idioma de áudio escolhido nos aplicativos dele, ou então o do servidor, quando ele é o idioma da região, e o define quando nenhum foi escolhido: nesse idioma, as faixas de áudio da região tocam primeiro, e as versões dela vêm primeiro quando o servidor ordena as versões por idioma.',
    parentalTitle: 'Controle parental',
    maxRating: 'Classificação etária máxima',
    maxRatingHelp: 'Títulos com classificação acima deste limite ficam ocultos para este usuário.',
    noLimit: 'Sem limite',
    blockUnratedMovies: 'Bloquear filmes sem classificação',
    blockUnratedShows: 'Bloquear séries sem classificação',
    ratingLimit: (rating: string) => `Até ${rating}`,
    saveParental: 'Salvar controle parental',
    devicesTitle: 'Dispositivos',
    delete: 'Excluir usuário',
    deleteConfirm: (name: string) =>
      `Excluir ${name}? Os dispositivos dele serão desconectados, e esta ação não pode ser desfeita.`,
    deleting: 'Excluindo…',
    deleted: (name: string) => `${name} foi excluído.`,
    canAddAddons: 'Pode adicionar os próprios complementos',
    canAddAddonsHelp:
      'Permite que este usuário adicione os próprios complementos Stremio na página Minhas fontes. Sem isso, os complementos dele são mantidos, mas não usados.',
    noPersonalAddons: 'Sem complementos próprios',
    blockedUntil: (time: string) => `Bloqueado até ${time}`,
    blockedHelp:
      'Foram digitadas senhas incorretas demais para esta conta: ela não pode entrar até esse horário, mesmo com a senha correta.',
    unblock: 'Desbloquear',
    unblocking: 'Desbloqueando…',
    unblocked: 'A conta foi desbloqueada.',
    twoStepOn: 'O login em duas etapas está ativado.',
    twoStepHelp:
      'Se este usuário perdeu o celular com o aplicativo autenticador e não tem mais nenhum código de recuperação, desative-o: ele entra apenas com a senha e depois o configura novamente em Minha conta.',
    turnOffTwoStep: 'Desativar login em duas etapas',
    turningOffTwoStep: 'Desativando…',
    turnOffTwoStepTitle: (name: string) => `Desativar o login em duas etapas de ${name}?`,
    turnOffTwoStepConfirm: (name: string) =>
      `${name} passará a entrar apenas com a senha. O registro de atividades registra que você o desativou.`,
    twoStepTurnedOff: 'Login em duas etapas desativado para este usuário.',
    playbackAccessTitle: 'Reprodução e acesso',
    maxPlaybacks: 'Reproduções simultâneas (0 = sem limite)',
    maxPlaybacksHelp:
      'Quantos dispositivos deste usuário podem reproduzir ao mesmo tempo. Ele ainda pode entrar em mais dispositivos.',
    maxBitrate: 'Qualidade máxima',
    maxBitrateHelp:
      'Acima dela, o vídeo é convertido para uma qualidade menor quando este usuário pode usar a conversão; caso contrário, é reproduzida uma versão mais leve.',
    bitrateNoLimit: 'Sem limite',
    bitrate4k: '40 Mbit/s (4K)',
    bitrate1080High: '20 Mbit/s (1080p, alta qualidade)',
    bitrate1080: '10 Mbit/s (1080p)',
    bitrate720: '4 Mbit/s (720p)',
    bitrate480: '2 Mbit/s (480p)',
    bitrateOther: (mbits: string) => `${mbits} Mbit/s`,
    liveTv: 'TV ao Vivo',
    liveTvHelp: 'Mostra os canais de TV ao Vivo para este usuário.',
    syncPlay: 'Assistir juntos',
    syncPlayHelp:
      'Reproduzir o mesmo título em sincronia com outras pessoas, nos aplicativos Jellyfin que oferecem esse recurso.',
    syncPlayCreateAndJoin: 'Criar e entrar em grupos',
    syncPlayJoin: 'Apenas entrar',
    syncPlayNone: 'Não',
    remoteControl: 'Pode controlar os aplicativos de outros usuários',
    remoteControlHelp:
      'Permite que este usuário reproduza, pause ou envie mensagens para os aplicativos Jellyfin de outros usuários. Todos podem controlar os próprios aplicativos.',
    liveTvManagement: 'Pode gravar TV ao Vivo',
    liveTvManagementHelp: 'Agendar e excluir gravações.',
    localFirst: 'Reproduzir arquivos locais primeiro',
    localFirstHelp:
      'Coloca as versões das pastas locais e dos compartilhamentos de rede antes das outras nos menus de versões deste usuário, seja qual for “Ordenar versões por”, para que a reprodução comece por elas. Os outros usuários mantêm a ordem do servidor.',
    boostVolume: 'Aumentar o volume no player web',
    boostVolumeHelp:
      'Deixa os vídeos mais altos no player web deste usuário, em um navegador e nos aplicativos Jellyfin para Android e iOS: cerca de 8 dB nas passagens calmas e 5 dB nas altas, com os picos contidos perto do fundo de escala. O navegador faz o trabalho, mas para que ele alcance as transmissões dos complementos, o servidor envia ele mesmo essas transmissões ao player web deste usuário em vez de mandá-lo à fonte delas, o que usa a banda do servidor. Não alcança os players próprios dos aplicativos, nem as transmissões que um iPhone ou iPad reproduz como HLS.',
    convertSound: 'Converter o som para equalizá-lo',
    convertSoundHelp:
      'O servidor converte o áudio dos vídeos e canais deste usuário e o equaliza em -16 LUFS, com a imagem copiada, para que todo aplicativo o reproduza mais alto e uniforme. O custo: nada é reproduzido diretamente, cada reprodução usa um pouco do processador do servidor, e o som surround chega a um receiver convertido em vez de como a faixa original.',
    savePlaybackAccess: 'Salvar reprodução e acesso',
    visibleLibrariesTitle: 'Bibliotecas visíveis',
    visibleLibrariesHelp:
      'As bibliotecas do servidor que os aplicativos deste usuário mostram. Bibliotecas adicionadas depois são mostradas. Isso apenas escolhe as bibliotecas mostradas: os títulos continuam acessíveis por outras bibliotecas, pela pesquisa ou por links. Para bloquear títulos, use o controle parental ou os gêneros bloqueados.',
    noServerLibraries: 'O servidor ainda não tem nenhuma biblioteca.',
    saveVisibleLibraries: 'Salvar bibliotecas visíveis',
    blockedGenresTitle: 'Gêneros bloqueados',
    blockedGenresHelp:
      'Títulos de qualquer um destes gêneros ficam ocultos para este usuário em todos os lugares, como os títulos acima do limite do controle parental dele.',
    genre: 'Gênero',
    genreHint: 'Escolha um gênero das bibliotecas do servidor ou digite um.',
    addGenre: 'Adicionar',
    removeGenre: (genre: string) => `Remover ${genre}`,
    noBlockedGenres: 'Nenhum gênero está bloqueado.',
    saveBlockedGenres: 'Salvar gêneros bloqueados',
    liveTvChannelsTitle: 'Canais da TV ao Vivo',
    liveTvChannelsHelp:
      'Os canais das fontes de TV ao Vivo do servidor que este usuário vê, escolhidos por grupo e por canal. Canais ocultos ficam fora da lista de canais, do guia, das buscas, do Replay, das gravações, dos agendamentos e dos lembretes, e não são reproduzidos. As fontes próprias do usuário não são limitadas.',
    channelsMode: 'Canais',
    channelsAll: 'Todos',
    channelsOnly: 'Somente estes',
    channelsExcept: 'Todos exceto estes',
    channelsOnlyHelp: 'Somente os grupos e canais marcados são mostrados.',
    channelsExceptHelp: 'Os grupos e canais marcados ficam ocultos.',
    channelGroups: 'Grupos',
    filterChannelGroups: 'Filtrar grupos',
    moreChannelGroups: (count: number) => `Mais ${count}: filtre para encontrá-los.`,
    newChannelGroup: 'Novo',
    noChannelGroups: 'As fontes de TV ao Vivo do servidor ainda não têm canais.',
    singleChannels: 'Canais',
    searchChannels: 'Buscar canais por nome ou número',
    tickedChannels: 'Canais marcados',
    noTickedChannels: 'Nenhum canal marcado.',
    goneChannel: 'Canal que não está mais listado',
    removeChannel: (name: string) => `Remover ${name}`,
    moreChannels: (count: number) => `Mais ${count}: busque para encontrá-los.`,
    hideNewChannelGroups: 'Ocultar os canais de grupos adicionados depois',
    hideNewChannelGroupsHelp:
      'Os grupos que uma fonte adicionar depois de salvar ficam ocultos. Eles aparecem aqui marcados, como Novo, até salvar de novo.',
    saveLiveTvChannels: 'Salvar canais da TV ao Vivo',
    allowedHoursTitle: 'Horários permitidos',
    allowedHoursHelp:
      'Quando há horários definidos, este usuário só pode entrar e usar o servidor durante eles, no fuso horário do servidor. Fora deles, os aplicativos dele são recusados.',
    noAllowedHours: 'Nenhum horário definido: este usuário pode usar o servidor a qualquer hora.',
    day: 'Dia',
    from: 'De',
    to: 'Até',
    addHours: 'Adicionar horário',
    removeHours: 'Remover',
    hoursOrder: 'Cada horário de término deve ser posterior ao horário de início.',
    saveAllowedHours: 'Salvar horários permitidos',
    days: {
      Sunday: 'Domingo',
      Monday: 'Segunda-feira',
      Tuesday: 'Terça-feira',
      Wednesday: 'Quarta-feira',
      Thursday: 'Quinta-feira',
      Friday: 'Sexta-feira',
      Saturday: 'Sábado',
      Everyday: 'Todos os dias',
      Weekday: 'Dias úteis (segunda a sexta-feira)',
      Weekend: 'Fins de semana',
    },
    canManageCollections: 'Pode gerenciar coleções',
    canManageCollectionsHelp:
      'Permite que este usuário crie coleções de títulos nos aplicativos Jellyfin, adicione ou remova títulos delas e as exclua. Todos os usuários veem as coleções, cada um apenas com os títulos que pode ver.',
    resetPinRequested: 'Redefinição de senha solicitada: PIN',
    resetPinValidUntil: (time: string) => `, válido até ${time}`,
    resetPinHelp: 'Passe este PIN ao usuário: ele entra com o PIN, que se torna a nova senha dele.',
    canManageSubtitles: 'Pode gerenciar legendas',
    canManageSubtitlesHelp:
      'Permite que este usuário adicione arquivos de legenda aos títulos pelos aplicativos Jellyfin dele e pesquise ali as legendas dos complementos.',
    canShare: 'Pode compartilhar títulos',
    canShareHelp:
      'Permite que este usuário dê acesso a um filme ou episódio a alguém sem conta, por meio de um link que expira, em Minha conta ou na página de um título no player web, enquanto Configurações › Segurança › Permitir links de compartilhamento estiver ativado. Desativar revoga os links dele.',
    remoteAccess: 'Pode se conectar de fora da rede local',
    remoteAccessHelp:
      'Permite que este usuário entre, navegue e reproduza de qualquer lugar. Desativado, só pode fazer isso da rede local, mesmo sendo administrador, conforme os Proxies conhecidos em Configurações › Segurança a definem. Os links que ele compartilha continuam sendo reproduzidos em qualquer lugar.',
    remoteAccessUnknownProxy:
      'Integridade vê um proxy reverso que não está em Proxies conhecidos: todos que passam por ele parecem locais, então esta limitação não vale para eles.',
    openKnownProxies: 'Abrir Proxies conhecidos',
    shareLinksTitle: 'Links compartilhados',
    shareLinksHelp:
      'Os links que este usuário criou para compartilhar um filme ou um episódio. “Revogar” encerra um deles imediatamente.',
    qualityGroup: 'Grupo de qualidade',
    qualityGroupHelp:
      'A resolução mais alta oferecida a este usuário. Versões superiores são deixadas de fora enquanto houver uma que se encaixe; se nenhuma se encaixar, elas são convertidas para uma resolução menor, ou recusadas quando este usuário não pode usar a conversão. A TV ao Vivo também é convertida para uma resolução menor. Somente as versões que se encaixam são baixadas como estão; com “Downloads convertidos” ativado em Configurações › Conversão (transcodificação), os aplicativos que solicitarem baixam as outras convertidas para uma resolução menor.',
    qualityGroupOriginal: 'Original (sem limite)',
    member: 'Membro',
    lastSignInLabel: 'Último login:',
    active: 'Ativo',
    pinRequested: 'PIN solicitado',
    emptyHelp:
      'Crie uma conta para cada pessoa que assiste aqui e, em seguida, entre com ela em um aplicativo Jellyfin.',
    devicesEmptyHelp: 'Os aplicativos aparecem aqui assim que entram com esta conta.',
    notFound: 'Usuário não encontrado',
    notFoundHelp:
      'Este usuário não existe mais, ou o endereço está incorreto. Escolha um usuário na lista.',
    backToUsers: 'Voltar para usuários',
    sectionsLabel: 'Seções do usuário',
    profileTitle: 'Nome e senha',
    accessHelp: 'Cada alteração é salva imediatamente.',
    noServerLibrariesHelp:
      'Adicione uma biblioteca em Conteúdo › Bibliotecas e, em seguida, escolha aqui quais este usuário vê.',
    jellyfinImport: {
      open: 'Importar de outro servidor',
      title: 'Importar de outro servidor',
      description:
        'Traga contas e seus dados de exibição de um servidor Jellyfin, Emby ou Plex: nomes das contas e status de administrador e, para cada usuário, os filmes e episódios reproduzidos com suas datas, pontos de retomada e favoritos, identificados pelo IMDb, TMDB ou TVDB, e se quiser suas avaliações, as coleções do servidor e suas listas de reprodução. Os dados do Polyfin são apenas complementados, e nada é gravado no outro servidor. Os servidores não fornecem senhas: você define uma para cada nova conta ou mantém aquela com que o usuário entra.',
      serverKind: 'Servidor',
      servers: {
        jellyfin: 'Jellyfin',
        emby: 'Emby',
        plex: 'Plex',
      },
      serverKindHelp: {
        jellyfin: 'Jellyfin, ou um servidor compatível com a API dele.',
        emby: 'Emby, com suas próprias chaves da API e usuários.',
        plex: 'Plex Media Server, lido com o token do proprietário.',
      },
      connectTitle: (server: string) => `Conectar ao ${server}`,
      connectHelp:
        'O endereço, a chave e as senhas são usados apenas nesta importação: o Polyfin não os salva.',
      address: 'Endereço do servidor',
      addressHelp: (server: string) =>
        `O endereço em que o ${server} abre em um navegador. Um endereço da rede local funciona.`,
      plexAddressHelp:
        'O endereço do próprio Plex Media Server, geralmente na porta 32400. Um endereço da rede local funciona.',
      connectWith: 'Conectar com',
      withApiKey: 'Uma chave da API',
      withAccount: 'Uma conta de usuário',
      apiKey: 'Chave da API',
      apiKeyHelp:
        'Crie uma no painel do Jellyfin, em Chaves de API: ela lê os dados de exibição de todos os usuários.',
      embyApiKeyHelp:
        'Crie uma nas configurações do Emby, em Chaves de API: ela lê os dados de exibição de todos os usuários. A chave pessoal de um usuário não é aceita: o Emby não informa a quem ela pertence.',
      plexToken: 'Token do Plex',
      plexTokenHelp:
        'O token do proprietário do servidor, que o Polyfin usa apenas para ler o servidor. Para encontrá-lo, abra o Plex em um navegador, conectado como proprietário. Abra o menu de qualquer filme, escolha Get Info e depois View XML: o endereço da página que se abre termina com X-Plex-Token= seguido do token. Copie o que vem depois.',
      accountName: 'Nome de usuário',
      accountNameHelp: (server: string) =>
        `A conta de um administrador lista todos os usuários. O Polyfin entra como os aplicativos do ${server} fazem e depois sai.`,
      accountPassword: 'Senha',
      accountPasswordHelp: 'Deixe em branco se a conta não tiver senha.',
      connect: 'Conectar',
      connecting: 'Conectando…',
      chooseTitle: 'Escolher quem importar',
      chooseHelp: (server: string, product: string, version: string) =>
        `${server}, ${product} ${version}. Para cada usuário, escolha a conta do Polyfin para onde importar ou crie uma.`,
      changeServer: 'Trocar de servidor',
      noUsers: (product: string) => `Este servidor ${product} não tem nenhum usuário.`,
      lastActivityLabel: 'Última atividade:',
      importAs: 'Importar para',
      skip: 'Não importar',
      newUser: 'Novo usuário',
      watchData: 'Importar dados de exibição',
      watchDataHelp: 'Títulos reproduzidos com suas datas, pontos de retomada e favoritos.',
      plexOwnerWatchDataHelp:
        'Títulos reproduzidos com suas datas e número de reproduções, pontos de retomada e avaliações.',
      plexWatchDataHelp:
        'Os títulos que esta conta reproduziu até o fim neste servidor, segundo o histórico de reprodução do Plex, com suas datas e número de reproduções. Os títulos que ela marcou como assistidos ou sincronizou da conta Plex, e os pontos de retomada dela, ficam no Plex.',
      watchDataNeeded: 'Sem isso, nada é importado para este usuário.',
      extras: {
        title: 'Trazer também',
        ratings: 'Avaliações',
        ratingsHelp:
          'As avaliações de cada usuário, de 0 a 10, como estão. Um joinha sem avaliação vira favorito. Uma avaliação que o Polyfin já tem é mantida.',
        collections: 'Coleções',
        collectionsHelp:
          'As coleções do servidor, como coleções do Polyfin que todo usuário vê, com os títulos encontrados. Uma coleção sem nenhum título encontrado não é criada.',
        playlists: 'Listas de reprodução',
        playlistsHelp:
          'As listas de reprodução de vídeo de cada usuário, como listas dele, na ordem, com os títulos encontrados.',
        plexHelp:
          'O Plex só fornece avaliações e listas de reprodução das contas lidas com o próprio token: a do proprietário e, depois que você entra no Plex como proprietário, as dos outros.',
      },
      plexNotice:
        'Com o token do proprietário, o Plex permite que o Polyfin leia os títulos reproduzidos, os pontos de retomada e as avaliações do proprietário. Das outras contas, ele fornece apenas o histórico de reprodução neste servidor: os títulos que elas reproduziram até o fim aqui. Os títulos marcados como assistidos sem reprodução, os sincronizados da conta Plex delas, os pontos de retomada e as avaliações delas não estão nele. Para ler os dados de exibição de cada conta, entre com o Plex como proprietário, acima. O Plex não tem favoritos para importar.',
      plexOwner: {
        title: 'Ler cada conta com o Plex',
        help: 'Entre com o Plex como proprietário deste servidor. O Plex então fornece ao Polyfin o token de cada conta com a qual o servidor é compartilhado, e o Polyfin lê com ele o que a conta reproduziu, marcou como assistido ou sincronizou, os pontos de retomada e as avaliações dela, não apenas as reproduções aqui. Os perfis do seu Plex Home são lidos com o PIN deles, quando têm um.',
        signIn: 'Entrar com o Plex como proprietário',
        checking: 'Perguntando ao Plex para quais contas ele fornece um token…',
        signedInAs: (name: string) => `Conectado ao Plex como ${name}, proprietário do servidor.`,
        signOut: 'Sair do Plex',
        notice:
          'O Polyfin lê cada conta abaixo com o token que o Plex fornece ao proprietário para ela, como os apps do Plex dessa conta leem este servidor. Os tokens servem apenas para esta importação: nunca são guardados, e a sua sessão no Plex é encerrada quando a importação termina.',
        all: 'Tudo: os títulos reproduzidos, marcados como assistidos ou sincronizados, os pontos de retomada e as avaliações.',
        pin: 'Um perfil do Plex Home com PIN: tudo com o PIN dele, sem ele apenas as reproduções neste servidor.',
        plays:
          'Apenas as reproduções neste servidor, segundo o histórico de reprodução do Plex: o Plex não fornece token para esta conta.',
        everything: 'Tudo',
        playsOnly: 'Só reproduções',
        pinLabel: 'PIN do perfil',
        pinHelp: (name: string) =>
          `O PIN que ${name} pede no Plex. Se ficar vazio, apenas as reproduções são lidas.`,
      },
      ownerNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Conectado como ${owner ?? 'um usuário'}, o Polyfin lê apenas os dados de exibição desse usuário.` : `Esta chave pertence a ${owner ?? 'um usuário'}: ela lê apenas os dados de exibição desse usuário.`} Para importar os de outro usuário, digite a senha da conta dele no servidor: o Polyfin entra como ele para lê-los. Sem a senha, a conta dele ainda pode ser criada.`,
      ownerOnlyNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Conectado como ${owner ?? 'este usuário'}` : `Esta chave pertence a ${owner ?? 'um usuário'}`}, que não é administrador deste servidor: o Polyfin importa apenas esta conta. Para listar os outros usuários do servidor, conecte-se com a conta de um administrador ou uma chave da API.`,
      serverPassword: (server: string) => `Senha no ${server}`,
      serverPasswordHelp: (user: string) =>
        `O Polyfin entra como ${user} para ler os dados de exibição e depois sai. Deixe em branco se a conta não tiver senha.`,
      keepPassword: 'Manter esta senha no Polyfin',
      keepPasswordHelp:
        'A nova conta entra no Polyfin com a mesma senha, se ela tiver pelo menos 8 caracteres.',
      nothingToImport: 'Escolha um usuário para criar ou dados de exibição para importar.',
      start: 'Iniciar importação',
      starting: 'Iniciando…',
      started: (created: number, importing: boolean) =>
        created === 0
          ? 'A importação foi iniciada.'
          : `${created === 1 ? '1 usuário criado.' : `${created} usuários criados.`}${importing ? ' A importação foi iniciada.' : ''}`,
      currentTitle: 'Importação em andamento',
      lastTitle: 'Última importação',
      startedLabel: 'Iniciada',
      endedLabel: 'Concluída',
      startedBy: (name: string, own: boolean) =>
        own ? `Histórico de exibição de ${name}, em Minha conta` : `Iniciada por ${name}`,
      runningHelp:
        'Você pode sair desta página: a importação continua. Se o Polyfin reiniciar nesse meio-tempo, a importação para: o que já foi importado permanece, e importar novamente não duplica nada.',
      stop: 'Parar importação',
      stopping: 'Parando…',
      states: {
        running: 'Em andamento',
        done: 'Concluída',
        stopped: 'Interrompida',
        failed: 'Falhou',
      },
      problems: {
        jellyfin_unreachable: (server: string) =>
          `Não foi possível acessar ${server} durante a importação. O que já foi importado foi mantido.`,
        jellyfin_key_refused: (server: string) =>
          `${server} recusou a chave da API durante a importação: ela pode ter sido revogada. O que já foi importado foi mantido.`,
        not_jellyfin: (server: string) =>
          `O servidor deixou de responder como ${server} durante a importação. O que já foi importado foi mantido.`,
        internal: () =>
          'O Polyfin encontrou um erro durante a importação: o registro do servidor traz mais detalhes. O que já foi importado foi mantido.',
        jellyfin_user_forbidden: () =>
          'O servidor não permite que esta chave leia os dados deste usuário. Importe-os com uma chave da API do painel do servidor ou com a chave do próprio usuário.',
      },
      errors: {
        emby: {
          invalid_jellyfin_address:
            'Digite o endereço do servidor Emby, como http://192.168.1.10:8096.',
          jellyfin_key_refused:
            'O Emby recusou esta chave da API. Crie uma nas configurações dele, em Chaves de API, e cole-a novamente.',
          jellyfin_sign_in_refused: 'O Emby recusou este nome ou senha.',
          jellyfin_sign_in_forbidden:
            'O Emby não permite que esta conta entre: ela pode estar desativada ou fora do horário permitido.',
          jellyfin_password_refused:
            'O Emby recusou esta senha. Deixe-a em branco se a conta não tiver senha.',
          jellyfin_unreachable:
            'Nada respondeu neste endereço. Verifique-o e confirme que o Emby está em execução.',
          not_jellyfin:
            'Um servidor respondeu neste endereço, mas não é o Emby. Verifique o endereço.',
        },
        plex: {
          invalid_jellyfin_address:
            'Digite o endereço do Plex Media Server, como http://192.168.1.10:32400.',
          jellyfin_key_refused:
            'O Plex recusou este token. Copie novamente o token do proprietário.',
          jellyfin_key_limited:
            'Este token não pode listar as contas do servidor: ele não é do proprietário do servidor. Use o token do proprietário.',
          jellyfin_unreachable:
            'Nada respondeu neste endereço. Verifique-o e confirme que o Plex Media Server está em execução.',
          not_jellyfin:
            'Um servidor respondeu neste endereço, mas não é o Plex Media Server. Verifique o endereço.',
        },
      } as Record<'emby' | 'plex', Partial<Record<string, string>>>,
      userStates: {
        waiting: 'Aguardando',
        reading: 'Lendo',
        saving: 'Salvando',
        done: 'Concluído',
        failed: 'Falhou',
      },
      notImported: 'Não importado',
      userMapping: (jellyfinName: string, userName: string) => `${jellyfinName} → ${userName}`,
      read: (count: number) => (count === 1 ? '1 item lido' : `${count} itens lidos`),
      counts: (played: number, resumed: number, favorites: number) =>
        `${played === 1 ? '1 marcado como reproduzido' : `${played} marcados como reproduzidos`}, ${resumed === 1 ? '1 ponto de retomada' : `${resumed} pontos de retomada`}, ${favorites === 1 ? '1 favorito' : `${favorites} favoritos`}`,
      upToDate: (count: number) => `${count} já no Polyfin`,
      ratings: (count: number) => (count === 1 ? '1 avaliado' : `${count} avaliados`),
      collections: (count: number) =>
        count === 1 ? '1 coleção criada' : `${count} coleções criadas`,
      playlists: (count: number) =>
        count === 1 ? '1 lista de reprodução criada' : `${count} listas de reprodução criadas`,
      listTitles: (count: number) =>
        count === 1
          ? '1 título adicionado a listas já importadas'
          : `${count} títulos adicionados a listas já importadas`,
      skipped: {
        collection: (name: string) =>
          `Coleção “${name}”: nenhum título encontrado, não foi criada.`,
        playlist: (name: string) =>
          `Lista de reprodução “${name}”: nenhum título encontrado, não foi criada.`,
      },
      watchlist: (count: number) =>
        count === 1 ? '1 adicionado a “Para assistir”' : `${count} adicionados a “Para assistir”`,
      undated: (count: number) =>
        count === 1
          ? '1 ponto de retomada sem data, não adicionado'
          : `${count} pontos de retomada sem data, não adicionados`,
      nothingAdded: {
        up_to_date: () => 'Tudo o que foi lido já estava no Polyfin: não havia nada a adicionar.',
        nothing_played: (server: string) =>
          `${server} não lista nada reproduzido, em andamento ou favorito para este usuário.`,
        no_sections: () =>
          'Este servidor Plex não tem biblioteca de filmes ou séries: não havia nada para ler.',
        plex_owner_nothing_watched: () =>
          'O proprietário não tem nada reproduzido ou em andamento nas bibliotecas de filmes e séries deste servidor.',
        plex_nothing_watched: () =>
          'Esta conta não tem nada reproduzido, em andamento ou avaliado nas bibliotecas de filmes e séries deste servidor.',
        plex_no_history: () =>
          'O Plex não lista nenhuma reprodução desta conta neste servidor. O histórico de reprodução dela contém apenas os títulos reproduzidos até o fim neste servidor: títulos marcados como assistidos sem reprodução, ou sincronizados da conta Plex, não estão nele. Para trazer tudo, pontos de retomada incluídos, entre com o Plex como proprietário nesta importação, com o PIN de um perfil do Plex Home que tenha um, ou a pessoa entra com o Plex em Minha conta › Importar de outro servidor.',
      },
      unmatched: (count: number) =>
        count === 1 ? '1 título não encontrado' : `${count} títulos não encontrados`,
      unmatchedTitle: (name: string) => `Títulos não encontrados para ${name}`,
      unmatchedHelp: 'Estes títulos não foram importados: o motivo diz por quê.',
      titleColumn: 'Título',
      kindColumn: 'Tipo',
      yearColumn: 'Ano',
      reasonColumn: 'Motivo',
      kinds: {
        movie: 'Filme',
        episode: 'Episódio',
        series: 'Série',
      },
      reasons: {
        no_identifier: 'Nenhum identificador do IMDb, TMDB ou TVDB',
        not_found: 'Não está no Polyfin',
        not_in_library: 'Não está mais nas bibliotecas do Plex',
        other_episode_order: 'Numerado em outra ordem de episódios do TheTVDB',
      },
      more: (count: number) => (count === 1 ? 'E mais 1.' : `E mais ${count}.`),
    },
    appImport: {
      open: 'Importar de um aplicativo',
      title: 'Importar de um aplicativo',
      description:
        'Traga os dados de visualização de um usuário a partir da conta dele em um aplicativo baseado em complementos: os filmes e episódios reproduzidos com as datas, os pontos de retomada e a biblioteca do aplicativo como favoritos, identificados por IMDb, TMDB ou Kitsu. Os dados do Polyfin são apenas complementados, e nada é gravado no aplicativo. Você precisa do login do usuário no aplicativo; os usuários também podem importar por conta própria, em Minha conta.',
      user: 'Para a conta de',
      userHelp:
        'O usuário do Polyfin a cujos dados de visualização e complementos a importação será adicionada.',
    },
    letterboxdImport: {
      open: 'Importar do Letterboxd',
      title: 'Importar do Letterboxd',
      description:
        'Traga os filmes de um usuário a partir do arquivo de exportação do Letterboxd: os filmes assistidos com as datas e o número de reproduções, a watchlist, as avaliações e, se você quiser, os filmes curtidos como favoritos. Os dados do Polyfin são apenas complementados. Os usuários também podem importar os próprios dados, em Minha conta.',
      user: 'Para a conta de',
      userHelp: 'O usuário do Polyfin a cujos dados de visualização a importação será adicionada.',
    },
  },
  invites: {
    title: 'Links de convite',
    create: 'Criar um link de convite',
    empty: 'Nenhum link de convite ainda.',
    emptyHelp:
      'Um link de convite permite que as pessoas para quem você o enviar criem a própria conta, com as configurações que você escolher.',
    maxUses: 'Contas que ele pode criar',
    maxUsesHelp: 'De 1 a 100. 1 por padrão.',
    expires: 'Expira',
    after: (days: number) => (days === 1 ? 'Após 1 dia' : `Após ${days} dias`),
    never: 'Nunca',
    accountsEnd: 'Encerramento das contas',
    accountsEndHelp:
      'Cada conta criada pelo link é encerrada esta quantidade de dias após ser criada.',
    accountsEndAfter: (days: number) =>
      days === 1
        ? 'As contas são encerradas após 1 dia'
        : `As contas são encerradas após ${days} dias`,
    model: 'Configurações de',
    modelHelp:
      'As novas contas copiam as permissões, as bibliotecas visíveis, os gêneros bloqueados, os horários permitidos, o controle parental, os limites de reprodução e de acesso e o grupo de qualidade deste usuário. Elas nunca são administradoras.',
    newUser: 'Um novo usuário, como em Criar um usuário',
    submit: 'Criar link',
    creating: 'Criando…',
    createdTitle: 'Link de convite criado',
    createdNotice: 'Copie este link agora: ele é exibido apenas desta vez.',
    copy: 'Copiar link',
    copied: 'Link copiado.',
    copyFailed: 'Não foi possível copiar o link. Selecione-o e copie-o manualmente.',
    done: 'Concluído',
    uses: (uses: number, maxUses: number) =>
      maxUses === 1 ? `${uses} de 1 conta criada` : `${uses} de ${maxUses} contas criadas`,
    settingsOf: (name: string) => `Configurações de ${name}`,
    newUserSettings: 'Configurações de novo usuário',
    expiresLabel: 'Expira',
    expiredLabel: 'Expirado',
    neverExpires: 'Nunca expira',
    createdBy: (name: string) => `Criado por ${name}`,
    createdByDeleted: 'Criado por um usuário excluído',
    states: {
      active: 'Ativo',
      used_up: 'Esgotado',
      expired: 'Expirado',
      revoked: 'Revogado',
    },
    revoke: 'Revogar',
    revokeTitle: 'Revogar este link de convite?',
    revokeConfirm:
      'O link deixa de funcionar imediatamente. As contas já criadas por meio dele são mantidas.',
    revoking: 'Revogando…',
    revoked: 'Link de convite revogado.',
  },
  groups: {
    tabsLabel: 'Usuários e grupos',
    title: 'Grupos',
    description:
      'Um grupo reúne os limites da página de um usuário. Seus membros o seguem: uma alteração no grupo chega a todos eles de uma vez.',
    listTitle: 'Grupos',
    empty: 'Nenhum grupo ainda.',
    emptyHelp:
      'Crie um grupo para definir de uma vez as permissões, os limites de reprodução, o controle parental, as bibliotecas, os gêneros e os horários de várias contas.',
    create: 'Criar um grupo',
    name: 'Nome',
    nameTaken: 'Este nome já é usado por outro grupo.',
    nameRule: 'De 1 a 64 caracteres, diferente dos nomes dos outros grupos.',
    copyFrom: 'Configurações de',
    copyFromHelp:
      'O grupo começa com as permissões, os limites de reprodução e de acesso, o grupo de qualidade, o controle parental, as bibliotecas visíveis, os gêneros bloqueados e os horários permitidos deste usuário.',
    submit: 'Criar grupo',
    creating: 'Criando…',
    created: (name: string) => `O grupo ${name} foi criado.`,
    memberCount: (count: number) =>
      count === 0 ? 'Nenhum membro' : count === 1 ? '1 membro' : `${count} membros`,
    notFound: 'Grupo não encontrado',
    notFoundHelp: 'Ele pode ter sido excluído.',
    backToGroups: 'Voltar aos grupos',
    sectionsLabel: 'Seções do grupo',
    pageHelp: 'Cada seção é salva separadamente e chega na hora aos membros que seguem o grupo.',
    rename: 'Renomear',
    renamed: 'O grupo foi renomeado.',
    updated: 'O grupo foi atualizado.',
    accessHelp:
      'As permissões que os membros recebem do grupo. A função, a tela de login, “Desativado” e a data de encerramento continuam na página de cada um.',
    delete: 'Excluir grupo',
    deleteTitle: (name: string) => `Excluir o grupo ${name}?`,
    deleteConfirm:
      'Seus membros mantêm os valores do grupo como configurações próprias, que suas páginas poderão alterar depois.',
    deleting: 'Excluindo…',
    deleted: (name: string) => `O grupo ${name} foi excluído.`,
    membersTitle: 'Membros',
    membersHelp:
      'Um membro que deixa de seguir o grupo mantém os valores dele como configurações próprias, marcadas como “Configurações próprias”.',
    noMembers: 'Nenhum membro ainda.',
    noMembersHelp:
      'Adicione membros aqui ou escolha o grupo na seção Acesso da página de um usuário.',
    ownSettings: 'Configurações próprias',
    followsHelp: 'Segue o grupo',
    ownSettingsHelp: 'Mantém as próprias configurações',
    addMembers: 'Adicionar membros',
    addMembersTitle: (name: string) => `Adicionar membros a ${name}`,
    addMembersHelp:
      'Os usuários escolhidos seguem o grupo: eles recebem os valores do grupo no lugar dos seus. Administradores não podem entrar em um grupo.',
    noCandidates: 'Ninguém para adicionar.',
    noCandidatesHelp: 'Todos os usuários que não são administradores já estão neste grupo.',
    movesFrom: (name: string) => `Sai do grupo ${name}`,
    add: 'Adicionar',
    adding: 'Adicionando…',
    membersAdded: (count: number) =>
      count === 1 ? '1 membro foi adicionado.' : `${count} membros foram adicionados.`,
    group: 'Grupo',
    noGroup: 'Nenhum grupo',
    groupHelp:
      'Um membro segue o grupo: os valores dele se aplicam, e esta página os mostra somente para leitura. Ao sair, eles ficam como configurações próprias do usuário.',
    administratorNoGroup:
      'Administradores não podem entrar em um grupo, pois suas permissões são diferentes.',
    administratorLeaves: (name: string) =>
      `Tornar este usuário administrador o remove do grupo ${name}.`,
    newAccountGroupHelp: 'As contas criadas entram neste grupo e o seguem.',
    newUserGroupHelp: 'O usuário entra neste grupo e o segue.',
    followGroup: 'Seguir o grupo',
    followGroupHelp:
      'Desativado, o usuário mantém os valores atuais do grupo como seus, e esta página pode alterá-los.',
    setByGroup: (name: string) => `Definido pelo grupo ${name}.`,
    openGroup: 'Abrir o grupo',
    joinTitle: 'Entrar no grupo?',
    joinConfirm: (user: string, group: string) =>
      `${user} entra em ${group} e passa a segui-lo: os valores do grupo substituem os seus em Acesso, Reprodução e acesso, Controle parental, Bibliotecas visíveis, Gêneros bloqueados e Horários permitidos.`,
    join: 'Entrar',
    followAgainTitle: 'Seguir o grupo novamente?',
    followAgainConfirm: (user: string, group: string) =>
      `${user} volta a receber os valores de ${group}: as configurações próprias nas seções do grupo serão substituídas.`,
    followAgain: 'Seguir o grupo',
    groupBadge: (name: string) => `Grupo ${name}`,
    groupOwnBadge: (name: string) => `Grupo ${name}, configurações próprias`,
    filter: 'Mostrar os usuários de',
    allGroups: 'Todos os grupos',
    noneShown: 'Nenhum usuário aqui.',
  },
  resetDefaults: {
    button: 'Redefinir para o padrão',
    title: (name: string) => `Redefinir as configurações de ${name}?`,
    groupTitle: (name: string) => `Redefinir as configurações do grupo ${name}?`,
    userHelp: (name: string) =>
      `As permissões de Acesso, Reprodução e acesso com o Grupo de qualidade, Controle parental, Bibliotecas visíveis, Gêneros bloqueados e Horários permitidos voltam às configurações de um novo usuário, as que “Criar um usuário” dá; os apps de ${name} já conectados as recebem na hora. O nome e a senha, Administrador, Mostrar na tela de login, Desativado, Encerra em, o grupo, os próprios addons, as conexões, as contas de acompanhamento e tudo o que foi assistido continuam.`,
    ownSettingsHelp: (group: string) =>
      `Continua no grupo ${group} com as próprias configurações, sem segui-lo.`,
    groupHelp:
      'As permissões de Acesso, Reprodução e acesso com o Grupo de qualidade, Controle parental, Bibliotecas visíveis, Gêneros bloqueados e Horários permitidos voltam às configurações de um novo usuário. O nome e os membros do grupo continuam.',
    followers: (count: number) =>
      count === 0
        ? 'Nenhum membro segue o grupo: a mudança não chega a ninguém.'
        : count === 1
          ? '1 membro segue o grupo e recebe essas configurações na hora.'
          : `${count} membros seguem o grupo e recebem essas configurações na hora.`,
    changesTitle: 'O que muda',
    loading: 'Procurando o que muda…',
    noChange: 'Nada muda: já são as configurações de um novo usuário.',
    confirm: 'Redefinir',
    resetting: 'Redefinindo…',
    done: (name: string) => `As configurações de ${name} foram redefinidas.`,
    groupDone: (name: string) => `As configurações do grupo ${name} foram redefinidas.`,
  },
}

export default users
