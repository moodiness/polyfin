import type en from '../en/account'

const account: typeof en = {
  account: {
    title: 'Minha conta',
    signedInAs: 'Conectado como ',
    sectionsLabel: 'Seções da conta',
    sections: {
      statistics: 'Estatísticas',
      tracking: 'Acompanhamento',
      resume: 'Continuar assistindo',
      hidden: 'Títulos ocultos',
      homeRows: 'Fileiras do início',
      metadataLanguage: 'Idioma dos metadados',
      sharedLinks: 'Links compartilhados',
      reminders: 'Lembretes',
      import: 'Importar de outro servidor',
      appImport: 'Importar de um aplicativo',
      notifications: 'Notificações',
      devices: 'Dispositivos',
      password: 'Senha',
      twoStep: 'Login em duas etapas',
    },
    metadataLanguage: {
      help: 'O idioma dos nomes, sinopses e imagens dos títulos nos seus aplicativos, quando um dos complementos do servidor os descreve nesse idioma; os títulos que nenhum complemento descreve nele mantêm a descrição do servidor. Gêneros, classificações etárias e pessoas continuam iguais para todos. Os nomes que o próprio Polyfin cria, como os das temporadas, seguem esse idioma quando o Polyfin o fala: inglês, francês, alemão, espanhol, italiano ou português.',
      label: 'Idioma dos metadados',
      server: 'O do servidor',
      noLanguage:
        'Nenhum complemento do servidor tem um idioma ainda, então os títulos são descritos no idioma do servidor. Um administrador define o idioma dos complementos em Conteúdo › Fontes.',
      saved: 'Idioma dos metadados salvo.',
    },
    homeRows: {
      help: 'As fileiras que o administrador adiciona para você à página inicial do player web. Desative uma para tirá-la da sua página inicial, e também das suas bibliotecas em outros aplicativos quando ela for uma biblioteca neles. As seções próprias do player web ficam em Configurações › Início dele.',
      empty: 'Nenhuma fileira do início',
      emptyHelp:
        'O administrador não adicionou nenhuma fileira para você à página inicial do player web.',
      shown: (name: string) => `Mostrar ${name} na sua página inicial`,
      on: (name: string) => `${name} voltou à sua página inicial.`,
      off: (name: string) => `${name} foi removida da sua página inicial.`,
    },
    watching: {
      resumeHelp:
        'Os títulos em andamento no seu Continuar assistindo, para os aplicativos que não conseguem remover um. Remover tira um título sem alterar se ele foi reproduzido. Uma importação de um serviço de acompanhamento ou de um aplicativo só o traz de volta com um ponto de retomada mais recente.',
      hiddenHelp:
        'Os títulos que você ocultou das suas bibliotecas, fileiras do início, buscas e títulos semelhantes. Eles ainda abrem a partir de um link. No player web, o menu ⋮ de um título o oculta.',
      remove: 'Remover',
      removeLabel: (name: string) => `Remover ${name} de Continuar assistindo`,
      removed: (name: string) => `${name} foi removido de Continuar assistindo.`,
      resumeEmpty: 'Nada em andamento',
      resumeEmptyHelp: 'Os filmes e episódios que você começar aparecem aqui até você terminá-los.',
      progress: (position: string, runtime: string) => `${position} de ${runtime}`,
      showAgain: 'Mostrar novamente',
      showLabel: (name: string) => `Mostrar ${name} novamente`,
      shown: (name: string) => `${name} voltou às suas listas.`,
      hiddenEmpty: 'Nenhum título oculto',
      hiddenEmptyHelp: 'Oculte um título pelo menu ⋮ dele no player web.',
      hiddenOn: 'Oculto',
      kinds: {
        movie: 'Filme',
        series: 'Série',
        episode: 'Episódio',
        video: 'Vídeo caseiro',
        replay: 'Replay',
      } as Record<string, string>,
    },
    reminders: {
      help: 'Os programas de TV ao Vivo dos quais você pediu um lembrete na página deles no player web. Cada lembrete chega aos seus destinos que escolheram Início de programa, com a antecedência definida em Notificações.',
      empty: 'Nenhum lembrete',
      emptyHelp: 'Na página de um programa futuro no player web, Lembrar-me cria um.',
      everyAiring: (time: string) => `Todas as exibições por volta das ${time}`,
      next: 'Próxima: ',
      noAiring: 'Nenhuma exibição no guia',
      delete: 'Excluir',
      deleteLabel: (name: string) => `Excluir o lembrete de ${name}`,
      deleted: (name: string) => `Lembrete de ${name} excluído.`,
      settingsTitle: 'Lembretes',
      before: 'Lembrar-me com antecedência de',
      beforeHelp: 'Quanto tempo antes do início de um programa o lembrete chega.',
      minutes: (minutes: number) =>
        minutes === 1 ? '1 minuto' : minutes === 60 ? '1 hora' : `${minutes} minutos`,
      onApps: 'Mostrar lembretes também nos meus aplicativos',
      onAppsHelp:
        'Cada lembrete também aparece nos seus aplicativos Jellyfin que aceitam mensagens, como o player web enquanto estiver aberto, do mesmo jeito que Enviar mensagem mostra uma.',
      noTarget:
        'Nenhum dos seus destinos escolheu Início de programa, e seus aplicativos estão desativados: seus lembretes não chegariam a lugar nenhum.',
      saved: 'Configurações de lembretes salvas.',
    },
    importEvery: (minutes: number) =>
      minutes < 60
        ? `a cada ${minutes} minutos`
        : minutes === 60
          ? 'a cada hora'
          : `a cada ${minutes / 60} horas`,
    serverImport: {
      description:
        'Traga seus filmes e episódios reproduzidos, pontos de retomada e favoritos de um servidor Jellyfin ou Emby que você usava antes, ou seus filmes da sua exportação do Letterboxd. Eles vão apenas para a sua conta e só complementam o que o Polyfin já tem. Nada é gravado no outro servidor.',
      serverKind: 'Servidor',
      address: 'Endereço do servidor',
      addressHelp:
        'O endereço em que o servidor abre em um navegador, como http://192.168.1.10:8096.',
      name: 'Seu nome de usuário nesse servidor',
      password: 'Sua senha nesse servidor',
      passwordHelp:
        'Usada apenas para esta importação e nunca guardada. O Polyfin entra com a sua conta, como os aplicativos do servidor fazem, e depois sai. Deixe em branco se a sua conta não tiver senha.',
      start: 'Importar meu histórico',
      starting: 'Iniciando…',
      started: 'A importação foi iniciada. Você pode sair desta página: ela continua.',
      lastTitle: 'Sua última importação',
      from: (server: string) => `De ${server}`,
    },
    letterboxd: {
      option: 'Letterboxd (arquivo de exportação)',
      help: 'No Letterboxd, abra Settings › Import & Export e escolha Export your data: o Letterboxd gera um arquivo ZIP. Escolha-o aqui como está. O Polyfin o lê na memória e nunca o guarda.',
      watchedHelp:
        'Os filmes que você assistiu são marcados como reproduzidos, na data mais recente do seu diário, tantas vezes quanto ele os registra. Cada filme é pesquisado pelo nome e ano nos complementos de metadados do servidor, e mantido quando eles conhecem exatamente um filme com esse nome e ano.',
      noSearch:
        'Nenhum complemento de metadados deste servidor pesquisa filmes: nenhum filme da exportação pode ser encontrado. Peça a um administrador para adicionar um.',
      file: 'Arquivo de exportação',
      fileHelp: 'O arquivo ZIP gerado pelo Letterboxd, de no máximo 20 MB.',
      choose: 'Escolher o arquivo',
      chooseAnother: 'Escolher outro arquivo',
      tooLarge: 'Este arquivo é maior que 20 MB: escolha o arquivo ZIP gerado pelo Letterboxd.',
      watchlist: 'Watchlist para a minha lista “Para assistir”',
      watchlistHelp:
        'Os filmes da sua watchlist do Letterboxd vão para a sua lista “Para assistir” nos aplicativos Jellyfin, até você começar a assisti-los.',
      ratings: 'Avaliações',
      ratingsHelp:
        'Suas estrelas viram sua avaliação de 0 a 10: 3,5 estrelas equivalem a 7. Uma avaliação que você já tem no Polyfin é mantida.',
      likes: 'Filmes curtidos como favoritos',
      likesHelp: 'Os filmes que você curtiu no Letterboxd viram seus favoritos.',
      forUser: {
        watchedHelp:
          'Os filmes que a pessoa assistiu são marcados como reproduzidos, na data mais recente do diário dela, tantas vezes quanto ele os registra. Cada filme é pesquisado pelo nome e ano nos complementos de metadados do servidor, e mantido quando eles conhecem exatamente um filme com esse nome e ano.',
        watchlist: 'Watchlist para a lista “Para assistir” da pessoa',
        watchlistHelp:
          'Os filmes da watchlist do Letterboxd da pessoa vão para a lista “Para assistir” dela nos aplicativos Jellyfin, até ela começar a assisti-los.',
        ratingsHelp:
          'As estrelas da pessoa viram a avaliação dela de 0 a 10: 3,5 estrelas equivalem a 7. Uma avaliação que ela já tem no Polyfin é mantida.',
        likesHelp: 'Os filmes que a pessoa curtiu no Letterboxd viram favoritos dela.',
      },
      start: 'Importar do Letterboxd',
      starting: 'Enviando…',
      started: 'A importação foi iniciada. Você pode sair desta página: ela continua.',
      currentTitle: 'Importação em andamento',
      lastTitle: 'Última importação',
      ownLastTitle: 'Sua última importação',
      from: 'De uma exportação do Letterboxd',
      startedLabel: 'Iniciada',
      endedLabel: 'Concluída',
      startedBy: (name: string) => `Iniciada por ${name}`,
      states: { running: 'Em andamento', done: 'Concluída', failed: 'Falhou' },
      problem:
        'O Polyfin encontrou um erro durante a importação: o registro do servidor tem mais detalhes. O que foi importado é mantido.',
      runningHelp:
        'Você pode sair desta página: a importação continua. Se o Polyfin reiniciar nesse meio-tempo, a importação para: o que foi importado é mantido, e importar novamente não adiciona nada duas vezes.',
      searching: (searched: number, movies: number) =>
        `Pesquisando os filmes: ${searched} de ${movies}.`,
      searchingLabel: 'Filmes pesquisados',
      summary: (movies: number, added: string) =>
        `${movies === 1 ? '1 filme' : `${movies} filmes`}: ${added}.`,
      played: (count: number) =>
        count === 1 ? `${count} marcado como reproduzido` : `${count} marcados como reproduzidos`,
      addedToWatchlist: (count: number) =>
        count === 1
          ? `${count} adicionado a “Para assistir”`
          : `${count} adicionados a “Para assistir”`,
      rated: (count: number) => (count === 1 ? `${count} avaliado` : `${count} avaliados`),
      favorites: (count: number) =>
        count === 1 ? `${count} marcado como favorito` : `${count} marcados como favoritos`,
      unmatched: (count: number) =>
        count === 1 ? '1 filme não encontrado' : `${count} filmes não encontrados`,
      unmatchedTitle: 'Filmes não encontrados',
      unmatchedHelp:
        'Estes filmes não foram importados. Cada um é pesquisado pelo nome e ano, e mantido apenas quando os complementos de metadados do servidor conhecem exatamente um filme com esse nome e ano. Abra-os no Letterboxd para verificá-los.',
      movieColumn: 'Filme',
      reasonColumn: 'Motivo',
      onLetterboxd: 'No Letterboxd',
      reasons: {
        not_found: 'Não encontrado',
        ambiguous: 'Vários filmes com esse nome e ano',
        search_failed: 'Falha na pesquisa',
      },
    },
    appImport: {
      description:
        'Traga o que você assistiu em um aplicativo baseado em complementos: os filmes e episódios reproduzidos com as datas, seus pontos de retomada e a biblioteca do aplicativo, que vai para a sua lista “Para assistir” nos aplicativos Jellyfin, identificados por IMDb, TMDB ou Kitsu. Eles apenas complementam o que o Polyfin já tem, e nada é gravado no aplicativo.',
      app: 'Aplicativo',
      apps: { nuvio: 'Nuvio', stremio: 'Stremio' },
      backend: 'Backend do Nuvio',
      backendHelp:
        'Deixe em branco para usar o do próprio Nuvio. Um backend auto-hospedado também funciona, no endereço dele, inclusive na rede local.',
      email: 'E-mail',
      password: 'Senha',
      passwordHelp:
        'Usada apenas para ler a conta, e nunca armazenada. O Polyfin entra como os aplicativos do Nuvio fazem, lê a conta e depois sai, a menos que você permaneça conectado.',
      signIn: 'Entrar no Nuvio',
      signingIn: 'Entrando…',
      chooseTitle: 'Escolha o que importar',
      chooseHelp: {
        nuvio:
          'O Nuvio mantém um histórico para cada perfil: escolha qual importar. O Polyfin guarda o que leu por apenas alguns minutos.',
        stremio:
          'O Stremio mantém um histórico para cada perfil, ou um único para a conta inteira sem perfis: escolha qual importar. O Polyfin guarda o que leu por apenas alguns minutos.',
      },
      stayConnected: 'Permanecer conectado',
      stayConnectedHelp: {
        nuvio: (every: string) =>
          `O Polyfin guarda a sessão, criptografada, nunca a sua senha, e importa novamente o perfil escolhido ${every}. Ele apenas lê a conta. Você pode se desconectar a qualquer momento.`,
        stremio: (every: string) =>
          `O Polyfin guarda a chave de autenticação, criptografada, e para um perfil com PIN a sessão desse perfil, nunca o PIN, e importa novamente o perfil escolhido ${every}. Ele apenas lê a conta. Você pode se desconectar a qualquer momento. Os termos do Stremio proíbem extrair os dados dele por meios automatizados: ler a sua conta ${every} torna essa zona cinzenta mais acentuada do que uma importação única.`,
      },
      staysConnected: (every: string) =>
        `Você permanece conectado: este perfil é importado novamente ${every}, da mesma forma. Os complementos são instalados apenas agora.`,
      wholeAccount: 'A conta inteira',
      profile: 'Perfil',
      profileNumber: (id: string) => `Perfil ${id}`,
      profileCounts: (played: number, resumes: number, library: number) =>
        `${played} assistidos, ${resumes === 1 ? '1 ponto de retomada' : `${resumes} pontos de retomada`}, ${library} na biblioteca.`,
      lockedProfile: (name: string) => `${name} (PIN)`,
      pin: 'PIN',
      pinHelp:
        'Este perfil pede o PIN dele. O Polyfin o envia apenas ao Stremio, para abrir o perfil, e nunca o guarda.',
      unlock: 'Abrir este perfil',
      unlocking: 'Abrindo…',
      addons: 'Complementos',
      addonsHelp:
        'Cada complemento marcado se torna um complemento próprio da conta, em Minhas fontes. O endereço dele pode conter configurações e chaves: o Polyfin nunca o exibe por inteiro.',
      addonsOff:
        'Os complementos próprios estão desativados para esta conta: os complementos do aplicativo não são oferecidos.',
      noAddons: 'Este perfil não tem nenhum complemento.',
      back: 'Entrar novamente',
      start: 'Importar este perfil',
      starting: 'Iniciando…',
      started: 'A importação foi iniciada. Você pode sair desta página: ela continua.',
      currentTitle: 'Importação em andamento',
      lastTitle: 'Última importação',
      ownLastTitle: 'Sua última importação',
      from: (app: string, profile: string) =>
        profile === '' ? `Do ${app}` : `Do ${app}, perfil ${profile}`,
      startedLabel: 'Iniciada',
      endedLabel: 'Concluída',
      startedBy: (name: string) => `Iniciada por ${name}`,
      states: { running: 'Em andamento', done: 'Concluída', failed: 'Falhou' },
      problem:
        'O Polyfin encontrou um erro durante a importação: o registro do servidor traz mais detalhes. O que foi importado é mantido.',
      runningHelp:
        'Você pode sair desta página: a importação continua. Se o Polyfin reiniciar nesse meio-tempo, a importação para: o que foi importado permanece, e importar novamente não adiciona nada em duplicidade.',
      read: (count: number) => (count === 1 ? '1 título lido.' : `${count} títulos lidos.`),
      counts: (read: number, played: number, resumed: number, watchlist: number) =>
        `${read === 1 ? '1 título lido' : `${read} títulos lidos`}: ${played === 1 ? '1 marcado como reproduzido' : `${played} marcados como reproduzidos`}, ${resumed === 1 ? '1 ponto de retomada' : `${resumed} pontos de retomada`}, ${watchlist === 1 ? '1 adicionado' : `${watchlist} adicionados`} a “Para assistir”.`,
      unmatched: (count: number) =>
        count === 1 ? '1 título não encontrado' : `${count} títulos não encontrados`,
      unmatchedTitle: 'Títulos não encontrados',
      unmatchedHelp:
        'Estes títulos não foram importados: o aplicativo os identifica por um identificador que o Polyfin não reconhece, ou nenhum título do Polyfin o possui. No caso de uma série, o aplicativo também pode indicar os episódios assistidos pela posição deles em uma lista de episódios que os complementos do Polyfin organizam de outra forma.',
      titleColumn: 'Título',
      kindColumn: 'Tipo',
      reasonColumn: 'Motivo',
      reasons: {
        other_identifier: 'Não é um identificador IMDb, TMDB ou Kitsu',
        not_found: 'Não está no Polyfin',
        episode_list:
          'Os episódios não correspondem à lista do Polyfin: nenhum foi marcado como reproduzido',
      },
      addonInstalled: 'adicionado',
      addonWaiting: 'aguardando',
      addonProblems: {
        not_allowed: 'não adicionado: os complementos próprios foram desativados',
        exists: 'já instalado',
        invalid_address: 'não adicionado: não é o endereço de um complemento',
        private_network:
          'não adicionado: está em uma rede local, que só a importação de um administrador alcança',
        unreachable: 'não adicionado: o complemento não respondeu',
        invalid_manifest: 'não adicionado: não é um complemento',
        internal: 'não adicionado: o Polyfin encontrou um erro',
      },
      stremio: {
        way: 'Entrar com',
        ways: {
          link: 'Um código de vinculação',
          password: 'E-mail e senha',
          key: 'Uma chave de autenticação',
        },
        linkHelp:
          'O Polyfin pede um código ao Stremio, como fazem os aplicativos de TV do Stremio. Você o aprova em um navegador em que esteja conectado ao Stremio: isso funciona seja qual for a forma de login da sua conta, incluindo Facebook e Apple, e o Polyfin nunca vê sua senha.',
        getCode: 'Obter um código de vinculação',
        gettingCode: 'Solicitando ao Stremio…',
        codeLabel: 'Código de vinculação',
        approve:
          'Abra o link em um navegador em que esteja conectado ao Stremio, confira se ele mostra este código e aprove-o.',
        openLink: 'Abrir o link',
        waiting:
          'Aguardando sua aprovação. O código vale por 5 minutos; depois que você o aprovar, o Polyfin lê a conta e depois sai, a menos que você permaneça conectado.',
        cancel: 'Cancelar',
        email: 'E-mail',
        password: 'Senha',
        passwordHelp:
          'Usada apenas para ler a conta e nunca armazenada. O Polyfin entra como os aplicativos do Stremio fazem, lê a conta e depois sai, a menos que você permaneça conectado. Uma conta criada com Facebook ou Apple não tem senha: use um código de vinculação.',
        signIn: 'Entrar no Stremio',
        signingIn: 'Entrando…',
        key: 'Chave de autenticação',
        keyHelp:
          'A chave que o aplicativo web do Stremio guarda depois que você entra: no console do seu navegador, nesse aplicativo, JSON.parse(localStorage.getItem("profile")).auth.key a retorna. Usada para ler a conta e armazenada apenas se você permanecer conectado. O Polyfin nunca a desconecta, pois isso desconectaria o seu navegador.',
        readKey: 'Ler a conta',
        reading: 'Lendo…',
      },
      connections: {
        title: 'Aplicativos conectados',
        help: (every: string) =>
          `Importados ${every}, da mesma forma que na primeira importação: o mesmo perfil, sem os complementos. O Polyfin apenas os lê.`,
        connectedAs: 'Conectado como ',
        connected: 'Conectado',
        connectedOn: (date: string) => ` desde ${date}`,
        connectedBy: (name: string) => ` por ${name}`,
        status: {
          connected: 'Conectado',
          importing: 'Importando',
          paused: 'Pausado',
          stopped: 'Entrar novamente',
        },
        never: 'Ainda não importado.',
        lastImport: 'Última importação ',
        colon: ': ',
        nextImport: 'Próxima importação ',
        paused:
          'O servidor não permite mais que os usuários importem o histórico de reprodução: esta conexão fica pausada até que isso seja permitido novamente.',
        stopped: (app: string) =>
          `O ${app} recusou o login que o Polyfin mantinha: nada será importado até que você entre novamente abaixo, com “Permanecer conectado” marcado. Desconectar faz o Polyfin esquecê-lo.`,
        problems: {
          unreachable: (app: string) =>
            `O ${app} não respondeu na última importação. A próxima tentará novamente.`,
          rate_limited: (app: string) =>
            `O ${app} pediu ao Polyfin que aguardasse na última importação. A próxima tentará novamente.`,
          unexpected: (app: string) =>
            `O ${app} respondeu à última importação de uma forma que esta versão do Polyfin não consegue ler. A próxima tentará novamente.`,
          profile_missing: (app: string) =>
            `O perfil importado não está mais na conta do ${app}. Entre novamente para escolher outro.`,
          profile_locked: (app: string) =>
            `O perfil importado agora pede um PIN, ou outro diferente, no ${app}. Entre novamente para escolhê-lo com o PIN dele.`,
          sign_in_refused: (app: string) => `O ${app} recusou o login que o Polyfin mantinha.`,
          internal: () =>
            'O Polyfin encontrou um erro durante a última importação: o registro do servidor traz mais detalhes. A próxima tentará novamente.',
        },
        importNow: 'Importar agora',
        importing: 'Importando…',
        disconnect: 'Desconectar',
        disconnectTitle: (app: string) => `Desconectar ${app}?`,
        disconnectBody: (app: string) =>
          `O Polyfin para de importar esta conta do ${app} e esquece o que guardava para lê-la, depois de encerrar a sessão que abriu. Uma chave de autenticação que você colou é apenas esquecida. O que foi importado permanece.`,
        disconnected: (app: string) => `${app} desconectado.`,
      },
    },
    sharedLinksHelp:
      'Compartilhe um filme ou um episódio com alguém que não tem conta: a pessoa assiste no navegador por meio de um link que expira, e isso conta entre as suas reproduções simultâneas.',
    statisticsHelp:
      'O que você assistiu neste servidor: horas, títulos, aplicativos e dispositivos, e suas reproduções recentes.',
    notificationsHelp:
      'Onde o Polyfin avisa sobre novos episódios das séries que você acompanha, suas gravações e reproduções, suas conexões que caem, os bloqueios da sua conta e os logins dela em novos dispositivos, lembra você dos programas que escolheu e envia seu resumo semanal e suas retrospectivas do mês e do ano.',
    devicesHelp:
      'Os aplicativos conectados à sua conta. Um dispositivo que você desconectar pedirá sua senha na próxima vez que for aberto.',
    passwordHelp: 'Alterar sua senha desconecta todos os seus dispositivos Jellyfin.',
    currentPassword: 'Senha atual',
    newPassword: 'Nova senha',
    confirmPassword: 'Confirmar nova senha',
    changePassword: 'Alterar senha',
    changingPassword: 'Alterando…',
    passwordChanged: 'Sua senha foi alterada.',
    twoStep: {
      help: 'Depois da sua senha, este aplicativo de administração pede um código de um aplicativo autenticador no seu celular. Os aplicativos Jellyfin não conseguem pedir esse código: depois que o recurso é ativado, eles entram com o Quick Connect, que você aprova aqui, e sua senha sozinha não permite mais entrar neles. Os aplicativos e navegadores já conectados continuam conectados.',
      off: 'Desativado',
      on: 'Ativado',
      offText: 'Sua senha sozinha permite que você entre.',
      onText: 'Você entra com sua senha e, em seguida, um código do seu aplicativo autenticador.',
      required:
        'Os administradores precisam usar o login em duas etapas neste servidor. Ative-o agora, ou isso será solicitado no seu próximo login.',
      requiredOn:
        'Os administradores precisam usar o login em duas etapas neste servidor: ele continua ativado.',
      unreadable:
        'A POLYFIN_SECRET_KEY não consegue mais ler a chave do seu aplicativo autenticador, como acontece após uma restauração com outra chave: só seus códigos de recuperação permitem que você entre. Desative o login em duas etapas com sua senha e ative-o novamente.',
      turnOn: 'Ativar',
      starting: 'Preparando…',
      scan: 'Escaneie este código QR com seu aplicativo autenticador ou digite a chave manualmente. O próprio Polyfin gerou o código: a chave não vai para nenhum outro lugar.',
      qrLabel: 'Código QR da chave, para seu aplicativo autenticador',
      key: 'Chave',
      openInApp: 'Abrir em um aplicativo autenticador neste dispositivo',
      code: 'Código do aplicativo',
      codeHelp:
        'O código de 6 dígitos que o aplicativo mostra para o Polyfin. Ele muda a cada 30 segundos.',
      confirm: 'Ativar',
      confirming: 'Verificando…',
      turnedOn: 'Login em duas etapas ativado.',
      recoveryTitle: 'Seus códigos de recuperação',
      recoveryHelp:
        'Cada código permite entrar uma vez, no lugar de um código do aplicativo, caso você perca o celular. Guarde-os em um lugar seguro, como um gerenciador de senhas: eles são mostrados só agora.',
      copy: 'Copiar os códigos',
      copied: 'Códigos copiados.',
      copyFailed: 'Não foi possível copiar os códigos: selecione-os e copie-os manualmente.',
      saved: 'Já salvei',
      codesLeft: (n: number) =>
        n === 1 ? 'Resta 1 código de recuperação' : `Restam ${n} códigos de recuperação`,
      newCodes: 'Novos códigos de recuperação',
      newCodesTitle: 'Gerar novos códigos de recuperação?',
      newCodesText: 'Os novos códigos invalidam os anteriores.',
      making: 'Gerando…',
      turnOff: 'Desativar',
      turnOffTitle: 'Desativar o login em duas etapas?',
      turnOffText:
        'Sua senha sozinha passará a permitir que você entre, aqui e nos aplicativos Jellyfin.',
      turningOff: 'Desativando…',
      turnedOff: 'Login em duas etapas desativado.',
      confirmWith: 'Confirmar com',
      withCode: 'Um código',
      withPassword: 'Minha senha',
      codeOrRecovery: 'Código do aplicativo ou código de recuperação',
      password: 'Senha',
    },
    tracking: {
      title: 'Acompanhamento',
      description:
        'Conecte os serviços que acompanham o que você assiste e ouve. O Polyfin informa a cada serviço conectado os filmes e episódios que você assiste, ou as músicas que você reproduz, nos seus aplicativos Jellyfin.',
      loading: 'Carregando serviços de acompanhamento…',
      status: {
        connected: 'Conectado',
        notConnected: 'Não conectado',
        waiting: 'Aguardando o código',
        waitingSignIn: 'Aguardando você',
        reconnect: 'Precisa conectar novamente',
        unreachable: 'Tentando novamente',
        unavailable: 'Não configurado',
        appRefused: 'Aplicativo recusado',
      },
      unavailable: (name: string) =>
        `${name} ainda não está configurado neste servidor: um administrador precisa primeiro adicionar o aplicativo ${name} nas configurações.`,
      setUpApp: (name: string) => `Configurar o aplicativo ${name}`,
      codeIntro: (name: string) =>
        `Conecte sua conta ${name}: o Polyfin fornece um código para você digitar no site do ${name}.`,
      keyIntro: (name: string) => `Conecte sua conta ${name} com sua chave da API do ${name}.`,
      signInIntro: (name: string) =>
        `Conecte sua conta ${name}: entre no site do ${name} e permita que o Polyfin registre (scrobble) as músicas que você reproduz.`,
      tokenIntro: (name: string) =>
        `Conecte sua conta ${name} com seu token de usuário do ${name} para registrar (scrobble) as músicas que você reproduz.`,
      keyLabel: (name: string) => `Chave da API do ${name}`,
      tokenLabel: (name: string) => `Token de usuário do ${name}`,
      savedKeyHelp: 'Só você pode exibi-la. Para usar outra chave, desconecte e conecte novamente.',
      keyHelp: {
        mdblist: 'Ela está em mdblist.com, em Preferences, na seção API key.',
        publicmetadb: 'Ela está na sua conta do PublicMetaDB.',
        listenbrainz: 'Ele está em listenbrainz.org, nas suas configurações, na seção User token.',
      },
      connect: 'Conectar',
      connecting: 'Conectando…',
      checking: 'Verificando a chave…',
      reconnect: 'Conectar novamente',
      newCode: 'Obter um novo código',
      signInAgain: 'Recomeçar',
      enterCode: (site: string) => `Acesse ${site}, entre e digite este código:`,
      codeLabel: 'Código a digitar',
      openSite: (site: string) => `Abrir ${site}`,
      waiting: 'Esta página é atualizada automaticamente depois que você digitar o código.',
      expires: (when: string) => `O código expira ${when}.`,
      codeEnded:
        'O código expirou ou foi recusado antes de ser digitado. Obtenha um novo código para tentar novamente.',
      signInStep: (site: string) =>
        `Acesse ${site}, entre se for solicitado e permita que o Polyfin use sua conta.`,
      signInWaiting: 'Esta página é atualizada automaticamente depois que você der a permissão.',
      signInExpires: (when: string) => `Esta solicitação expira ${when}.`,
      signInEnded:
        'O Polyfin não foi autorizado a tempo ou foi recusado. Recomece para tentar mais uma vez.',
      pinIntro: (name: string) =>
        `Conecte suas listas do ${name}: autorize o Polyfin no site do ${name} e cole aqui o código exibido.`,
      redirectIntro: (name: string) =>
        `Conecte sua lista do ${name}: autorize o Polyfin no site do ${name}, que trará você de volta para cá.`,
      pinStep: (site: string) =>
        `Acesse ${site}, entre se for solicitado, autorize o Polyfin e copie aqui o código exibido.`,
      redirectStep: (site: string) =>
        `Acesse ${site}, entre se for solicitado e autorize o Polyfin: você voltará para cá ao concluir.`,
      codeField: (name: string) => `Código do ${name}`,
      codeFieldHelp: (name: string) =>
        `O código que o ${name} exibe depois que você autoriza o Polyfin.`,
      addressField: 'Endereço ou código',
      addressFieldHelp: (name: string) =>
        `Se o seu navegador não conseguiu abrir a página para a qual o ${name} o enviou, copie aqui o endereço dessa página da barra de endereços.`,
      pasteEnded:
        'Esta solicitação expirou antes de o Polyfin ser autorizado. Recomece para tentar mais uma vez.',
      invalidCode: (name: string) =>
        `O ${name} não aceitou este código, ou ele é de uma solicitação anterior. Recomece para obter um novo.`,
      tokenExpires: (date: string) => `, até ${date}`,
      returning: (name: string) => `Conectando sua conta ${name}…`,
      connectedAs: 'Conectado como ',
      connectedWithKey: 'Conectado com sua chave da API',
      connected: 'Conectado',
      connectedOn: (date: string) => ` em ${date}`,
      lastSent: ', último envio ',
      nothingSent: ', nada enviado ainda',
      connectedToast: (name: string) => `${name} conectado.`,
      disconnectedToast: (name: string) => `${name} desconectado.`,
      problemReconnect: (name: string) =>
        `O ${name} não aceita mais esta conexão, então nada é enviado a ele. Conecte novamente para voltar a enviar o que você assiste.`,
      problemUnreachable: (name: string) =>
        `Não foi possível acessar o ${name} recentemente. O Polyfin continua tentando e enviará o que você assistiu assim que ele responder.`,
      disconnect: 'Desconectar',
      disconnectTitle: (name: string) => `Desconectar ${name}?`,
      disconnectBody: (name: string) =>
        `O Polyfin deixará de informar o que você assiste. O que já foi enviado permanece no ${name}.`,
      invalidKey: (name: string) =>
        `O ${name} não aceitou esta chave. Copie-a novamente da sua conta ${name}.`,
      invalidToken: (name: string) =>
        `O ${name} não aceitou este token. Copie-o novamente das suas configurações do ${name}.`,
      serviceUnreachable: (name: string) =>
        `Não foi possível acessar o ${name}. Tente novamente em alguns minutos.`,
      notAvailable: (name: string) =>
        `${name} não está configurado neste servidor: um administrador precisa primeiro adicionar o aplicativo ${name}.`,
      appRefused: (name: string) =>
        `${name} recusou o aplicativo deste servidor. Um administrador precisa verificar as credenciais em Configurações › Acompanhamento.`,
      history: {
        toggle: (name: string) => `Importar meu histórico do ${name}`,
        toggleHelp: (name: string, every: string) =>
          `Marca como reproduzido no Polyfin o que você assistiu no ${name} e retoma de onde você parou, para que Continuar assistindo e A seguir fiquem iguais. Importado ${every}; nada é enviado de volta ao ${name}.`,
        listsToggle: (name: string) => `Importar minhas listas do ${name}`,
        listsToggleHelp: (name: string, every: string) =>
          `Marca como reproduzidos no Polyfin os episódios que suas listas do ${name} contam como assistidos: até o progresso de cada entrada em Watching, On hold e Dropped, e todos em Completed. Importado ${every}; a importação só adiciona.`,
        listCounts: (played: number, unmapped: number) =>
          `${played === 1 ? '1 título marcado' : `${played} títulos marcados`} como reproduzido${played === 1 ? '' : 's'}, ${unmapped === 1 ? '1 título não encontrado' : `${unmapped} títulos não encontrados`} no Polyfin.`,
        importNow: 'Importar agora',
        importing: 'Importando…',
        importingStatus: 'Importando seu histórico…',
        never: 'Ainda não importado.',
        lastImport: 'Última importação',
        colon: ': ',
        nextImport: 'Próxima importação',
        counts: (played: number, resumed: number, unmapped: number) =>
          `${played === 1 ? '1 título marcado' : `${played} títulos marcados`} como reproduzido${played === 1 ? '' : 's'}, ${resumed === 1 ? '1 ponto de retomada' : `${resumed} pontos de retomada`}, ${unmapped === 1 ? '1 título não encontrado' : `${unmapped} títulos não encontrados`} no Polyfin.`,
        problem: {
          reconnect: (name: string) =>
            `${name} recusou a conexão durante a última importação. Conecte-se novamente para importar seu histórico.`,
          unreachable: (name: string) =>
            `Não foi possível ler o ${name} por completo durante a última importação. O que foi lido foi importado; a próxima importação tentará novamente.`,
          rate_limited: (name: string) =>
            `${name} pediu ao Polyfin para aguardar durante a última importação. O que foi lido foi importado; a próxima importação tentará novamente.`,
        },
        watchlist: {
          lists: {
            trakt: 'watchlist',
            simkl: 'lista Plan to Watch',
            anilist: 'lista Plan to Watch',
            myanimelist: 'lista Plan to Watch',
          },
          toggle: (name: string, list: string) => `Importar minha ${list} do ${name}`,
          toggleHelp: (name: string, list: string) =>
            `Coloca os filmes e séries da sua ${list} do ${name} na visualização “Para assistir” dos seus aplicativos Jellyfin, a cada importação. Um título sai dela assim que você começa a assisti-lo ou quando o ${name} deixa de listá-lo. Nada é enviado de volta ao ${name}.`,
          counts: (added: number, unmapped: number) =>
            `${added} adicionado${added === 1 ? '' : 's'} a “Para assistir”, ${unmapped === 1 ? '1 título não encontrado' : `${unmapped} títulos não encontrados`} no Polyfin.`,
          added: (added: number) =>
            `${added} adicionado${added === 1 ? '' : 's'} a “Para assistir”.`,
        },
        ratings: {
          toggle: 'Sincronizar minhas avaliações',
          toggleHelp: (name: string) =>
            `Envia ao ${name} as avaliações que você dá a filmes e séries como suas notas e, a cada importação, aplica suas notas do ${name} aos títulos que você ainda não avaliou. Uma série que o ${name} divide em várias entradas é ignorada.`,
          counts: (rated: number, skipped: number) =>
            `${rated === 1 ? '1 título avaliado' : `${rated} títulos avaliados`}, ${skipped === 1 ? '1 avaliação ignorada' : `${skipped} avaliações ignoradas`}.`,
        },
      },
    },
  },
  shareLinks: {
    share: 'Compartilhar um título',
    off: 'Não é possível criar links de compartilhamento agora: um administrador os desativou para este servidor ou para sua conta. Enquanto isso, os links ainda ativos não são reproduzidos.',
    empty: 'Nenhum link compartilhado',
    emptyHelp:
      '“Compartilhar um título” disponibiliza um filme ou um episódio por meio de um link. No player web, a página de um filme ou de um episódio também tem a opção “Compartilhar”.',
    userEmpty: 'Este usuário não tem nenhum link compartilhado.',
    expiresLabel: 'Expira',
    expiredLabel: 'Expirado',
    revokedLabel: 'Revogado',
    plays: (count: number) => (count === 1 ? '1 reprodução' : `${count} reproduções`),
    viewers: (count: number) =>
      count === 1 ? '1 espectador simultâneo' : `${count} espectadores simultâneos`,
    states: { active: 'Ativo', expired: 'Expirado', revoked: 'Revogado' },
    revoke: 'Revogar',
    revokeLabel: (title: string) => `Revogar o link para ${title}`,
    revokeTitle: 'Revogar este link compartilhado?',
    revokeConfirm:
      'O link deixa de funcionar imediatamente, e quem estiver assistindo por meio dele é interrompido em poucos segundos.',
    revoking: 'Revogando…',
    revoked: 'Link compartilhado revogado.',
    search: 'Pesquisar nas suas bibliotecas',
    searchHelp: 'Um filme, ou uma série para escolher um de seus episódios.',
    searchButton: 'Pesquisar',
    searching: 'Pesquisando…',
    noTitles: 'Nenhum filme ou série encontrado.',
    titlesFound: (term: string) => `Títulos encontrados para ${term}`,
    pickTitle: (name: string, details: string) => `Compartilhar ${name}, ${details}`,
    episodesOf: (series: string) => `Episódios de ${series}`,
    noEpisodes: 'Esta série não tem nenhum episódio que você possa compartilhar.',
    back: 'Voltar aos resultados',
    chosen: 'Título',
    change: 'Alterar',
    expires: 'Expira após',
    expiresHelp: 'Depois disso, ninguém poderá abrir o link. Você pode revogá-lo antes.',
    after: (hours: number) =>
      hours === 1 ? '1 hora' : hours === 24 ? '1 dia' : `${hours / 24} dias`,
    viewersLabel: 'Espectadores simultâneos',
    viewersHelp:
      'Quantas pessoas podem assistir pelo link ao mesmo tempo. Cada uma conta como uma das suas reproduções simultâneas.',
    submit: 'Criar link',
    creating: 'Criando…',
    createdTitle: 'Link criado',
    createdNotice: (title: string) =>
      `Qualquer pessoa com este link pode assistir a ${title} em um navegador, sem conta, até que ele expire. Copie-o agora: ele só é exibido esta vez.`,
    copy: 'Copiar o link',
    copied: 'Link copiado.',
    copyFailed: 'Não foi possível copiar o link. Selecione-o e copie-o manualmente.',
    done: 'Concluído',
  },
  resetPreferences: {
    button: 'Redefinir minhas preferências',
    title: 'Redefinir suas preferências?',
    help: 'O que você mesmo escolheu volta aos padrões do servidor:',
    metadataLanguage: 'Idioma dos metadados: O do servidor',
    homeRows: 'Fileiras do início: todas as fileiras voltam a aparecer',
    appPreferences:
      'As preferências que os apps Jellyfin salvaram na sua conta: idiomas e modo do áudio e das legendas, preferências da tela inicial e de exibição',
    kept: 'O que você assistiu, seus favoritos, seus pontos de retomada, suas conexões e importações continuam.',
    confirm: 'Redefinir',
    resetting: 'Redefinindo…',
    done: 'Suas preferências foram redefinidas.',
  },
}

export default account
