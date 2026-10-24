import type en from '../en/system'

const system: typeof en = {
  apiKeys: {
    title: 'Chaves da API',
    description:
      'As chaves permitem que ferramentas e aplicativos, como gerenciadores de solicitações ou scripts, usem a API Jellyfin deste servidor com direitos de administrador. Qualquer pessoa que tenha uma chave tem esses direitos: mantenha as chaves em sigilo e revogue as que você não usa mais.',
    listTitle: 'Chaves',
    empty: 'Nenhuma chave da API ainda.',
    created: 'Criada',
    lastUsed: 'Último uso',
    createTitle: 'Criar uma chave',
    app: 'Nome do aplicativo',
    appHint: 'Quem vai usar esta chave, para reconhecê-la depois. De 1 a 64 caracteres.',
    create: 'Criar chave',
    creating: 'Criando…',
    newKeyTitle: (app: string) => `Chave para ${app}`,
    newKeyNotice: 'Copie esta chave agora: ela não será exibida novamente.',
    copy: 'Copiar',
    copied: 'Copiada.',
    copyFailed: 'Não foi possível copiar a chave. Selecione-a e copie-a manualmente.',
    done: 'Já copiei',
    revoke: 'Revogar',
    revoking: 'Revogando…',
    revokeConfirm: (app: string) =>
      `Revogar a chave para ${app}? As ferramentas e os aplicativos que a usam deixarão de funcionar. Esta ação não pode ser desfeita.`,
    revoked: (app: string) => `A chave para ${app} foi revogada.`,
    emptyHint:
      'Crie uma abaixo para cada ferramenta ou aplicativo, para que cada uma possa ser revogada separadamente.',
    revokeTitle: 'Revogar esta chave?',
    revokeLabel: (app: string) => `Revogar a chave para ${app}`,
  },
  system: {
    schedule: {
      title: 'Agenda',
      description:
        'As tarefas do servidor, as próximas gravações e as próximas atualizações da TV ao Vivo.',
      tasksTitle: 'Tarefas',
      task: 'Tarefa',
      lastRun: 'Última execução',
      nextRun: 'Próxima execução',
      notRunYet: 'Não executada desde a inicialização do servidor',
      byHandOnly: 'Executada somente manualmente',
      every: (span: string) => `Intervalo: ${span}`,
      daily: (time: string) => `Todos os dias às ${time}, horário do servidor`,
      took: (span: string) => `levou ${span}`,
      results: {
        Completed: 'Concluída',
        Failed: 'Falhou',
        Cancelled: 'Interrompida',
      } as Record<string, string>,
      running: 'Em execução',
      cancelling: 'Interrompendo',
      runNow: 'Executar agora',
      runLabel: (name: string) => `Executar ${name} agora`,
      stop: 'Interromper',
      stopLabel: (name: string) => `Interromper ${name}`,
      started: (name: string) => `${name} iniciada.`,
      autoRefresh: 'Atualiza a cada 5 segundos.',
      recordingsTitle: 'Próximas gravações',
      recordingsOff: 'A gravação está desativada. Ative-a em Configurações › Gravações.',
      noTimers: 'Nenhuma gravação agendada.',
      recordingNow: 'Gravando',
      failed: 'Falhou',
      series: 'Série',
      scheduledBy: (name: string) => `Agendada por ${name}`,
      unknownChannel: 'Canal não encontrado',
      tunersShort: 'Sintonizadores insuficientes',
      tunersShortHelp:
        'Há mais gravações simultâneas nos canais deste sintonizador do que sintonizadores disponíveis: uma delas pode falhar.',
      refreshesTitle: 'Atualizações da TV ao Vivo',
      refreshesHelp: (hours: number) =>
        `As listas de canais IPTV e os guias de programação, tanto os do servidor quanto os dos próprios usuários, são baixados novamente a cada ${hours} h (Configurações › TV ao Vivo), pela tarefa “Atualizar guias da TV ao Vivo”.`,
      noRefreshes: 'Nenhuma fonte IPTV ou guia de programação.',
      channelList: 'Lista de canais',
      iptvLists: 'Listas de canais, filmes e séries',
      guide: 'Guia de programação',
      due: 'Na próxima verificação',
      lastDownload: 'Último download',
      nextDownload: 'Próximo download',
      notYet: 'Ainda não',
      noTasks: 'Nenhuma tarefa.',
      noTasksHint: 'O servidor lista suas tarefas aqui depois de iniciado.',
      recordingsHint: 'As gravações agendadas por um aplicativo ou pelo guia aparecem aqui.',
      refreshesHint:
        'Adicione uma fonte IPTV em Conteúdo › Fontes, ou um guia de programação a um catálogo de TV, e os downloads aparecerão aqui.',
      backupsTitle: 'Backups do banco de dados',
      backUpNow: 'Criar backup agora',
    },
    health: {
      title: 'Integridade',
      noIptv: 'Nenhuma fonte IPTV.',
      owner: { server: 'Servidor', user: (name: string) => `Complementos de ${name}` },
      description:
        'Como estão o servidor e suas fontes, com base no que o Polyfin já registra: esta página não envia nenhuma solicitação a eles.',
      problemsTitle: 'Requer atenção',
      allGood: 'Nenhum problema encontrado.',
      update: (version: string) => `Polyfin ${version} está disponível`,
      updateRunning: (version: string) => `Este servidor executa a versão ${version}.`,
      releaseNotes: 'Notas de versão',
      checkedAt: (time: string) => `Verificado às ${time}. Atualiza a cada 10 segundos.`,
      ok: 'OK',
      warning: 'Aviso',
      error: 'Erro',
      problems: {
        database: 'O banco de dados não responde.',
        disk: (folder: string, free: string) =>
          `Pouco espaço restante para ${folder}: ${free} livres.`,
        addon: (name: string, failure: string) => `${name}: ${failure}`,
        iptv: (name: string) => `${name}: o último download da lista de canais falhou.`,
        tuner: (name: string) => `${name}: o sintonizador não conseguiu reproduzir um canal.`,
        tunerUnreachable: (name: string) => `${name}: o sintonizador não responde.`,
        folder: (name: string) =>
          `${name}: o Polyfin não consegue ler esta pasta local. Verifique se ela está montada e pode ser lida pelo usuário 65532.`,
        share: (name: string) =>
          `${name}: o Polyfin não consegue ler este compartilhamento de rede. Verifique o endereço dele e se o usuário dele tem permissão de leitura.`,
        shareUnreachable: (name: string) =>
          `${name}: o Polyfin não consegue acessar este compartilhamento de rede.`,
        shareRefused: (name: string) =>
          `${name}: o compartilhamento de rede recusa o usuário ou a senha do Polyfin.`,
        shareHostKey: (name: string) =>
          `${name}: o servidor SFTP apresenta uma chave de host diferente da salva, e o Polyfin a recusa até que ela seja aceita.`,
        folderWatchLimit: (name: string) =>
          `${name}: o Polyfin não consegue monitorar alterações nesta pasta local: o limite de pastas monitoradas do sistema foi atingido. Ela continua sendo rastreada conforme o agendamento. Aumente fs.inotify.max_user_watches no host.`,
        folderInstanceLimit: (name: string) =>
          `${name}: o Polyfin não consegue monitorar alterações nesta pasta local: o limite de monitores ou de arquivos abertos do sistema foi atingido. Ela continua sendo rastreada conforme o agendamento. Aumente fs.inotify.max_user_instances no host, ou o limite de arquivos abertos do contêiner.`,
        folderOpenFilesLimit: (name: string) =>
          `${name}: o Polyfin não consegue monitorar alterações nesta pasta local: o limite de arquivos abertos do sistema foi atingido, pois o macOS monitora cada pasta e arquivo por meio de um deles. Ela continua sendo rastreada conforme o agendamento. Aumente kern.maxfilesperproc e kern.maxfiles com sysctl.`,
        folderUnwatched: (name: string) =>
          `${name}: o Polyfin não consegue monitorar alterações nesta pasta local. Ela continua sendo rastreada conforme o agendamento.`,
        shareUnwatchedUnreachable: (name: string) =>
          `${name}: o Polyfin não consegue monitorar alterações neste compartilhamento de rede: ele está inacessível. Ele é rastreado conforme o agendamento e novamente assim que responder.`,
        shareUnwatchedRefused: (name: string) =>
          `${name}: o Polyfin não consegue monitorar alterações neste compartilhamento de rede: ele recusa o usuário ou a senha do Polyfin. Verifique-os e digite-os novamente.`,
        shareUnwatched: (name: string) =>
          `${name}: o Polyfin não consegue monitorar alterações neste compartilhamento de rede. Ele continua sendo rastreado conforme o agendamento.`,
        folderNoSearch: (name: string) =>
          `${name}: nenhum complemento do servidor descreve e pesquisa os títulos dos arquivos, então eles continuam sem associação. Adicione um em Conteúdo › Fontes: os complementos próprios dos usuários, em Minhas fontes, não contam.`,
        folderSearch: (name: string, addon: string, failure: string) =>
          `${name}: não foi possível pesquisar os títulos dos arquivos${addon === '' ? '' : failure === '' ? ` (${addon})` : ` (${addon}: ${failure})`}. Eles continuam sem associação até que uma pesquisa dê certo, o que o Polyfin tenta de novo sozinho, depois de alguns minutos e, então, com menos frequência.`,
        searchLate: (name: string) =>
          `${name}: a pesquisa dele não respondeu a tempo. As pesquisas o deixam de fora por alguns minutos e depois o consultam de novo.`,
        guide: (name: string) => `${name}: a última obtenção do guia de programação falhou.`,
        paused: (host: string) =>
          `Miniaturas pausadas para ${host}: ele pediu para diminuir o ritmo.`,
        unknownProxy: (address: string) =>
          `${address} encaminha requisições (X-Forwarded-For), mas não é um proxy conhecido, então todos os clientes dele contam como ${address}. Se for o seu proxy reverso, adicione-o em Proxies conhecidos, em Configurações › Segurança.`,
        discoveryPort: (port: number) =>
          `O Polyfin não consegue escutar na porta UDP ${port}, então os aplicativos Jellyfin na rede local não o encontram sozinhos. Provavelmente outro programa a está usando, como um servidor Jellyfin no mesmo host. Salvar as configurações tenta novamente.`,
        torrentRoute: (why: string) =>
          `As streams de torrent não são reproduzidas: ${why}. Nada é enviado por outro caminho; verifique a rota em Configurações › Torrents.`,
        torrentWhy: {
          interface_missing: (name: string) => `a interface de rede ${name} não existe`,
          interface_down: (name: string) => `a interface de rede ${name} está inativa`,
          interface_no_address: (name: string) => `a interface de rede ${name} não tem endereço`,
          interface_unsupported: () => 'uma interface de rede só funciona no Linux',
          proxy_unreachable: (name: string) => `o proxy SOCKS5 ${name} não responde`,
          proxy_refused: (name: string) => `o proxy SOCKS5 ${name} recusou o usuário e a senha`,
          wireguard_endpoint: (name: string) =>
            `o servidor WireGuard ${name} não pode ser encontrado`,
          wireguard_down: (name: string) => `o túnel WireGuard para ${name} não pode iniciar`,
          wireguard_handshake: (name: string) =>
            `o servidor WireGuard ${name} não responde ao handshake: ele está inacessível ou uma chave da configuração está errada`,
          no_route: () => 'nenhuma rota está definida',
        },
        torrentUdp: (name: string) =>
          `O proxy SOCKS5 ${name} não oferece UDP ASSOCIATE: os torrents são reproduzidos via TCP, com os trackers UDP e o DHT desligados.`,
        torrentStart: (why: string) =>
          `Um torrent não começou a tempo: ${why}. Verifique se a rota alcança a internet; o registro mostra a resposta de cada tracker no nível de depuração.`,
        torrentStartWhy: {
          no_udp:
            'nenhum peer foi encontrado, e a rota não transporta UDP, de que a DHT e os trackers UDP precisam',
          refused:
            'os trackers recusaram o anúncio ou não responderam, e a DHT não encontrou nenhum peer',
          no_peers: 'os trackers e a DHT não encontraram nenhum peer',
          silent: 'foram encontrados peers, mas nenhum o enviou',
        },
        torrentStartPeers: (found: number, why: string) =>
          `${found} ${found === 1 ? 'peer encontrado' : 'peers encontrados'}: ${why}`,
        torrentPeerWhy: {
          connected: (n: number) => `${n} ${n === 1 ? 'conectado' : 'conectados'}`,
          timedOut: (n: number) => `${n} sem resposta a tempo`,
          refused: (n: number) => `${n} ${n === 1 ? 'recusou' : 'recusaram'} a conexão`,
          unreachable: (n: number) => `${n} ${n === 1 ? 'inacessível' : 'inacessíveis'}`,
          proxy: (n: number) => `${n} ${n === 1 ? 'recusado' : 'recusados'} pelo proxy`,
          closed: (n: number) => `${n} ${n === 1 ? 'fechou' : 'fecharam'} a conexão no handshake`,
          left: (n: number) =>
            `${n} ${n === 1 ? 'conectado, depois saiu' : 'conectados, depois saíram'}`,
          other: (n: number) => `${n} ${n === 1 ? 'falhou' : 'falharam'} por outro motivo`,
          untried: (n: number) => `${n} ainda não ${n === 1 ? 'contatado' : 'contatados'}`,
        },
        dlnaPort: (port: number) =>
          `O Polyfin não consegue escutar na porta UDP ${port}, então os dispositivos DLNA na rede local não o encontram sozinhos. Provavelmente outro programa a está usando, como outro servidor DLNA no mesmo host. Salvar as configurações tenta novamente.`,
        discoveryAddress: (address: string) =>
          `Os aplicativos Jellyfin que procuram servidores na rede local recebem ${address}, um endereço do contêiner do Polyfin que eles não conseguem acessar. Defina Endereço na rede local em Configurações › Geral.`,
        proxyNameUnresolved: (name: string) =>
          `O proxy conhecido ${name} não resolve para nenhum endereço, então nenhuma requisição é considerada confiável como vinda dele. Verifique o nome dele em Configurações › Segurança e se o Polyfin compartilha uma rede com ele.`,
        task: (name: string) => `A tarefa “${name}” falhou.`,
        httpsPort: (port: number) =>
          `O Polyfin não consegue escutar na porta TCP ${port}, então o HTTPS não é servido. O HTTP continua funcionando. Provavelmente outro programa a está usando, ou ela é inferior a 1024. Escolha outra porta HTTPS em Configurações › Segurança.`,
        httpsFileMissing: (file: string) =>
          `O arquivo de certificado HTTPS ${file} não existe. O último certificado lido continua em uso.`,
        httpsFilesMismatch:
          'A chave privada HTTPS não corresponde ao certificado. O último certificado lido continua em uso.',
        httpsFileUnreadable: (file: string) =>
          `O Polyfin não consegue ler o arquivo de certificado HTTPS ${file}, ou ele não contém nenhum certificado ou chave PEM que o Polyfin consiga ler. O último certificado lido continua em uso.`,
        httpsRenewal: (error: string) => `O Let’s Encrypt não emitiu o certificado HTTPS: ${error}`,
        httpsExpires: (date: string) => `O certificado HTTPS expira em ${date}.`,
        httpsExpired: (date: string) => `O certificado HTTPS expirou em ${date}.`,
        subtitleOcrLanguage: (name: string, file: string) =>
          `As legendas em imagem em ${name} não são convertidas em texto: o Tesseract não tem dados para este idioma. Adicione o arquivo ${file} à pasta indicada por POLYFIN_TESSDATA_DIR.`,
        metadataLanguages: (name: string) =>
          `${name}: todos os complementos de metadados do servidor têm um idioma, então eles descrevem os títulos no idioma deles para todos os usuários, seja qual for o idioma dos metadados de cada um. Mantenha Idioma das descrições como Nenhum neste complemento, ou em mais um complemento de metadados.`,
        nvidiaDriver: (version: string) =>
          `A placa de vídeo NVIDIA não converte vídeo: o driver dela é anterior à versão ${version}, de que o FFmpeg precisa para o NVENC. Atualize o driver para usar a placa.`,
        backupFailed: 'O último backup do banco de dados falhou.',
        backupStale: 'O último backup do banco de dados tem mais de dois dias.',
        backupDestinationFailed: (name: string) =>
          `${name}: o último envio dos backups do banco de dados falhou.`,
        backupDestinationStale: (name: string) =>
          `${name}: o backup do banco de dados mais recente nesse destino tem mais de dois dias.`,
        conversionsFull:
          'As conversões de vídeo estão no limite: as novas aguardam ou são reproduzidas de outra forma.',
        unreadableSecrets: (names: string) =>
          `Chaves armazenadas não podem ser descriptografadas com POLYFIN_SECRET_KEY e são consideradas não definidas: ${names}. Defina novamente a chave com a qual elas foram criptografadas, ou digite-as novamente.`,
        plaintextSecrets:
          'Chaves e tokens são armazenados sem criptografia. Defina POLYFIN_SECRET_KEY para criptografá-los.',
        appStopped: (app: string, user: string) =>
          `${app} recusou o login que o Polyfin mantinha para ${user}: nada é importado até que ${user} entre novamente em Minha conta, com Manter conectado marcado.`,
        trackingStopped: (service: string, user: string) =>
          `${service} recusou o token ou a chave que o Polyfin mantinha para ${user}: nada é enviado a ele nem importado até que ${user} clique em Conectar novamente em Minha conta › Acompanhamento.`,
      },
      addonsTitle: 'Complementos',
      addonsHelp:
        'Com base nas requisições feitas para os aplicativos desde que o servidor foi iniciado. “Verificar” solicita o manifesto a um complemento uma vez, no máximo uma vez por minuto.',
      noAddons: 'O servidor não tem nenhum complemento.',
      addonStatus: {
        ok: 'Respondendo',
        failing: 'Com falha',
        unknown: 'Nenhuma requisição desde o início',
        off: 'Desativado',
      },
      failures: {
        unreachable: 'não pôde ser acessado',
        timeout: 'não respondeu a tempo',
        private_network: 'está em uma rede local',
        rate_limited: 'pediu para diminuir o ritmo (requisições demais)',
        server_error: 'respondeu com um erro do servidor',
        http_error: 'recusou a requisição',
        invalid_response: 'enviou uma resposta grande demais',
      } as Record<string, string>,
      lastSuccess: 'Última resposta',
      lastFailure: 'Última falha',
      responseTime: 'Tempo de resposta',
      requests: (count: number, failures: number) =>
        `${count} ${count === 1 ? 'requisição' : 'requisições'}, ${failures} com falha`,
      check: 'Verificar',
      checkLabel: (name: string) => `Verificar ${name} agora`,
      checking: 'Verificando…',
      iptvTitle: 'Fontes IPTV',
      guidesTitle: 'Guias de programação',
      sharesTitle: 'Compartilhamentos de rede',
      shareKinds: {
        smb: 'SMB',
        webdav: 'WebDAV',
        s3: 'S3',
        sftp: 'SFTP',
        rclone: 'rclone',
      } as Record<string, string>,
      shareStatus: {
        notified: 'Notificado',
        polled: 'Consultado periodicamente',
        unwatched: 'Não monitorado',
        scans: 'Apenas nos rastreamentos',
      },
      sharePolledEvery: (share: string, minutes: number) =>
        `${share === 's3' ? 'Buckets S3 não informam' : share === 'sftp' ? 'Servidores SFTP não informam' : 'Remotes rclone não informam'} alterações: as pastas da pasta são listadas novamente ${minutes === 1 ? 'a cada minuto' : `a cada ${minutes} minutos`}, conforme suas configurações.`,
      shareScansOnly:
        'Suas configurações pedem para procurar alterações apenas nos rastreamentos: a pasta é lida conforme o agendamento e quando rastreada manualmente.',
      shareNotified:
        'O servidor informa ao Polyfin cada alteração: o compartilhamento é rastreado novamente alguns segundos depois. Suas pastas também são listadas a cada 10 minutos, caso alguma alteração não seja informada.',
      sharePartial:
        'O servidor informa ao Polyfin as alterações feitas pela rede, mas deixou de informar uma em uma pasta do compartilhamento, como uma feita no próprio disco do servidor: as pastas do compartilhamento também são listadas novamente a cada 2 minutos.',
      sharePolled: {
        webdav:
          'Servidores WebDAV não informam alterações: as pastas do compartilhamento são listadas novamente a cada 2 minutos.',
        unsupported:
          'Este servidor SMB não informa alterações: as pastas do compartilhamento são listadas novamente a cada 2 minutos.',
        silent:
          'Este servidor SMB não informou uma alteração encontrada em uma listagem: as pastas do compartilhamento são listadas novamente a cada 2 minutos.',
        failed:
          'O Polyfin perdeu as notificações do servidor e vai solicitá-las novamente em breve. Enquanto isso, as pastas do compartilhamento são listadas novamente a cada 2 minutos.',
        starting:
          'O Polyfin está pedindo ao servidor que informe as alterações. Enquanto isso, as pastas do compartilhamento são listadas novamente a cada 2 minutos.',
      } as Record<string, string>,
      shareUnwatched: {
        unreachable:
          'O compartilhamento não pode ser acessado. Ele é rastreado conforme o agendamento e novamente assim que responder.',
        refused:
          'O compartilhamento recusa o usuário ou a senha do Polyfin. Verifique-os e digite-os novamente.',
        host_key_changed:
          'O servidor SFTP apresenta uma chave de host diferente da salva. Aceite-a em Editar a pasta se a alteração for esperada.',
        failed:
          'O Polyfin não consegue monitorar este compartilhamento. Ele continua sendo rastreado conforme o agendamento.',
      } as Record<string, string>,
      sharesOff:
        'O monitoramento de alterações está desativado nas configurações: os compartilhamentos são rastreados apenas conforme o agendamento.',
      transcoderTitle: 'Transcodificador',
      gpu: 'GPU',
      noGpu: 'Nenhuma: o vídeo é convertido na CPU',
      toneMapping: 'HDR para SDR na GPU',
      cpuEncoders: 'Codificadores de CPU',
      conversions: 'Conversões de vídeo',
      noLimit: 'sem limite',
      remuxes: 'Remuxes',
      maxHeight: 'Convertido até',
      transcodingOff: 'A conversão está desativada nas configurações.',
      databaseTitle: 'Banco de dados',
      reachable: 'Respondendo',
      unreachable: 'Sem resposta',
      size: 'Tamanho',
      cacheTitle: 'Cache das fontes',
      cacheHelp:
        'Partes dos arquivos em reprodução, mantidas em disco até o tamanho definido em Configurações › Reprodução. Um cache cheio é normal: as partes mais antigas abrem espaço.',
      cacheUse: (used: string, limit: string) => `${used} de ${limit}`,
      sourcesOpen: 'Arquivos abertos',
      torrentPieces: 'Dos quais partes de torrents',
      torrentsTitle: 'Torrents',
      torrentsHelp:
        'Os torrents que a reprodução está lendo agora, somente pela rota deles. Um torrent para quando a reprodução dele termina: sem mais pares, sem mais compartilhamento.',
      torrentRoute: {
        interface: 'Interface de rede',
        proxy: 'Proxy SOCKS5',
        wireguard: 'Túnel WireGuard',
        '': 'Nenhuma rota',
      },
      torrentAddresses: 'Endereços do túnel',
      torrentHandshake: 'Último handshake',
      torrentNoHandshake: 'Nenhum ainda',
      torrentsPlaying: 'Torrents em reprodução',
      torrentPeers: 'Pares',
      torrentUTPPeers: 'Pares via uTP',
      torrentsShared: 'Compartilhado desde o início',
      disksTitle: 'Espaço em disco',
      folders: { cache: 'cache', recordings: 'gravações', backups: 'backups' } as Record<
        string,
        string
      >,
      folderTitle: {
        cache: 'Pasta do cache',
        recordings: 'Pasta das gravações',
        backups: 'Pasta dos backups',
      } as Record<string, string>,
      free: (free: string) => `${free} livres`,
      notMeasured: 'Não é possível medir aqui.',
      thumbnailsTitle: 'Miniaturas (trickplay)',
      thumbnailsOff:
        'As miniaturas e as imagens de capítulos estão desativadas em Configurações › Miniaturas (trickplay).',
      waiting: (count: number, length: number) => `${count} de ${length} aguardando`,
      working: 'Criando imagens agora',
      idle: 'Ocioso',
      pausedHosts: 'Fontes pausadas',
      noPausedHosts: 'Nenhuma fonte está pausada.',
      until: (time: string) => `até ${time}`,
      ratingsTitle: 'Avaliações do MDBList',
      ratingsHelp:
        'As solicitações que o MDBList permite por dia à chave de avaliações do servidor, e as que restam até a redefinição diária. Os títulos cujas avaliações não estão guardadas são consultados em lotes, em segundo plano.',
      requestsLeft: 'Solicitações restantes hoje',
      requestsLeftOf: (left: string, limit: string) => `${left} de ${limit}`,
      notToldYet: 'Conhecido na próxima consulta ao MDBList',
      resets: 'Redefinição diária',
      ratingsHeld: 'Nova consulta',
      heldReasons: {
        limit: 'O MDBList limita as solicitações',
        refused: 'O MDBList recusou a chave',
        failed: 'O MDBList não respondeu',
      } as Record<string, string>,
      notHeld: 'Agora',
      titlesWaiting: 'Títulos aguardando',
      titlesKept: 'Títulos com avaliações guardadas',
      backupTitle: 'Backups',
      backupOff: 'Os backups estão desativados. Ative-os em Configurações › Backups.',
      backup: {
        result: 'Última execução',
        none: 'Nenhum backup ainda',
        succeeded: 'Concluído',
        failed: 'Falhou',
        made: 'Último backup feito',
        file: 'Arquivo',
        size: 'Tamanho',
        next: 'Próximo backup',
        runHint:
          'Execute “Fazer backup do banco de dados” na página Agendamento para fazer o backup agora.',
      },
      processTitle: 'Polyfin',
      version: 'Versão',
      uptime: 'Em execução há',
      memory: 'Memória',
      heap: 'Em uso',
      goroutines: 'Goroutines',
      goroutinesDownload: 'Baixar as goroutines',
      goroutinesHelp:
        'Quando uma solicitação nunca recebe resposta, baixe o que o servidor está fazendo e envie com seu relato. Não contém endereços nem segredos.',
      secretsTitle: 'Chaves armazenadas',
      secretsHelp:
        'As chaves do servidor, os tokens e chaves de rastreamento dos usuários e as senhas dos compartilhamentos de rede, criptografados no banco de dados com POLYFIN_SECRET_KEY (openssl rand -base64 32).',
      encryption: 'Criptografia',
      encrypted: 'Ativada',
      notEncrypted: 'Desativada: POLYFIN_SECRET_KEY não está definida',
      storedPlain: 'Armazenadas sem criptografia',
      unreadable: 'Não é possível descriptografar',
      noneUnreadable: 'Nenhuma',
      proxiesTitle: 'Proxies conhecidos',
      proxiesHelp:
        'Os nomes de host listados em Proxies conhecidos, resolvidos a cada alteração das configurações, a cada minuto e assim que um endereço desconhecido encaminha solicitações. Todo endereço para o qual um nome é resolvido é considerado confiável como proxy.',
      resolvesToNothing: 'Não resolve para nenhum endereço',
      secretNames: {
        publicMetaDbKey: 'Chave do PublicMetaDB',
        theIntroDbKey: 'Chave do TheIntroDB',
        mdblistRatingsKey: 'Chave da API do MDBList para avaliações',
        tmdbKey: 'Chave da API do TMDB',
        traktClientSecret: 'Client secret do Trakt',
        lastFmSecret: 'Segredo compartilhado do Last.fm',
        aniListClientSecret: 'Client secret do AniList',
        myAnimeListClientSecret: 'Client secret do MyAnimeList',
        smtpPassword: 'Senha SMTP',
        torrentProxyPassword: 'Senha do proxy SOCKS5 dos torrents',
        torrentWireGuard: 'Configuração do WireGuard dos torrents',
        metricsToken: 'Token de métricas',
      },
      connectionOf: (service: string, user: string) => `conexão ${service} de ${user}`,
      targetOf: (target: string, user: string) => `destino da notificação “${target}” de ${user}`,
      serverTargetOf: (target: string) => `destino da notificação do servidor “${target}”`,
      sharePasswordOf: (folder: string) => `senha ou chave do compartilhamento “${folder}”`,
      destinationSecretOf: (name: string) => `senha ou chave do destino do backup “${name}”`,
      certificateKeys: 'chaves do certificado Let’s Encrypt',
      twoStepOf: (user: string) => `chave do login em duas etapas de ${user}`,
      noAddonsHint: 'Adicione um complemento Stremio em Conteúdo › Fontes.',
      noIptvHint: 'Adicione uma conta IPTV em Conteúdo › Fontes.',
      noGuides: 'Nenhum guia de programação.',
      noGuidesHint: 'Adicione um guia de programação a um catálogo de TV em Conteúdo › TV ao Vivo.',
      serverTitle: 'Servidor',
    },
    logs: {
      title: 'Registros',
      description:
        'As últimas linhas do registro do servidor, mantidas na memória. Os endereços mantêm apenas o host, e senhas, tokens e chaves ficam ocultos.',
      follow: 'Acompanhar',
      pause: 'Pausar',
      level: 'Nível',
      levels: {
        all: 'Todos',
        info: 'Info e acima',
        warn: 'Avisos e erros',
        error: 'Somente erros',
      },
      search: 'Pesquisar',
      download: 'Baixar',
      clear: 'Limpar a exibição',
      empty: 'Nenhuma linha ainda.',
      noMatch: 'Nenhuma linha corresponde.',
      shown: (shown: number, total: number) => `${shown} de ${total} linhas exibidas.`,
      following: 'Acompanhando: novas linhas aparecem a cada 2 segundos.',
      paused:
        'Pausado: as linhas escritas nesse meio-tempo aparecem quando você voltar a acompanhar.',
      detailedHint:
        'As linhas de depuração aparecem quando “Registro detalhado” está ativado, em Configurações › Diagnóstico.',
      region: 'Registro do servidor',
      emptyHint: 'As linhas aparecem aqui à medida que o servidor as escreve.',
      noMatchHint: 'Escolha outro nível ou altere a pesquisa.',
      filters: 'Filtrar o registro',
    },
  },
}

export default system
