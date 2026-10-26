import type en from '../en/settings'
import type { RangeText } from '../en/settings'

const settings: typeof en = {
  settings: {
    tracking: {
      description:
        'Cada usuário pode conectar as próprias contas Trakt, Simkl, MDBList, PublicMetaDB, AniList e MyAnimeList na página Minha conta, e o Polyfin informa a esses serviços o que ele assiste; o Last.fm e o ListenBrainz recebem as músicas que ele ouve. Trakt, Simkl, AniList, MyAnimeList e Last.fm precisam primeiro de um aplicativo deste servidor, configurado aqui. MDBList, PublicMetaDB e ListenBrainz não precisam de nada.',
      traktSetup:
        'Crie um aplicativo em trakt.tv, em Settings, Your API Apps, New application. Dê a ele qualquer nome e copie aqui o client ID e o client secret.',
      redirectUri: 'URI de redirecionamento a informar no aplicativo',
      traktClientId: 'Client ID do Trakt',
      traktClientIdHelp:
        'Deixe vazio para desativar o Trakt. Os usuários podem se conectar quando o client ID e o client secret estiverem salvos.',
      traktClientSecret: 'Client secret do Trakt',
      traktClientSecretHelp: 'Exibido na página do aplicativo em trakt.tv, abaixo do client ID.',
      simklSetup:
        'Crie um aplicativo em simkl.com, em Settings, Developer, Create new app, do tipo “TV, devices & command line” (o AUTH V2 do Simkl; ele não precisa de URI de redirecionamento, e client IDs de aplicativos AUTH V1 mais antigos são recusados). Dê a ele qualquer nome e copie aqui o client ID.',
      simklClientId: 'Client ID do Simkl',
      simklClientIdHelp: 'Deixe vazio para desativar o Simkl. Nenhum client secret é necessário.',
      lastFmSetup:
        'Crie uma conta de API em last.fm, em last.fm/api/account/create. Dê a ela qualquer nome e deixe a callback URL vazia: os usuários autorizam o Polyfin no Last.fm, e o próprio Polyfin solicita a sessão deles ao Last.fm. Depois, copie aqui a chave da API e o shared secret.',
      lastFmApiKey: 'Chave da API do Last.fm',
      lastFmApiKeyHelp:
        'Deixe vazio para desativar o Last.fm. Os usuários podem se conectar quando a chave da API e o shared secret estiverem salvos.',
      lastFmSecret: 'Shared secret do Last.fm',
      lastFmSecretHelp: 'Exibido na página da sua conta de API em last.fm, abaixo da chave da API.',
      aniListSetup:
        'Crie um aplicativo em anilist.co, em Settings, Developer, Create New Client. Dê a ele qualquer nome e, como URL de redirecionamento, o endereço abaixo, onde o AniList mostra aos usuários o código que eles colam no Polyfin. Depois, copie aqui o client ID e o client secret.',
      aniListClientId: 'Client ID do AniList',
      aniListClientIdHelp:
        'Deixe vazio para desativar o AniList. Os usuários podem se conectar quando o client ID e o client secret estiverem salvos.',
      aniListClientSecret: 'Client secret do AniList',
      aniListClientSecretHelp:
        'Exibido na página do aplicativo em anilist.co, abaixo do client ID.',
      myAnimeListSetup:
        'Crie um aplicativo em myanimelist.net, em Account Settings, API, Create ID, do tipo “web”. Dê a ele qualquer nome e, como URL de redirecionamento, o endereço abaixo: o MyAnimeList envia o navegador dos usuários de volta para lá depois que eles autorizam o Polyfin. Apenas o navegador deles precisa alcançá-lo. Depois, copie aqui o client ID e o client secret.',
      myAnimeListRedirectHelp:
        'O endereço público deste servidor em Configurações › Geral, ou então o endereço que você usa agora.',
      myAnimeListClientId: 'Client ID do MyAnimeList',
      myAnimeListClientIdHelp:
        'Deixe vazio para desativar o MyAnimeList. Os usuários podem se conectar quando o client ID e o client secret estiverem salvos.',
      myAnimeListClientSecret: 'Client secret do MyAnimeList',
      myAnimeListClientSecretHelp:
        'Exibido na página do aplicativo em myanimelist.net, abaixo do client ID.',
    },
    title: 'Configurações',
    description: 'Opções que se aplicam a todo o servidor.',
    serverName: 'Nome do servidor',
    serverNameHelp: (r: RangeText) =>
      `De ${r.min} a ${r.max} caracteres, exibido nos aplicativos Jellyfin.`,
    quickConnect: 'Permitir Quick Connect',
    quickConnectHelp:
      'Permite que aplicativos de TV e celular entrem com um código de 6 dígitos aprovado aqui.',
    language: 'Idioma dos nomes gerados',
    languageHelp:
      'O Polyfin nomeia algumas coisas por conta própria nos aplicativos Jellyfin: temporadas (“Temporada 1”, “Especiais”), episódios sem título e o tipo adicionado a bibliotecas que compartilham um nome (“Populares (Filmes)”).',
    legacyAuthorization: 'Permitir autorização legada',
    legacyAuthorizationHelp:
      'Aceita os antigos cabeçalhos X-Emby-*, o parâmetro api_key e o esquema Emby para aplicativos que ainda precisam deles. Desativado por padrão, como no Jellyfin 12.3.',
    updateCheck: 'Verificar novas versões',
    updateCheckHelp:
      'Uma vez por dia, e logo após a inicialização, o Polyfin consulta o GitHub sobre sua versão mais recente, enviando apenas a própria versão. Uma versão mais nova aparece no Início e em Sistema › Integridade, e é enviada às notificações de Nova versão. Desativado, o Polyfin nunca contata o GitHub.',
    restartConfirmed: 'O Polyfin é iniciado de novo quando para',
    restartConfirmedHelp:
      'Ative quando o contêiner do Polyfin tiver uma política de reinício que o inicie de novo, como unless-stopped, always ou on-failure, ou quando o gerenciador de serviços fizer isso. Sem o socket do Docker, que ele nunca recebe, o Polyfin não vê a política de reinício do contêiner: enquanto estiver desativado, Reiniciar no Início avisa que o Polyfin pode não voltar.',
    updateToolUrl: 'Endereço da ferramenta de atualização',
    updateToolUrlHelp:
      'A API de atualização de uma ferramenta que você executa ao lado do Polyfin para atualizar o contêiner, como http://updater:8080/v1/update. Atualizar agora, no Início, envia a ela uma requisição POST sem corpo. Vazio, o botão não aparece.',
    updateToolToken: 'Token da ferramenta de atualização',
    updateToolTokenHelp:
      'Enviado no cabeçalho Authorization: Bearer em cada requisição, se a ferramenta pedir um. Guardado criptografado, como os outros segredos.',
    localDiscovery: 'Responder à descoberta na rede local',
    localDiscoveryHelp:
      'Aplicativos Jellyfin e Emby que procuram servidores na rede local encontram o Polyfin e o listam, sem que seja preciso digitar o endereço. Eles perguntam na porta UDP 7359, e o Polyfin responde apenas a endereços da rede local. Aplica-se imediatamente, sem reinicialização. No Docker, isso exige a rede do host ou a porta 7359/udp publicada: consulte a documentação de instalação.',
    localAddress: 'Endereço na rede local',
    localAddressHelp:
      'O endereço pelo qual os aplicativos da rede local acessam o Polyfin, como http://192.168.1.10:8096. As respostas de descoberta o informam, e os aplicativos Jellyfin o leem como o endereço local do servidor. Vazio, as respostas informam o endereço da interface de rede que alcança o aplicativo, o que é correto, a menos que o Polyfin rode em uma rede bridge do Docker: nesse caso, informe o endereço do host e a porta do Polyfin.',
    torrents: {
      description:
        'Alguns complementos listam torrents em vez de links. O Polyfin pode reproduzi-los: ele entra no swarm de um torrent apenas enquanto uma reprodução o lê, e mantém todo o tráfego em uma única rota, uma interface de rede, como a de uma VPN, um proxy SOCKS5 ou um túnel WireGuard que o próprio Polyfin executa. Nada é enviado por outro caminho. As partes ficam dentro do Espaço em disco para arquivos em leitura, em Configurações › Reprodução.',
      play: 'Reproduzir streams de torrent',
      playHelp:
        'Desativado por padrão. Reproduzir um torrent o baixa de outros peers e compartilha o que foi baixado enquanto o limite de upload permitir: você responde pelo que este servidor baixa e compartilha. Os usuários também precisam de Pode reproduzir streams de torrent, na página deles em Usuários. Requer uma rota.',
      route: 'Rota',
      routeHelp:
        'Todo socket de torrent, para peers, trackers e a DHT, passa apenas por aqui. Quando ela está fora do ar, as versões torrent são ignoradas, inclusive durante uma reprodução, e Sistema › Integridade informa o motivo.',
      routeHelpNoInterface:
        'Todo socket de torrent, para peers, trackers e a DHT, passa apenas por aqui. Este sistema não vincula sockets a uma interface de rede, o que apenas o Linux faz, como no Polyfin Desktop para Windows e macOS: um proxy SOCKS5 ou um túnel WireGuard leva os torrents. Quando ela está fora do ar, as versões torrent são ignoradas, inclusive durante uma reprodução, e Sistema › Integridade informa o motivo.',
      routes: {
        interface: 'Interface de rede',
        proxy: 'Proxy SOCKS5',
        wireguard: 'Túnel WireGuard',
      },
      interface: 'Interface de rede para torrents',
      interfaceHelp:
        'O nome dela, como wg0. Todo socket de torrent é vinculado a ela, e os nomes dos trackers são resolvidos por ela. Apenas Linux. No Docker, o contêiner precisa ver a interface: consulte a documentação de torrents.',
      proxy: 'Proxy SOCKS5 para torrents',
      proxyHelp:
        'O endereço e a porta dele, como 10.0.0.2:1080. Peers e trackers HTTP passam pelo CONNECT dele. Trackers UDP, a DHT e o uTP precisam do UDP ASSOCIATE dele, e ficam desativados sem ele. Por um proxy, nenhum peer consegue se conectar de fora.',
      proxyUser: 'Usuário do proxy',
      proxyUserHelp: 'Vazio quando o proxy não pede nenhum.',
      proxyPassword: 'Senha do proxy',
      proxyPasswordHelp: 'Armazenada criptografada quando POLYFIN_SECRET_KEY está definida.',
      wireGuard: 'Configuração do WireGuard',
      wireGuardHelp:
        'Cole o .conf que seu provedor de VPN fornece para o WireGuard. O Polyfin executa o túnel sozinho, apenas para os torrents, sem nada para configurar neste sistema. Os nomes dos trackers são resolvidos pelo túnel com os servidores DNS dele, nunca de outra forma. Nenhum peer pode se conectar de fora. Armazenada criptografada quando POLYFIN_SECRET_KEY está definida, e nunca mais exibida.',
      wireGuardSaved: (endpoint: string, addresses: string) =>
        `Uma configuração está salva: servidor ${endpoint}, endereços ${addresses}.`,
      wireGuardDns: (servers: string) =>
        `Os nomes dos trackers são resolvidos por ${servers}, pelo túnel.`,
      wireGuardNoDns:
        'Ela não indica nenhum servidor DNS: os trackers indicados por um nome de host não são alcançados, os indicados por um endereço são.',
      wireGuardReplace: 'Cole outra configuração para substituir a salva.',
      wireGuardInvalid: (problem: { line: number; key: string; reason: string }) => {
        const where = problem.line > 0 ? `Linha ${problem.line}: ` : ''
        const expected: Record<string, string> = {
          PrivateKey: 'uma chave de 32 bytes em base64',
          PublicKey: 'uma chave de 32 bytes em base64',
          PresharedKey: 'uma chave de 32 bytes em base64',
          Address: 'endereços IP, como 10.2.0.2/32, separados por vírgulas',
          DNS: 'endereços IP separados por vírgulas',
          MTU: 'um número de 576 a 65535',
          ListenPort: 'uma porta de 0 a 65535',
          Endpoint: 'um nome de host ou um endereço IP com uma porta, como vpn.example.com:51820',
          AllowedIPs:
            'endereços IP com o comprimento do prefixo, como 0.0.0.0/0, separados por vírgulas',
          PersistentKeepalive: 'off, ou um número de segundos de 0 a 65535',
        }
        switch (problem.reason) {
          case 'too_long':
            return 'A configuração do WireGuard deve ser um texto de no máximo 8192 bytes.'
          case 'syntax':
            return `${where}cada linha deve ser uma [seção], uma Chave = Valor ou um comentário #.`
          case 'outside':
            return `${where}${problem.key} vem antes de [Interface] ou [Peer].`
          case 'unknown':
            return `${where}${problem.key} não faz parte de uma configuração do WireGuard.`
          case 'repeated':
            return problem.key === 'Peer'
              ? `${where}só uma seção [Peer] é suportada.`
              : `${where}${problem.key} aparece duas vezes.`
          case 'missing':
            return `${where}${problem.key} está faltando na configuração do WireGuard.`
          case 'no_ipv4':
            return `${where}Address precisa de um endereço IPv4, usado pelos trackers UDP e pela DHT.`
          case 'not_all':
            return `${where}AllowedIPs precisa de 0.0.0.0/0: os peers de um torrent estão em qualquer lugar.`
          default:
            return `${where}${problem.key} deve ser ${expected[problem.key] ?? 'válido'}.`
        }
      },
      downloadLimit: 'Limite de download (Mbps, 0 = nenhum)',
      downloadLimitHelp: (r: RangeText) => `De 0 a ${r.max}; ${r.default} por padrão.`,
      uploadLimit: 'Limite de upload (Mbps, 0 = sem compartilhamento)',
      uploadLimitHelp: (r: RangeText) =>
        `Os peers favorecem quem compartilha de volta, então o início pode ser mais lento com 0. O compartilhamento para quando a reprodução termina. De 0 a ${r.max}; ${r.default} por padrão.`,
      atOnce: 'Torrents ao mesmo tempo (0 = sem limite)',
      atOnceHelp: (r: RangeText) =>
        `Quantos torrents podem rodar ao mesmo tempo, para todos os usuários juntos. No limite, um título reproduz outra de suas versões que não seja um torrent, ou é informado como impossível de reproduzir por enquanto; uma vaga é liberada quando um torrent para, 30 segundos após sua reprodução, ou na hora quando o mesmo dispositivo ou usuário reproduz outro título. De 0 a ${r.max}; ${r.default} por padrão.`,
    },
    dlna: {
      title: 'DLNA',
      description:
        'TVs, receivers, consoles e players da rede local que não têm o aplicativo Jellyfin podem navegar pelas bibliotecas e reproduzi-las via DLNA. Um dispositivo não pode entrar: cada um mostra as bibliotecas do usuário escolhido para ele, com os limites desse usuário.',
      share: 'Compartilhar bibliotecas via DLNA',
      shareHelp:
        'Anuncia o Polyfin aos dispositivos da rede local na porta UDP 1900 e responde somente a eles. Aplica-se na hora, sem reiniciar. No Docker, é preciso usar a rede do host ou uma rede que dê ao contêiner seu próprio endereço: consulte a documentação de DLNA.',
      name: 'Nome na rede',
      nameHelp: 'O nome com que os dispositivos listam o Polyfin. Se vazio, o nome do servidor.',
      newDevicesUser: 'Mostrar aos novos dispositivos as bibliotecas de',
      newDevicesUserHelp:
        'O usuário cujas bibliotecas um dispositivo visto pela primeira vez mostra. Com “Ninguém”, um novo dispositivo vê um servidor vazio até que você escolha um usuário para ele abaixo.',
      noOne: 'Ninguém',
      markPlayed: 'Marcar como reproduzido o que os dispositivos DLNA reproduzem',
      markPlayedHelp:
        'Os dispositivos não informam a posição: um título conta como reproduzido assim que o dispositivo lê mais do que “Marcar como reproduzido após (%)” dele. Os dispositivos leem adiantado, então um título pode ser marcado antes da hora.',
      devices: 'Dispositivos',
      devicesEmpty: 'Nenhum dispositivo pediu nada ainda.',
      device: 'Dispositivo',
      address: 'Endereço',
      model: 'Modelo',
      lastSeen: 'Visto por último',
      showsLibrariesOf: 'Mostra as bibliotecas de',
      blocked: 'Bloqueado',
      profile: 'Perfil',
      automaticProfile: (name: string) => `Automático (${name})`,
      forget: 'Esquecer',
      profiles: 'Perfis',
      profilesHelp:
        'Os perfis usam o formato XML do plugin DLNA do Jellyfin. Um dispositivo recebe o primeiro que corresponde ao que ele envia, primeiro os perfis adicionados, senão o genérico.',
      builtin: 'Integrado',
      addProfile: 'Adicionar um perfil',
      replaceProfile: 'Substituir',
      removeProfile: 'Remover',
      profileXml: 'XML do perfil',
      devicesHelp:
        'Os dispositivos da rede local que pediram algo ao Polyfin, o visto mais recentemente primeiro, cada um identificado pelo seu endereço. As alterações se aplicam na hora.',
      devicesLabel: 'Dispositivos DLNA',
      automatic: 'Automático',
      deviceSaved: (name: string) => `${name} salvo.`,
      forgetTitle: (name: string) => `Esquecer ${name}?`,
      forgetBody:
        'O Polyfin esquece o usuário, o bloqueio e o perfil escolhidos para ele. Se ele pedir algo novamente, será um novo dispositivo e mostrará as bibliotecas do usuário escolhido para novos dispositivos.',
      forgotten: (name: string) => `${name} esquecido.`,
      profilesLabel: 'Perfis DLNA',
      added: 'Adicionado',
      viewXml: 'Ver XML',
      profileXmlHelp:
        'Cole o XML de um perfil feito para o plugin DLNA do Jellyfin ou abra o arquivo dele.',
      openFile: 'Abrir um arquivo',
      add: 'Adicionar',
      adding: 'Adicionando…',
      replacing: 'Substituindo…',
      profileAdded: (name: string) => `Perfil ${name} adicionado.`,
      profileReplaced: (name: string) => `Perfil ${name} substituído.`,
      removeTitle: (name: string) => `Remover o perfil ${name}?`,
      removeBody:
        'Os dispositivos aos quais ele é imposto voltam a ter o perfil escolhido a partir do que enviam.',
      profileRemoved: (name: string) => `Perfil ${name} removido.`,
    },
    metrics: 'Expor métricas',
    metricsHelp:
      'Fornece as métricas do Polyfin ao Prometheus em /metrics: reproduções, conversões, solicitações a complementos, importações, notificações, solicitações HTTP, rastreamentos, o banco de dados, o espaço em disco e o processo. Elas não identificam nenhum usuário, título, endereço ou dispositivo. O Prometheus envia o token de métricas em cada solicitação; sem ele, /metrics responde 401 e, enquanto esta opção estiver desativada, 404.',
    metricsToken: 'Token de métricas',
    metricsTokenHelp:
      'Criado pelo Polyfin quando as métricas são ativadas pela primeira vez. O Prometheus o envia como bearer token: consulte a documentação de administração para ver uma configuração de coleta.',
    metricsTokenPending: 'O Polyfin cria o token de métricas quando você salvar.',
    newMetricsToken: 'Criar um novo token',
    newMetricsTokenTitle: 'Criar um novo token de métricas?',
    newMetricsTokenConfirm:
      'O token atual para de funcionar na hora: o Prometheus precisa do novo para ler as métricas.',
    newMetricsTokenMade: 'Novo token de métricas criado',
    legacyWarningTitle: 'Aviso de segurança',
    legacyWarning:
      'Métodos legados podem enviar credenciais em URLs, que acabam ficando em registros, no histórico do navegador e em proxies. Só ative esta opção se um aplicativo que você usa não conseguir entrar de outra forma.',
    prepareAhead: 'Preparar a reprodução com antecedência',
    prepareAheadHelp:
      'O Polyfin lê o arquivo assim que a página de um título é aberta e prepara o próximo episódio perto do fim do atual, para que a reprodução comece na hora. Ele também lista com antecedência as versões dos títulos em Continuar assistindo e A seguir, para que as páginas deles as mostrem na hora. Isso envia algumas solicitações a mais às suas fontes, inclusive para títulos que são abertos, mas não reproduzidos.',
    transcoding: 'Conversão (transcodificação)',
    transcodingHelp:
      'Recodifica vídeo e áudio para aplicativos que não conseguem reproduzir um arquivo como ele é. Quando desativada, os aplicativos reproduzem os arquivos como são ou apenas reempacotados, sem recodificação, e um título que um aplicativo não consegue reproduzir assim não será iniciado nesse aplicativo. Isso inclui gravações convertidas para HEVC, em aplicativos que não reproduzem HEVC.',
    analysisTimeout: 'Tempo máximo para analisar uma versão',
    analysisTimeoutHelp: (r: RangeText) =>
      `Antes de uma primeira reprodução, o Polyfin analisa o arquivo ou o canal para saber como reproduzi-lo. Se a fonte não responder dentro desse número de segundos, o Polyfin desiste dessa versão e passa para a próxima. Um número menor passa adiante mais cedo, mas pode desistir de fontes lentas que teriam funcionado. Um canal é analisado por no máximo 8 segundos. De ${r.min} a ${r.max} segundos; ${r.default} por padrão.`,
    versionAttempts: 'Versões tentadas quando uma não funciona',
    versionAttemptsHelp: (r: RangeText) =>
      `Quando um aplicativo reproduz um título sem escolher uma versão, o Polyfin tenta as versões em ordem até que uma funcione, mas não analisa mais do que esse número. As versões já analisadas também são tentadas, pois não custam nada. Um número maior encontra uma versão que funciona com mais frequência, mas um título que não é reproduzido demora mais para indicar isso. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    preferDirectPlay: 'Preferir versões que o aplicativo reproduz sem conversão',
    preferDirectPlayHelp:
      'Quando um aplicativo reproduz um título sem escolher uma versão, o Polyfin escolhe a primeira versão que o aplicativo reproduz como é ou apenas reempacotada, em vez da primeira que simplesmente reproduz. A primeiríssima reprodução de um título pode ser um pouco mais lenta, pois mais versões podem ser analisadas; nada muda depois que elas são conhecidas.',
    nameHowVersionsPlay: 'Indicar no nome das versões como elas são reproduzidas',
    nameHowVersionsPlayHelp:
      'Os nomes das versões terminam com a forma como o aplicativo reproduz cada uma: “(reprodução direta)”, “(stream direta)” quando copiada sem conversão, “(convertida: motivo)” com HDR, o limite de qualidade, as legendas, o vídeo ou o áudio como motivo, ou “(não reproduzível aqui)”. O Polyfin decide isso pelo perfil de dispositivo que o aplicativo enviou na última reprodução, então os nomes não dizem nada antes da primeira reprodução de um dispositivo nem após uma reinicialização, e nada para uma versão ainda não analisada nem descrita pelo RemuxDB. Os nomes ficam no idioma dos nomes gerados. Nada é lido para isso.',
    versionOrderGroup: 'Ordem das versões',
    versionOrder: {
      label: 'Ordenar versões por',
      help: 'Cada critério agrupa as versões de um filme ou episódio, e o seguinte as ordena dentro de cada grupo; versões empatadas mantêm a ordem dos complementos. A primeira versão é a que “Reproduzir” inicia, e as versões tentadas quando ela não funciona vêm em seguida. O Polyfin se baseia no que sabe de uma versão sem ler o arquivo: sua análise depois de reproduzida, senão a descrição do RemuxDB, senão as palavras dos seus rótulos. Uma versão sobre a qual um critério não sabe nada fica depois das outras. O idioma de áudio preferido é o de cada usuário, definido no aplicativo dele, ou então o Idioma de áudio padrão: primeiro as versões conhecidas nesse idioma, depois as marcadas como MULTI ou DUAL, depois as de idioma desconhecido. Enquanto a página de um título estiver aberta, as versões exibidas mantêm suas posições.',
      none: 'Nenhum: as versões mantêm a ordem dos complementos.',
      add: 'Adicionar critério',
      remove: (name: string) => `Remover ${name}`,
      criterion: (position: number) => `Critério ${position}`,
      criteria: {
        language: 'Idioma de áudio preferido',
        'resolution-high': 'Resolução, da maior para a menor',
        'resolution-low': 'Resolução, da menor para a maior',
        'size-large': 'Tamanho do arquivo, do maior para o menor',
        'size-small': 'Tamanho do arquivo, do menor para o maior',
        codec: 'Codec de vídeo, na ordem abaixo',
        'hdr-first': 'HDR e Dolby Vision primeiro',
        'hdr-last': 'HDR e Dolby Vision por último',
        source: 'Fonte, na ordem abaixo',
      },
    },
    versionCodecOrder: {
      label: 'Ordem dos codecs de vídeo',
      help: 'A ordem em que o critério “Codec de vídeo” organiza as versões.',
    },
    versionSourceOrder: {
      label: 'Ordem das fontes',
      help: 'A ordem em que o critério “Fonte” organiza as versões: os complementos e pastas do servidor que fornecem versões de filmes e episódios. Uma fonte adicionada depois fica por último até você movê-la, e os complementos próprios dos usuários vêm depois das fontes do servidor. Isso muda apenas a ordem das versões: descrições, legendas e resultados de pesquisa seguem a ordem de Conteúdo › Fontes.',
      none: 'Nenhuma fonte do servidor fornece versões de filmes ou episódios ainda.',
      dragHandle: (name: string) => `Arrastar ${name}`,
    },
    versionCodecs: {
      av1: 'AV1',
      hevc: 'HEVC (H.265)',
      vp9: 'VP9',
      h264: 'H.264 (AVC)',
      vc1: 'VC-1',
      mpeg4: 'MPEG-4 Parte 2 (Xvid, DivX)',
      mpeg2video: 'MPEG-2',
    },
    hideVersionsLarger: 'Ocultar versões maiores que (GB, 0 = sem limite)',
    hideVersionsLargerHelp: (r: RangeText) =>
      `Versões sabidamente maiores não são listadas em lugar nenhum: nem nas páginas dos títulos, nem na reprodução, nem nos downloads. Quando as regras abaixo e esta ocultariam todas as versões de um título, nenhuma é ocultada. Até ${r.max} GB; ${r.default} por padrão.`,
    hiddenVersionCodecs: 'Ocultar estes codecs de vídeo',
    hiddenVersionCodecsHelp: 'Versões sabidamente nesses codecs não são listadas em lugar nenhum.',
    hiddenVersionWords: 'Ocultar versões cujos rótulos contêm',
    hiddenVersionWordsHelp: (r: RangeText) =>
      `Uma palavra ou expressão por linha, buscada sem diferenciar maiúsculas e minúsculas no nome, na descrição e no nome do arquivo de uma versão: para complementos que indicam nos rótulos que um arquivo ainda não está pronto para reprodução. Até ${r.max} linhas; nenhuma por padrão.`,
    hiddenReleases: 'Ocultar lançamentos de baixa qualidade',
    hiddenReleasesHelp:
      'Versões cujo nome ou nome do arquivo as marca como um destes tipos não são listadas em lugar nenhum. Uma marca conta como palavra inteira depois das palavras do próprio título, sem diferenciar maiúsculas e minúsculas: um filme chamado Cam não é uma cópia de câmera, nem um arquivo .ts um telesync. Nenhum por padrão.',
    releaseKinds: {
      cam: 'Cópias de câmera (CAM, HDCAM)',
      telesync: 'Telesyncs (TS, HDTS, TELESYNC)',
      telecine: 'Telecines (TC, TELECINE)',
      screener: 'Screeners (SCR, DVDSCR)',
      early: 'Cópias antecipadas (R5, WORKPRINT)',
    },
    syncAddonSubtitles: 'Sincronizar legendas dos complementos',
    syncAddonSubtitlesHelp:
      'Ajusta o tempo das legendas dos complementos à versão reproduzida, quando estão adiantadas, atrasadas, ou mais rápidas ou mais lentas que ela. Na primeira reprodução de uma versão, o Polyfin identifica em segundo plano quando há fala: a partir de uma legenda em texto dentro do arquivo, senão de três trechos do áudio, lendo no máximo 300 MB do arquivo de um complemento. As versões torrent ficam de fora, pois seria preciso baixá-las do swarm delas. A partir daí, uma legenda que se encaixa claramente melhor deslocada ou acelerada chega sincronizada, com o nome terminando em “(sincronizada)”.',
    remuxDb: 'Descrever versões pelo RemuxDB',
    remuxDbHelp:
      'Mostra as faixas de áudio, legenda e vídeo de uma versão antes da primeira reprodução, como o RemuxDB as encontrou no mesmo arquivo. Quando os detalhes de um título são abertos, o Polyfin consulta o RemuxDB, um banco de dados da comunidade, pelo identificador IMDb do título e encontra as versões pelos nomes de arquivo. A reprodução continua analisando cada versão.',
    remuxDbUrl: 'Endereço do RemuxDB',
    remuxDbUrlHelp: 'O servidor RemuxDB que o Polyfin consulta.',
    playbackHistoryGroup: 'Histórico de reprodução',
    playbackHistory: 'Manter histórico de reprodução',
    playbackHistoryHelp:
      'Guarda cada vídeo reproduzido, com quem o reproduziu, em qual aplicativo e como, para Sistema › Estatísticas e as estatísticas de cada usuário em Minha conta. Desativado, nada mais é guardado; o que já foi guardado permanece até ficar antigo demais.',
    playbackHistoryDays: 'Dias para manter o histórico de reprodução',
    playbackHistoryDaysHelp: (r: RangeText) =>
      `Reproduções mais antigas são excluídas. A verificação é feita todos os dias. De ${r.min} a ${r.max}, ${r.default} por padrão.`,
    defaultLanguagesGroup: 'Idiomas padrão',
    defaultAudioLanguage: 'Idioma de áudio padrão',
    defaultAudioLanguageHelp:
      'Para os usuários que não escolheram um idioma de áudio no aplicativo, o Polyfin reproduz a faixa de áudio neste idioma quando uma versão tem uma, e coloca primeiro as versões conhecidas neste idioma quando as versões são ordenadas por idioma. A escolha do próprio usuário sempre prevalece, e o aplicativo dele continua a mostrá-la como ele a deixou. Este idioma vem antes da faixa que o arquivo marca como padrão, mesmo para usuários cujo aplicativo reproduz a faixa padrão seja qual for o idioma, uma configuração ativada até que eles a desativem; um arquivo sem ele reproduz a faixa marcada. Nenhum por padrão.',
    defaultSubtitleLanguage: 'Idioma de legendas padrão',
    defaultSubtitleLanguageHelp:
      'Para os usuários que não escolheram um idioma de legendas no aplicativo, o Polyfin mostra legendas neste idioma conforme o modo de legendas de cada um. A escolha do próprio usuário sempre prevalece, e o aplicativo dele continua a mostrá-la como ele a deixou. Nenhum por padrão.',
    noDefaultLanguage: 'Nenhum',
    publicAddress: 'Endereço público',
    publicAddressHelp:
      'O endereço em que as pessoas abrem o Polyfin, como https://media.example.org. Os links nas notificações começam com ele; vazio, as mensagens não têm link. Quando começa com https://, “Redirecionar HTTP para HTTPS”, em Configurações › Segurança, leva à porta dele, ou 443 quando nenhuma é indicada.',
    email: 'E-mail',
    emailHelp:
      'Os destinos de e-mail passam por este servidor SMTP. Eles podem ser adicionados depois que um servidor e um endereço de remetente forem salvos.',
    smtpHost: 'Servidor SMTP',
    smtpHostHelp:
      'O nome do host, como smtp.example.org. Vazio, destinos de e-mail não podem ser adicionados.',
    smtpPort: 'Porta SMTP',
    smtpPortHelp: 'Geralmente 587 com STARTTLS e 465 com TLS desde o início.',
    smtpSecurity: 'Criptografia SMTP',
    smtpSecurityHelp:
      'Como a conexão é criptografada. Com STARTTLS, nada é enviado se o servidor não oferecer suporte. Sem criptografia, a senha só é enviada para esta máquina.',
    smtpSecurities: { starttls: 'STARTTLS', tls: 'TLS desde o início', none: 'Nenhuma' },
    smtpUser: 'Usuário SMTP',
    smtpUserHelp: 'Vazio para enviar sem login.',
    smtpPassword: 'Senha SMTP',
    smtpPasswordHelp: 'Ela é guardada criptografada e nunca mais é exibida.',
    smtpFrom: 'Endereço do remetente',
    smtpFromHelp: 'O endereço de onde as mensagens vêm, como polyfin@example.org.',
    smtpFromName: 'Nome do remetente',
    smtpFromNameHelp: 'Exibido com o endereço. Vazio, é usado o nome do servidor.',
    weeklySummary: 'Resumo semanal',
    weeklySummaryHelp:
      'Os destinos que escolheram “Resumo semanal” em seus Eventos o recebem uma vez por semana: os destinos do servidor e dos administradores na semana do servidor, os demais na própria semana. Nada é enviado em uma semana sem nada a relatar. Sistema › Agendamento o envia agora.',
    weeklySummaryDay: 'Enviar o resumo semanal em',
    weeklySummaryDayHelp: (day: string) => `${day} por padrão.`,
    weeklySummaryHour: 'Enviar o resumo semanal às',
    weeklySummaryHourHelp: (hour: string) =>
      `No fuso horário do servidor. A retrospectiva do mês e a retrospectiva do ano também são enviadas neste horário, no dia 1º. ${hour} por padrão.`,
    maxConversions: 'Conversões de vídeo simultâneas (0 = sem limite)',
    maxConversionsHelp: (r: RangeText) =>
      `Converter vídeo é o trabalho mais pesado que o servidor faz. Quando esse número de reproduções estiver com o vídeo sendo convertido (incluindo legendas gravadas na imagem), uma nova reprodução recebe uma versão que não precisa de conversão, ou não é iniciada. Reproduções já em andamento nunca são interrompidas. A TV ao Vivo também mantém seus próprios limites: 4 canais por usuário e 16 para o servidor. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    maxConversionHeight: 'Qualidade máxima do vídeo convertido',
    maxConversionHeightHelp:
      'O vídeo convertido é reduzido no máximo a esta altura, mantendo suas proporções, para que seja reproduzido bem em uma conexão mais lenta. Arquivos reproduzidos como estão ou apenas reempacotados mantêm sua qualidade. Uma placa de vídeo converte até 4K; o processador vai até 1080p e, quando converte HDR para SDR, até “Qualidade máxima do HDR convertido pelo processador”, em Conversão (transcodificação) › HDR.',
    conversionHeightOriginal: 'Original',
    conversionHeight: (height: number) => `${height}p`,
    convertedDownloads: 'Downloads convertidos',
    convertedDownloadsHelp:
      'Aplicativos que solicitam isso, como o Streamyfin, salvam um título na qualidade escolhida pelo usuário: o Polyfin envia uma cópia menor, feita enquanto o aplicativo faz o download e nunca mantida no servidor. Sem isso, eles salvam o arquivo como está. Cada download conta para “Conversões de vídeo simultâneas” enquanto é convertido e como uma das “Reproduções simultâneas” do usuário. O usuário precisa de “Pode fazer download” e de “Pode usar conversão (transcodificação)” quando o vídeo ou o áudio é convertido. Um download interrompido recomeça do início. Desativado por padrão.',
    conversion: {
      description:
        'Quando um aplicativo não consegue reproduzir um arquivo como ele está, o Polyfin o converte com o FFmpeg. Estas configurações definem como, para filmes, séries e TV ao Vivo. Os padrões atendem à maioria dos servidores.',
      groups: {
        general: 'Geral',
        subtitles: 'Legendas em imagem',
        gpu: 'Placa de vídeo',
        video: 'Vídeo',
        hdr: 'HDR',
        interlaced: 'Vídeo entrelaçado',
        audio: 'Áudio',
        performance: 'Desempenho',
      },
      subtitleOcr: 'Transformar legendas em imagem em texto',
      subtitleOcrHelp:
        'Legendas PGS e de DVD (VobSub) são imagens, e muitos aplicativos aceitam apenas texto: para eles, o Polyfin incrusta a faixa no vídeo, o que exige uma conversão. Com esta opção ativada, o Tesseract lê cada faixa em imagem uma vez, em segundo plano, gerando uma cópia em texto listada como mais uma faixa marcada “(OCR)”, que é reproduzida sem conversão. Até a cópia ficar pronta, a faixa é incrustada como antes. A faixa em imagem continua disponível: itálicos, cores e posições se perdem, e fontes estilizadas podem ser lidas incorretamente. “Ao reproduzir” lê as faixas de uma versão na primeira vez que um aplicativo que não consegue exibi-las a reproduz; “Também após rastreamentos da biblioteca” também lê as faixas de novos arquivos locais. Desativado por padrão.',
      subtitleOcrChoices: {
        off: 'Desativado',
        played: 'Ao reproduzir',
        scans: 'Também após rastreamentos da biblioteca',
      },
      subtitleOcrTracks: 'Faixas de legendas em imagem lidas ao mesmo tempo',
      subtitleOcrTracksHelp: (r: RangeText) =>
        `Cada faixa é lida por um processo do Tesseract usando um núcleo do processador. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
      hardwareAcceleration: 'Placa de vídeo usada para converter',
      hardwareAccelerationHelp:
        'Uma placa de vídeo (GPU) converte vídeo muito mais rápido que o processador. Cada sistema oferece as suas: NVIDIA e VAAPI no Linux, VideoToolbox no macOS, NVIDIA, Intel e AMD no Windows. Sem uma placa que funcione, o processador faz a conversão. Uma alteração é aplicada ao salvar, sem reiniciar.',
      vaapiDevice: 'Placa de vídeo para VAAPI',
      vaapiDeviceHelp:
        'O nó de renderização em que o VAAPI converte o vídeo, quando várias placas de vídeo poderiam fazê-lo. Por padrão, uma de cada vez, a primeira que funcionar.',
      vaapiDeviceEach: 'Uma de cada vez',
      vaapiDeviceMissing: (path: string) => `${path} (não encontrado)`,
      hardware: {
        auto: 'Automático: a primeira que funcionar, na ordem abaixo',
        nvenc: 'NVIDIA (NVENC)',
        vaapi: 'AMD ou Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
        none: 'Nenhuma: somente o processador',
      },
      detected: 'Detectado neste servidor',
      noGpu: 'Nenhuma placa de vídeo é usada: o processador converte o vídeo.',
      gpu: 'Placa de vídeo',
      gpuNames: {
        cuda: 'NVIDIA',
        vaapi: 'AMD ou Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
      } as Record<string, string>,
      encoders: 'Codificadores na placa',
      gpuToneMapping: 'HDR para SDR na placa',
      processorToneMappingHint:
        'O HDR é então convertido pelo processador, até “Qualidade máxima do HDR convertido pelo processador”.',
      qualityFactor: 'Aceita um número de qualidade',
      processor: 'Codificadores no processador',
      processorToneMapping: 'HDR para SDR no processador',
      yes: 'Sim',
      no: 'Não',
      none: 'Nenhum',
      hardwareDecoding: 'Ler estes formatos na placa de vídeo',
      hardwareDecodingHelp:
        'A própria placa lê (decodifica) estes formatos, o que deixa o processador livre. Desmarque um se os vídeos nesse formato não forem convertidos ou ficarem com aparência errada: o processador passa a lê-lo. Outros formatos são tentados na placa. HEVC 10 bits requer HEVC.',
      hardwareDecodingNoGpu:
        'Nenhuma placa de vídeo é usada, então o processador lê todos os formatos.',
      codecs: {
        h264: 'H.264',
        hevc: 'HEVC',
        hevc_10bit: 'HEVC 10 bits',
        vp9: 'VP9',
        av1: 'AV1',
        mpeg2video: 'MPEG-2',
        vc1: 'VC-1',
      },
      encoderPreset: 'Velocidade de codificação',
      encoderPresetHelp:
        'Mais lento gera uma imagem melhor com o mesmo tamanho, mas exige mais processamento. Se os vídeos convertidos travarem, escolha um mais rápido. Automático mantém a escolha do Polyfin: muito rápido no processador, médio em placas NVIDIA e o padrão da própria placa nas demais.',
      presets: {
        auto: 'Automático',
        veryslow: 'Muito lento (melhor imagem)',
        slower: 'Mais lento',
        slow: 'Lento',
        medium: 'Médio',
        fast: 'Rápido',
        faster: 'Mais rápido',
        veryfast: 'Muito rápido',
        superfast: 'Super-rápido',
        ultrafast: 'Ultrarrápido (trabalho mais leve)',
      },
      h264Quality: 'Qualidade H.264 (0 = pela taxa de bits)',
      hevcQuality: 'Qualidade HEVC (0 = pela taxa de bits)',
      qualityHelp: (r: RangeText) =>
        `Um número menor gera uma imagem melhor e mais dados; a taxa de bits continua definindo o máximo. 0 visa apenas a taxa de bits. O Jellyfin usa 23 para H.264 e 28 para HEVC. 0, ou de ${r.min} a ${r.max}; ${r.default} por padrão.`,
      qualityIgnored:
        'Esta placa de vídeo não aceita um número de qualidade: ela continua visando a taxa de bits.',
      allowHevcEncoding: 'Permitir conversão para HEVC',
      allowHevcEncodingHelp:
        'O HEVC precisa de menos dados que o H.264 para a mesma imagem, mas exige mais processamento para ser gerado. Quando ativado, os aplicativos que listam o HEVC primeiro o recebem. Quando desativado, apenas os aplicativos que não conseguem reproduzir H.264 recebem HEVC.',
      noHevcEncoder: 'O FFmpeg não consegue gerar HEVC neste servidor.',
      adaptiveQuality: 'Várias qualidades ao converter',
      adaptiveQualityHelp:
        'Os vídeos convertidos também oferecem até duas qualidades inferiores, como 720p e 480p abaixo de 1080p. Os aplicativos que conseguem alternar passam para elas quando a conexão fica lenta, em vez de parar para carregar. Uma qualidade que o aplicativo deixa continua sendo convertida por 10 segundos, para que voltar a ela não recomece do zero, e todas contam como uma única conversão. Vídeos copiados, TV ao Vivo e música mantêm uma única qualidade. “Rede local” significa o mesmo que nos PINs de redefinição de senha, através dos proxies conhecidos. Desativado por padrão.',
      adaptiveQualities: { off: 'Desativado', remote: 'Fora da rede local', always: 'Sempre' },
      toneMapping: 'Converter HDR em SDR (tone mapping)',
      toneMappingHelp:
        'Mantém corretos as cores e o brilho dos vídeos HDR em telas que exibem apenas SDR. Quando desativado, os vídeos HDR convertidos ficam desbotados, e os vídeos Dolby Vision sem camada HDR10 não são convertidos.',
      toneMappingUnavailable:
        'Nem a placa de vídeo nem o FFmpeg conseguem fazer isso neste servidor: os vídeos HDR só são convertidos quando esta opção está desativada.',
      toneMappingAlgorithm: 'Método de tone mapping',
      toneMappingAlgorithmHelp:
        'Como as partes claras são reduzidas para SDR. Automático usa BT.2390 em placas NVIDIA e Hable no processador. Placas Intel e o VideoToolbox usam o próprio método e ignoram esta escolha.',
      algorithms: {
        auto: 'Automático',
        bt2390: 'BT.2390 (somente placas NVIDIA)',
        hable: 'Hable',
        reinhard: 'Reinhard',
        mobius: 'Möbius',
        clip: 'Recorte',
        linear: 'Linear',
      },
      gpuToneMappingSetting: 'Fazer tone mapping de HDR na placa de vídeo',
      gpuToneMappingSettingHelp:
        'Placas NVIDIA, placas Intel (via VAAPI no Linux, QSV no Windows) e o VideoToolbox no macOS convertem HDR em SDR por conta própria, até 4K. Desative esta opção se as cores parecerem erradas: o processador passa a fazer isso, com qualidade inferior. As placas Intel só aceitam vídeos HDR10 que trazem as informações da tela de masterização, e o VideoToolbox, HDR10 e HLG; o processador converte os demais, e o Dolby Vision sem camada HDR10 só é convertido em placas NVIDIA. Placas AMD nunca fazem isso: o driver delas no Linux falhou ou travou em todas as formas que o Polyfin tentou, e o filtro AMF do FFmpeg no Windows não faz tone mapping.',
      gpuToneMappingUnavailable:
        'A placa de vídeo deste servidor não consegue fazer isso: o processador converte HDR em SDR.',
      processorToneMappingHeight: 'Qualidade máxima do HDR convertido pelo processador',
      processorToneMappingHeightHelp:
        'Converter HDR em SDR é um trabalho pesado para o processador: cerca de quatro vezes a própria codificação em 1080p. Automático mede este servidor na inicialização: 1080p quando ele converte pelo menos uma vez e meia mais rápido que a reprodução do vídeo, senão 720p, que também é usado até a medição terminar. Acima de 1080p só vale quando uma placa de vídeo codifica o vídeo: sozinho, o processador para em 1080p.',
      toneMappingHeightAuto: 'Automático (medido na inicialização)',
      height4k: '4K',
      measuredHeight: (height: number) => `Medido: ${height}p`,
      upToHeight: (height: number) => `até ${height}p`,
      toneMappingPeak: 'Brilho máximo em nits (0 = o do vídeo)',
      toneMappingDesat: 'Dessaturação das áreas claras (0 = desativada)',
      toneMappingPeakHelp: (peak: RangeText, desat: RangeText) =>
        `Somente quando o processador converte HDR em SDR: uma placa de vídeo que faz isso os ignora. O pico substitui o nível mais claro que o vídeo diz atingir: 0, ou de ${peak.min} a ${peak.max}; ${peak.default} por padrão. A dessaturação atenua a cor das partes muito claras: de ${desat.min} a ${desat.max}; ${desat.default} por padrão.`,
      processorCannotToneMap:
        'O FFmpeg não consegue fazer tone mapping no processador deste servidor.',
      deinterlaceMethod: 'Método de desentrelaçamento',
      deinterlaceMethodHelp:
        'Transmissões de TV e DVDs costumam ser entrelaçados, o que mostra linhas em forma de pente depois de convertidos. O Yadif é rápido; o Bwdif é um pouco mais nítido.',
      noBwdif: 'O FFmpeg deste servidor não tem Bwdif.',
      deinterlacers: { yadif: 'Yadif', bwdif: 'Bwdif' },
      deinterlaceDoubleRate: 'Dobrar a taxa de quadros',
      deinterlaceDoubleRateHelp:
        'Gera uma imagem a partir de cada meio quadro, para movimentos mais suaves em esportes e programas de TV. Somente para vídeos de até 30 quadros por segundo.',
      downmixAlgorithm: 'Mixagem para estéreo',
      downmixAlgorithmHelp:
        'Como o som surround é mixado para duas caixas de som. Dave750 e Modo noturno mantêm as vozes claras; RFC 7845 e AC-4 seguem padrões.',
      downmixes: {
        None: 'Mixagem própria do FFmpeg',
        Dave750: 'Dave750',
        NightmodeDialogue: 'Modo noturno (vozes mais claras)',
        Rfc7845: 'RFC 7845',
        Ac4: 'AC-4',
      },
      downmixBoost: 'Volume ao mixar para estéreo',
      downmixBoostHelp: (r: RangeText) =>
        `Mixagens em estéreo costumam soar mais baixas: o volume é multiplicado por este número. De ${r.min} a ${r.max}; ${r.default} (sem alteração) por padrão. O Jellyfin usa 2.`,
      maxAudioChannels: 'Máximo de canais de áudio',
      maxAudioChannelsHelp:
        'O som convertido mantém no máximo este número de canais, mesmo que o aplicativo aceite mais.',
      audioChannels: (channels: number): string =>
        channels === 0
          ? 'Quantos o aplicativo aceitar'
          : channels === 1
            ? 'Mono'
            : channels === 2
              ? 'Estéreo'
              : '5.1',
      audioBitratePerChannel: 'Taxa de bits de áudio por canal em kb/s (0 = automático)',
      audioBitratePerChannelHelp: (r: RangeText) =>
        `Automático gera as taxas de bits do Jellyfin: 128 kb/s em mono, 384 kb/s em estéreo e 640 kb/s em 5.1, menos com um limite de taxa de bits baixo. 0, ou de ${r.min} a ${r.max}; ${r.default} por padrão.`,
      encodingThreads: 'Threads do processador por conversão (0 = automático)',
      encodingThreadsHelp: (r: RangeText) =>
        `Limita o quanto do processador uma conversão usa, para deixar espaço para outras tarefas. De ${r.min} a ${r.max}; 0 deixa o FFmpeg escolher.`,
      aheadSeconds: 'Segundos preparados com antecedência',
      aheadSecondsHelp: (r: RangeText) =>
        `O Polyfin prepara um vídeo no máximo este número de segundos além da parte que o aplicativo pediu e depois aguarda. Mais segundos ajudam com fontes lentas e mantêm a conexão delas ocupada, mas usam mais processamento e espaço em disco quando as pessoas param de assistir cedo. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
      keepConvertedVideo: 'Manter o vídeo convertido quando a faixa de áudio mudar',
      keepConvertedVideoHelp:
        'Converte a imagem e o som separadamente, para que escolher outra faixa de áudio no player prepare apenas o novo som: a imagem já convertida continua sendo reproduzida, e a nova faixa começa mais cedo. A imagem é mantida por 30 segundos depois que um player a interrompe e, enquanto isso, conta como uma conversão.',
    },
    catalogsTitle: 'Catálogos',
    catalogLimit: 'Títulos lidos por catálogo de filmes e séries',
    catalogLimitHelp: (r: RangeText) =>
      `Alguns catálogos são quase infinitos, por isso o Polyfin para de ler um catálogo após esta quantidade de títulos. Um número maior mostra mais títulos, mas as listas carregam mais devagar e o complemento recebe mais solicitações. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    channelLimit: 'Canais lidos por catálogo de TV ao Vivo',
    channelLimitHelp: (r: RangeText) =>
      `O Polyfin para de ler um catálogo de TV ao Vivo após esta quantidade de canais e lê no máximo esta quantidade de programas por dia para o guia. Um número maior mostra mais conteúdo, mas carrega mais devagar e o complemento recebe mais solicitações. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    skipButtons: 'Botões para pular abertura e créditos',
    skipButtonsHelp:
      'Os aplicativos oferecem pular aberturas, recapitulações e créditos, encontrados em bancos de dados da comunidade. Quando desativado, os aplicativos não mostram botões de pular e os bancos de dados não são consultados.',
    publicMetaDbKey: 'Chave do PublicMetaDB',
    publicMetaDbKeyHelp:
      'Opcional. Uma chave da API do PublicMetaDB adiciona o PublicMetaDB como fonte de marcadores para pular.',
    theIntroDbKey: 'Chave do TheIntroDB (opcional)',
    theIntroDbKeyHelp:
      'Aumenta o limite diário do TheIntroDB e inclui seus próprios envios. A leitura funciona sem ela.',
    segmentSources: {
      label: 'Fontes de marcadores para pular',
      help: 'Para cada tipo de trecho (abertura, recapitulação, créditos, prévia), vale a primeira fonte ativada nesta lista que o tiver; as seguintes preenchem as lacunas. Fontes desativadas nunca são consultadas.',
      turnOn: (name: string) => `Consultar ${name}`,
      needsKey: 'Requer uma chave',
    },
    audioIntros: 'Encontrar aberturas e créditos no áudio dos episódios locais',
    audioIntrosHelp:
      'Quando um episódio de uma pasta local é reproduzido sem abertura ou créditos vindos de seus capítulos ou das fontes acima, o Polyfin compara em segundo plano o áudio dos episódios da temporada e atribui a cada um o trecho que compartilham perto do início como abertura, e o que compartilham perto do fim como créditos. Os outros episódios da temporada passam então a ter os seus, e o episódio reproduzido os terá a partir da próxima reprodução. Requer os botões de pular ativados.',
    similarTitles: 'Títulos semelhantes',
    similarTitlesHelp:
      'A página de um título lista títulos parecidos, encontrados nos catálogos dos complementos. Quando desativado, a lista fica vazia e os complementos recebem menos solicitações.',
    generateImages: 'Gerar imagens para coleções sem imagem',
    generateImagesHelp:
      'Uma coleção de um complemento sem pôster, e uma lista inteligente exibida como coleção, mostram os pôsteres de seus primeiros títulos com seu nome, desenhados pelo Polyfin depois que seus títulos são lidos. Usuários sob controle parental ou que bloqueiam gêneros não os veem. Quando desativado, elas não mostram imagem. Bibliotecas configuradas com uma imagem gerada a mantêm.',
    tmdbKey: 'Chave da API do TMDB',
    tmdbKeyHelp:
      'Opcional. Com uma chave da API ou um token de acesso de leitura de uma conta TMDB, o Editar imagens do jellyfin-web também busca no TMDB os pôsteres, imagens de fundo e logos de um título, as imagens de um episódio, os pôsteres de uma temporada e as fotos de uma pessoa, que o Polyfin repassa. Sem chave, o Editar imagens lista apenas as imagens dos complementos e o TMDB nunca é consultado.',
    mdblistRatingsKey: 'Chave da API do MDBList para avaliações',
    mdblistRatingsKeyHelp:
      'Opcional. Com uma chave de uma conta MDBList, filmes e séries mostram a avaliação do IMDb, a nota da crítica e a nota do público do MDBList, buscadas em segundo plano e mantidas para todos os usuários. As chaves do MDBList dos próprios usuários para acompanhamento nunca são usadas para isso. Sem chave, o MDBList nunca é consultado.',
    criticRatingShows: 'A avaliação da crítica mostra',
    criticRatingShowsHelp:
      'Os aplicativos Jellyfin têm um único campo para a avaliação da crítica, de 0 a 100: ele mostra a nota da crítica ou a nota do público. Na página de um título, o player web mostra as duas, com a avaliação do IMDb.',
    criticRatingChoices: { critics: 'Nota da crítica', audience: 'Nota do público' },
    lyrics: 'Letras de músicas do LRCLIB',
    lyricsHelp:
      'Os aplicativos mostram as letras das músicas, sincronizadas linha por linha quando o LRCLIB, um banco de dados gratuito de letras, as tem. O Polyfin busca uma música uma única vez, pelo artista, título, álbum e duração, quando ela começa a tocar ou quando seus detalhes ou sua letra são abertos. Quando desativado, as músicas ficam sem letra e o LRCLIB nunca é consultado.',
    evenVolume: 'Equalizar o volume entre as músicas',
    evenVolumeHelp:
      'Os aplicativos aumentam ou diminuem cada música, ou cada álbum, para a mesma intensidade sonora, a partir das tags ReplayGain das músicas locais ou da intensidade medida. As próprias músicas não são alteradas. Quando desativado, os aplicativos não recebem ganho e nada é medido.',
    measureLoudness: 'Medir a intensidade sonora de músicas sem ganho',
    measureLoudnessHelp:
      'O FFmpeg mede a intensidade sonora das músicas sem tag ReplayGain, duas de cada vez na prioridade mais baixa, uma única vez cada: músicas locais após um rastreamento, músicas de complementos após a primeira reprodução. Músicas com mais de 15 minutos e audiolivros nunca são medidos.',
    measureLoudnessChoices: {
      off: 'Desativado',
      local: 'Músicas locais',
      all: 'Músicas locais e de complementos',
    },
    playedPercent: 'Marcar como reproduzido após (%)',
    playedPercentHelp: (r: RangeText) =>
      `Um título é marcado como reproduzido quando a reprodução passa desta parte da sua duração. De ${r.min} a ${r.max}; ${r.default} por padrão, como no Jellyfin.`,
    resumePercent: 'Manter ponto de retomada após (%)',
    resumePercentHelp: (r: RangeText) =>
      `O ponto onde a reprodução parou é mantido, para retomar dali, depois de passar desta parte da duração de um título. Deve ser menor que o limite de reproduzido. De ${r.min} a ${r.max}; ${r.default} por padrão, como no Jellyfin. Um título com menos de 5 minutos é marcado como reproduzido assim que passa deste ponto.`,
    versionListMinutes: 'Manter listas de versões por (minutos)',
    versionListMinutesHelp: (r: RangeText) =>
      `Por quanto tempo o Polyfin usa as versões e legendas que os complementos listam para um título antes de consultá-los novamente. Uma lista mais antiga ainda aparece imediatamente quando o título é aberto de novo, enquanto o Polyfin consulta. Manter as listas por mais tempo envia menos solicitações ao complemento de streams, o que ajuda com provedores que recusam solicitações em excesso, mas novas versões aparecem mais tarde. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    catalogRefreshMinutes: 'Atualizar catálogos após (minutos)',
    catalogRefreshMinutesHelp: (r: RangeText) =>
      `Quanto tempo uma página de catálogo lida de um complemento, incluindo o guia da TV ao Vivo, pode ter antes que o Polyfin a leia novamente. Os aplicativos nunca esperam por isso: uma página mais antiga ainda aparece imediatamente enquanto o Polyfin a lê novamente em segundo plano, e ela é mantida por mais um dia. Um tempo maior envia menos solicitações aos complementos, mas novos títulos aparecem mais tarde. De ${r.min} a ${r.max} (um dia); ${r.default} por padrão.`,
    collectionReadHour: 'Ler todas as coleções diariamente',
    collectionReadHourNever: 'Nunca',
    collectionReadHourHelp:
      'Lê todas as coleções das bibliotecas de coleções do servidor neste horário, uma após a outra, e depois os catálogos das listas inteligentes do servidor, para que abram imediatamente. Isso solicita muitas páginas aos complementos: deixe desativado para um complemento que você compartilha com outras pessoas.',
    collectionLibraryApps: 'Aplicativos que mostram bibliotecas de coleções por coleção',
    collectionLibraryAppsHelp:
      'As bibliotecas de coleções são apresentadas aos aplicativos como bibliotecas de conteúdo misto, para que o jellyfin-web e o Streamyfin lhes deem uma linha na tela inicial. Os aplicativos marcados são informados de que elas contêm coleções, como faz a visualização Coleções do Jellyfin: os aplicativos que listam uma biblioteca pelo tipo mostram então suas coleções, e os títulos de uma coleção depois que ela é aberta, em vez dos títulos de todas as suas coleções juntos. O jellyfin-web e o Streamyfin deixam essas bibliotecas fora das linhas da tela inicial, então não os marque. A lista mostra os aplicativos que entraram neste servidor.',
    collectionLibraryAppsNone: 'Nenhum aplicativo entrou neste servidor ainda.',
    localScanHours: 'Rastrear pastas locais a cada (horas, 0 = nunca)',
    localScanHoursHelp: (r: RangeText) =>
      `Com que frequência cada pasta local é rastreada novamente em busca de arquivos adicionados, alterados ou removidos. As pastas também são rastreadas na inicialização, quando você solicita e quando as pastas monitoradas mudam. 0 desativa o agendamento. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    watchLocalFolders: 'Monitorar alterações em pastas e compartilhamentos',
    watchLocalFoldersHelp: {
      container:
        'Rastreia novamente uma pasta do contêiner alguns segundos depois que seus arquivos param de mudar, para que arquivos adicionados, renomeados ou removidos apareçam em menos de um minuto.',
      computer:
        'Rastreia novamente uma pasta do computador onde o Polyfin é executado alguns segundos depois que seus arquivos param de mudar, para que arquivos adicionados, renomeados ou removidos apareçam em menos de um minuto.',
    },
    watchSharesHelp:
      'Um compartilhamento SMB é rastreado novamente alguns segundos depois que seu servidor avisa sobre uma alteração. Pastas WebDAV são listadas novamente a cada 2 minutos, buckets S3, pastas SFTP e remotes do rclone com a frequência que cada pasta pedir, um compartilhamento com muitas pastas em várias rodadas, e rastreados novamente quando uma listagem muda. Sistema › Integridade informa como cada compartilhamento é monitorado, quais pastas ou compartilhamentos não podem ser e por quê.',
    personalAddons: 'Permitir complementos próprios dos usuários',
    personalAddonsHelp:
      'Permite que os usuários adicionem seus próprios complementos Stremio, além dos do servidor. Quando desativado, os complementos deles são mantidos, mas não usados, e seus aplicativos Jellyfin mostram apenas os complementos do servidor.',
    serverImports:
      'Usuários podem importar o histórico de reprodução de outro servidor ou aplicativo',
    serverImportsHelp:
      'Em Minha conta, cada usuário pode importar seus filmes e episódios reproduzidos, pontos de retomada e favoritos de um servidor Jellyfin ou Emby, ou da sua conta Stremio ou Nuvio, entrando lá com as próprias credenciais. Quando desativado, essas seções ficam ocultas. Um administrador ainda pode importar para um usuário, em Usuários.',
    importEveryMinutes: 'Importar contas conectadas a cada',
    importEveryMinutesHelp:
      'Com que frequência o Polyfin importa novamente as contas do Stremio e do Nuvio às quais os usuários continuam conectados, e os históricos de reprodução que eles importam de serviços de acompanhamento. Uma alteração se aplica imediatamente: cada próxima importação ocorre esse intervalo após a última. Cada importação lê a conta inteira novamente, então um intervalo curto envia mais solicitações a esses serviços e, no caso do Stremio, cujos termos proíbem a extração automatizada, torna essa zona cinzenta mais acentuada. 6 horas por padrão.',
    shareLinks: 'Permitir links de compartilhamento',
    shareLinksHelp:
      'Usuários com “Pode compartilhar títulos”, na página deles em Usuários, podem dar um filme ou um episódio a alguém sem conta, por meio de um link que expira após 1 hora a 30 dias. O convidado assiste em um navegador, em no máximo 1080p e dentro do grupo de qualidade do usuário, e conta imediatamente entre as reproduções do usuário. Quando desativado, nenhum link pode ser criado e todos os links são recusados, sem serem excluídos. Desativado por padrão.',
    importEveryChoice: (minutes: number) => (minutes < 60 ? `${minutes} min` : `${minutes / 60} h`),
    loginAttempts: 'Bloquear uma conta após este número de senhas incorretas (0 = nunca)',
    loginAttemptsHelp: (r: RangeText) =>
      `Após este número de senhas incorretas seguidas, a conta não consegue entrar por 15 minutos, mesmo com a senha correta. Um administrador pode desbloqueá-la antes na página Usuários. 0, ou de ${r.min} a ${r.max}.`,
    twoStepAdministrators: 'Exigir login em duas etapas para administradores',
    twoStepAdministratorsHelp:
      'Os administradores entram neste aplicativo de administração com um código de um aplicativo autenticador após a senha. Quem ainda não configurou faz isso logo após a senha, antes de obter uma sessão. A senha sozinha deixa então de entrar em qualquer aplicativo Jellyfin: eles usam o Quick Connect, aprovado aqui. As sessões e os dispositivos já conectados continuam conectados. Desativado por padrão.',
    inactiveDeviceDays: 'Desconectar dispositivos sem uso há (dias, 0 = nunca)',
    inactiveDeviceDaysHelp: (r: RangeText) =>
      `Aplicativos Jellyfin sem uso há este número de dias são desconectados e precisam entrar novamente. A verificação ocorre a cada hora. O login nesta página de administração não é afetado. De ${r.min} a ${r.max}.`,
    knownProxies: 'Proxies conhecidos',
    knownProxiesHelp: (r: RangeText) =>
      `Os proxies reversos na frente do Polyfin, um por linha: um endereço IP, uma rede como 172.18.0.0/16 ou um nome de host como o nome de um contêiner em uma rede Docker (traefik), até ${r.max}. Um nome é resolvido a cada minuto e cada endereço para o qual ele resolve é considerado confiável, então liste apenas nomes que a sua própria rede resolve. Uma solicitação vinda de um deles vem do cliente indicado no cabeçalho X-Forwarded-For ou X-Real-IP: é esse endereço que limita as falhas de login, aparece nos dispositivos e indica a rede local. Cabeçalhos encaminhados de qualquer outro endereço são ignorados. Deixe vazio quando nada estiver na frente do Polyfin.`,
    https: {
      title: 'HTTPS',
      intro:
        'O Polyfin pode servir HTTPS por conta própria, em uma porta exclusiva, sem um proxy reverso na frente. O HTTP continua funcionando na sua porta aconteça o que acontecer com o HTTPS: uma porta que não pode ser aberta, ou um certificado que não pode ser lido ou renovado, aparece em Sistema › Integridade. As alterações se aplicam imediatamente, sem reinicialização.',
      mode: 'Servir HTTPS',
      modeHelp:
        'Arquivos de certificado: um certificado obtido por outra ferramenta, lido novamente quando muda. Let’s Encrypt: um certificado que o próprio Polyfin obtém e renova, para nomes de host públicos que apontam para este servidor. Desativado por padrão.',
      modes: { off: 'Desativado', files: 'Arquivos de certificado', letsencrypt: 'Let’s Encrypt' },
      port: 'Porta HTTPS',
      portHelp: (r: RangeText) =>
        `A porta TCP em que o HTTPS escuta no contêiner: publique-a, como a 8096 é publicada para HTTP. O usuário do contêiner não pode escutar abaixo de 1024: para acessar o Polyfin na 443, encaminhe a 443 para esta porta no roteador ou nas portas do contêiner, e defina o Endereço público, em Configurações › Notificações, como https:// e o nome do host, sem porta. De ${r.min} a ${r.max}; ${r.default} por padrão, como no Jellyfin.`,
      redirect: 'Redirecionar HTTP para HTTPS',
      redirectHelp:
        'Solicitações da internet que chegam por HTTP recebem um redirecionamento permanente (308) para o mesmo endereço por HTTPS, na porta pela qual a internet acessa o HTTPS: a porta do Endereço público, em Configurações › Notificações, quando ele começa com https://, 443 quando ele não indica nenhuma, caso contrário a porta HTTPS. A rede local, as verificações do Let’s Encrypt e as solicitações que um proxy conhecido recebeu por HTTPS permanecem em HTTP. Só se aplica enquanto o HTTPS estiver sendo servido. Desativado por padrão.',
      certificate: 'Certificado',
      certificateHelp:
        'O caminho no contêiner do arquivo PEM do certificado com sua cadeia, como /certs/fullchain.pem. Ao salvar, verifica-se se ele pode ser lido. O Polyfin o lê novamente até um minuto após uma alteração e mantém o último certificado válido enquanto uma alteração não puder ser lida.',
      privateKey: 'Chave privada',
      privateKeyHelp:
        'O caminho no contêiner da chave privada RSA ou ECDSA do certificado, como arquivo PEM, por exemplo /certs/privkey.pem. Ao salvar, verifica-se se ela pertence ao certificado.',
      hosts: 'Nomes de host',
      hostsHelp: (r: RangeText) =>
        `Os nomes públicos que apontam para este servidor, um por linha, como media.example.org, até ${r.max}. Um único certificado contém todos eles. Nomes que só resolvem na rede local, e curingas, exigem o desafio DNS-01, que o Polyfin não responde: para eles, use arquivos de certificado de outra ferramenta.`,
      email: 'E-mail',
      emailHelp: 'Opcional. O Let’s Encrypt pode escrever para este endereço sobre a conta.',
      terms: 'Aceito os termos de serviço do Let’s Encrypt',
      termsHelp:
        'O Let’s Encrypt só emite certificados depois que seus termos de serviço, em letsencrypt.org/repository, forem aceitos.',
      challenge: 'Desafio',
      challengeHelp:
        'Como o Let’s Encrypt verifica que os nomes apontam para este servidor. HTTP-01: o Polyfin responde na sua porta HTTP, que a porta 80 da internet deve alcançar. TLS-ALPN-01: o Polyfin responde na sua porta HTTPS, que a porta 443 da internet deve alcançar. O certificado, sua chave e a chave da conta são guardados no banco de dados, as chaves criptografadas com POLYFIN_SECRET_KEY, e o certificado é renovado quando resta um terço da sua validade.',
      challenges: {
        'http-01': 'HTTP-01 (porta 80 para a porta HTTP)',
        'tls-alpn-01': 'TLS-ALPN-01 (porta 443 para a porta HTTPS)',
      },
      useAddress: (address: string) => `Usar ${address}`,
      offeredAddress:
        'Os endereços em que o Let’s Encrypt serve HTTPS, em Configurações › Segurança: sem porta quando o roteador encaminha a 443 para a porta HTTPS. “Redirecionar HTTP para HTTPS” leva à porta do endereço escolhido.',
    },
    detailedLog: 'Registro detalhado (para diagnosticar um problema)',
    detailedLogHelp:
      'O Polyfin escreve muito mais no seu registro, imediatamente e sem reinicialização. Desative quando o problema for encontrado.',
    saved: 'Configurações salvas.',
    thumbnailsHelp:
      'Imagens geradas a partir dos próprios títulos, depois que são assistidos. Para gerá-las, o Polyfin lê pequenas partes do arquivo na fonte, em ritmo moderado, quando ninguém está reproduzindo dessa fonte: no máximo 60 solicitações por título, uma a cada 3 segundos, e 120 por hora por fonte, nunca o arquivo inteiro. Uma fonte que pede ao Polyfin para desacelerar não recebe solicitações de imagens por 2 horas. Desativado por padrão.',
    trickplay: 'Miniaturas ao navegar por um título',
    trickplayHelp:
      'Os aplicativos mostram uma pequena imagem do momento para o qual o usuário se move na barra de reprodução. O Polyfin as gera depois que um título é assistido, e para o próximo episódio quando a reprodução é preparada com antecedência. Filmes longos recebem miniaturas mais espaçadas: alguns minutos de intervalo em vez de alguns segundos.',
    trickplayInterval: 'Uma miniatura a cada (segundos, no mínimo)',
    trickplayIntervalHelp: (r: RangeText) =>
      `Com que frequência a barra de reprodução muda de imagem. O Polyfin lê no máximo 60 imagens por título, então um título com mais de cerca de 10 minutos recebe uma imagem por intervalo, com os intervalos distribuídos uniformemente pela duração: cerca de 1 minuto para um episódio de 1 hora, 2 minutos para um filme de 2 horas. De ${r.min} a ${r.max}; ${r.default} por padrão, como no Jellyfin.`,
    trickplayWidth: 'Largura das miniaturas',
    trickplayWidthHelp:
      'Miniaturas mais largas ficam mais nítidas em telas grandes, mas ocupam mais espaço. Alterar isso gera novas miniaturas na próxima vez que um título for reproduzido.',
    pixels: (width: number) => `${width} pixels`,
    chapterImages: 'Imagens de capítulos',
    chapterImagesHelp:
      'Os aplicativos mostram uma imagem para cada capítulo de um título, na sua lista de cenas. O Polyfin usa a imagem lida mais próxima do início de cada capítulo, das mesmas leituras usadas para as miniaturas, depois que o título é assistido.',
    thumbnailStorage: 'Espaço para imagens (GB)',
    thumbnailStorageHelp: (r: RangeText) =>
      `As miniaturas e as imagens de capítulos são guardadas no banco de dados. Acima desse tamanho, as imagens dos títulos assistidos há mais tempo são removidas; elas são geradas novamente se o título for reproduzido de novo. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    cacheSize: 'Espaço em disco para arquivos em leitura (GB)',
    cacheSizeHelp: (r: RangeText) =>
      `Espaço que o Polyfin pode usar para guardar as partes dos arquivos em reprodução, para que avanços, retrocessos e reinícios não as baixem novamente. As partes lidas nos últimos 30 segundos são mantidas mesmo acima desse limite. De ${r.min} a ${r.max} GB; ${r.default} por padrão.`,
    workFolders: {
      conversionFolder: 'Pasta de conversão',
      conversionFolderHelp:
        'Onde são gravados os segmentos dos arquivos convertidos ou reempacotados durante a reprodução: as gravações mais intensas, para as quais convém um disco rápido ou uma pasta em memória. Precisa de espaço para os segmentos de todas as conversões em andamento.',
      readingFolder: 'Pasta de arquivos em leitura',
      readingFolderHelp:
        'Onde são guardadas as partes dos arquivos dos complementos e as peças dos torrents em reprodução, dentro do espaço em disco acima: muitas vezes vários GB, para os quais convém um disco grande.',
      cacheFolder: 'Pasta de cache',
      cacheFolderHelp:
        'Onde são guardadas as imagens repassadas, até 1 GB, de uma inicialização para a outra, junto com os guias e as legendas em imagem em leitura.',
      where: {
        container:
          'Uma pasta do contêiner do Polyfin, onde está montado um disco ou uma pasta em memória; vazia, o cache do seu volume de dados, mostrado. O Polyfin precisa conseguir gravar nela. Uma pasta nova vale a partir da próxima inicialização do Polyfin; a anterior fica como está.',
        computer:
          'Uma pasta do computador em que o Polyfin roda; vazia, o cache da sua pasta de dados, mostrado. O Polyfin precisa conseguir gravar nela. Uma pasta nova vale a partir da próxima inicialização do Polyfin; a anterior fica como está.',
      },
    },
    recordingsTitle: 'Gravações',
    recordingsFolder: (folder: string) => `As gravações são salvas em ${folder}.`,
    recording: 'Gravar TV ao Vivo',
    recordingHelp:
      'Permite que os usuários agendem gravações de programas e séries da TV ao Vivo, salvas na pasta abaixo.',
    recordingsFolderLabel: 'Pasta de gravações',
    recordingsFolderHelp: {
      container:
        'Uma pasta do contêiner do Polyfin (a mostrada quando vazio), no volume de dados dele. Para manter as gravações em outro disco, monte-o ali, ou em outro lugar e digite o caminho dele aqui. O Polyfin precisa poder gravar nela. Alterá-la não move as gravações já feitas.',
      computer:
        'Uma pasta do computador em que o Polyfin roda (a mostrada quando vazio), na pasta de dados dele. Para manter as gravações em outro disco, digite aqui uma pasta desse disco. O Polyfin precisa poder gravar nela. Alterá-la não move as gravações já feitas.',
    },
    recordingPrePadding: 'Iniciar gravações antes (minutos)',
    recordingPrePaddingHelp: (r: RangeText) =>
      `Quantos minutos antes do programa uma gravação começa. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    recordingPostPadding: 'Continuar gravando após o fim (minutos)',
    recordingPostPaddingHelp: (r: RangeText) =>
      `Quantos minutos após o programa uma gravação continua. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    recordingRetentionDays: 'Manter gravações por (dias, 0 = para sempre)',
    recordingRetentionDaysHelp: (r: RangeText) =>
      `Gravações mais antigas que isso são excluídas. A verificação é feita todos os dias. De ${r.min} a ${r.max}.`,
    recordingCommercials: 'Encontrar comerciais nas gravações',
    recordingCommercialsHelp:
      'Quando uma gravação termina, o Polyfin a lê em segundo plano, uma gravação por vez, em busca dos momentos que são ao mesmo tempo pretos e silenciosos, e considera os trechos curtos entre eles como intervalos comerciais. Os aplicativos então oferecem pulá-los, conforme cada usuário escolheu nas configurações de reprodução do seu aplicativo, enquanto os botões de pular abertura e créditos estiverem ativados. Gravações com mais de 4 horas não são lidas.',
    convertRecordings: 'Converter gravações concluídas para HEVC',
    convertRecordingsHelp:
      'As gravações concluídas são convertidas para HEVC, uma por vez, nos horários abaixo, para ocuparem menos espaço. A imagem mantém o tamanho; o áudio e as legendas são copiados como estão. A conversão usa a “Placa de vídeo usada para converter”, com a “Qualidade HEVC” e o “Método de desentrelaçamento” de Conversão (transcodificação), e conta como uma das “Conversões de vídeo simultâneas”: ela espera enquanto as reproduções as ocupam. Ela só começa se houver espaço livre para uma cópia do tamanho da gravação. A cópia só substitui a gravação se tiver a mesma duração, as mesmas faixas, for decodificada até o fim e for menor; caso contrário, ela é excluída, e a gravação é mantida como está e não é tentada novamente. Uma gravação que alguém está reproduzindo é substituída quando a pessoa para; o estado de reprodução, os pontos de retomada e os comerciais são mantidos. Aplicativos que não reproduzem HEVC recebem a gravação convertida ao reproduzi-la: com “Conversão (transcodificação)” desativada, eles não conseguem reproduzi-la. Desativado por padrão.',
    convertRecordingsBetween: 'Converter entre',
    convertRecordingsAnd: 'e',
    convertRecordingsBetweenHelp: (from: string, until: string) =>
      `No fuso horário do servidor. Uma conversão ainda em andamento no fim é interrompida e recomeça do início na noite seguinte. De ${from} a ${until} por padrão.`,
    convertRecordingsFrom: 'Início das conversões',
    convertRecordingsUntil: 'Fim das conversões',
    convertRecordingsOnProcessor: 'Usar o processador quando não houver placa de vídeo',
    convertRecordingsOnProcessorHelp:
      'Sem uma placa de vídeo que codifique HEVC, as gravações são convertidas no processador com x265, na prioridade mais baixa. É lento e mantém o processador ocupado durante os horários acima. Sem placa de vídeo e com esta opção desativada, nada é convertido. Desativado por padrão.',
    liveTvRefreshHours: 'Atualizar listas e guias da TV ao Vivo a cada (horas)',
    liveTvRefreshHoursHelp: (r: RangeText) =>
      `Com que frequência as listas de canais IPTV e os guias de programação XMLTV são baixados novamente. Um download que falhou é tentado novamente antes: após 5 minutos, 15 minutos e depois a cada hora. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    backupsFolder: (folder: string) => `Os backups do banco de dados são salvos em ${folder}.`,
    backups: 'Fazer backup do banco de dados todos os dias',
    backupsHelp:
      'Grava uma cópia do banco de dados na pasta abaixo todos os dias, no horário escolhido, e exclui as mais antigas além do número mantido.',
    backupFolderLabel: 'Pasta de backups',
    backupFolderHelp: {
      container:
        'Uma pasta do contêiner do Polyfin (a mostrada quando vazio), no volume de dados dele. Os backups ficam mais seguros em um disco diferente do banco de dados: monte um ali, ou em outro lugar e digite o caminho dele aqui. O Polyfin precisa poder gravar nela.',
      computer:
        'Uma pasta do computador em que o Polyfin roda (a mostrada quando vazio), na pasta de dados dele. Os backups ficam mais seguros em um disco diferente do banco de dados: digite aqui uma pasta de um desses discos. O Polyfin precisa poder gravar nela.',
    },
    backupHour: 'Fazer backup todos os dias às',
    backupHourHelp: (hour: string) => `No fuso horário do servidor. ${hour} por padrão.`,
    backupsKept: 'Backups a manter',
    backupsKeptHelp: (r: RangeText) =>
      `Após cada backup, o Polyfin exclui os backups mais antigos além deste número. Outros arquivos da pasta nunca são alterados. De ${r.min} a ${r.max}; ${r.default} por padrão.`,
    lastBackup: 'Último backup',
    backupDestinations: 'Destinos',
    backupDestinationsHelp:
      'Cada backup também é enviado para estes lugares, com o mesmo nome de arquivo, depois de gravado na pasta: armazenamento compatível com S3, servidores SFTP ou WebDAV. Um envio que falhou é tentado novamente no próximo backup.',
    webPlayerHelp:
      'O que o player web (jellyfin-web, em /web/) mostra além das próprias páginas. O script é aplicado na próxima vez que uma página do player web for carregada; o player web mantém o CSS e a mensagem de login por até um minuto.',
    openWebPlayer: 'Abrir o player web',
    webTheme: 'Tema',
    webThemeHelp:
      'A aparência do player web. LumaaGlaass é o tema que acompanha o Polyfin. Nenhum mantém a aparência do jellyfin-web, cujo tema de exibição cada usuário escolhe em Configurações › Exibição. Personalizado usa o seu CSS e o seu JavaScript abaixo, mantidos enquanto outro tema é escolhido.',
    webThemeChoices: { lumaaglaass: 'LumaaGlaass', none: 'Nenhum', custom: 'Personalizado' },
    webThemeReload:
      'O player web guarda o CSS personalizado que leu por até um minuto: depois de uma alteração, recarregue as páginas abertas dele um minuto depois.',
    webThemeTexts: {
      lumaaglaass:
        'O que o Polyfin serve para LumaaGlaass, mostrado aqui e não editável. Escolha Personalizado, ou “Editar como personalizado”, para escrever o seu.',
      none: 'Nenhum não serve CSS nem JavaScript personalizado. Escolha Personalizado para escrever o seu.',
    },
    editAsCustom: 'Editar como personalizado',
    editAsCustomTitle: 'Substituir seus textos personalizados?',
    editAsCustomConfirm:
      'Seu CSS e seu JavaScript personalizados são substituídos pelos do LumaaGlaass, e o tema passa a Personalizado. Salvar a página mantém a alteração.',
    themeCopy: 'Manter uma cópia no Polyfin',
    themeCopyHelp: (depth: number, file: string, total: string, files: number) =>
      `Salva as folhas de estilos, scripts, fontes e imagens que os textos personalizados carregam de outro lugar, como um tema em um CDN, na pasta de dados do Polyfin, e faz os textos carregarem esses arquivos do Polyfin: o tema funciona então sem acessar o CDN dele. Segue @import, url() e os endereços que os scripts citam, ${depth} níveis abaixo, só de endereços na internet, até ${file} por arquivo, e ${total} e ${files} arquivos no total.`,
    themeCopySaved: (files: number, size: string, date: string) =>
      `Uma cópia de ${files === 1 ? 'um arquivo' : `${files} arquivos`}, ${size}, salva em ${date}.`,
    themeCopyFiles: 'Arquivos salvos',
    themeCopyUnsaved: (count: number) =>
      count === 1
        ? 'Não foi possível salvar um endereço; ele ainda é carregado de outro lugar:'
        : `Não foi possível salvar ${count} endereços; eles ainda são carregados de outro lugar:`,
    themeCopyMissing: (count: number) =>
      count === 1
        ? 'Falta um arquivo da cópia na pasta de dados do Polyfin, como depois de restaurar um backup em outro sistema. Enquanto isso, os navegadores o carregam do endereço original, o que exige internet. Atualize a cópia para baixá-lo de novo.'
        : `Faltam ${count} arquivos da cópia na pasta de dados do Polyfin, como depois de restaurar um backup em outro sistema. Enquanto isso, os navegadores os carregam dos endereços originais, o que exige internet. Atualize a cópia para baixá-los de novo.`,
    themeCopyOutside: (count: number) =>
      count === 1
        ? 'Os textos personalizados carregam um endereço de outro lugar:'
        : `Os textos personalizados carregam ${count} endereços de outro lugar:`,
    themeCopyStillOutside: (count: number) =>
      count === 1
        ? 'Um endereço adicionado depois da cópia é carregado de outro lugar; atualize a cópia para salvá-lo também:'
        : `${count} endereços adicionados depois da cópia são carregados de outro lugar; atualize a cópia para salvá-los também:`,
    themeCopyNothing: 'Os textos personalizados não carregam nada de outro lugar.',
    themeCopyKeep: 'Manter uma cópia no Polyfin',
    themeCopyUpdate: 'Atualizar a cópia',
    themeCopyRemove: 'Remover a cópia',
    themeCopyRemoveTitle: 'Remover a cópia?',
    themeCopyRemoveConfirm:
      'Os arquivos dela são apagados, e os textos personalizados voltam a carregá-los dos endereços originais.',
    themeCopyKept: 'Cópia salva. Recarregue as páginas abertas do player web daqui a um minuto.',
    themeCopyRemoved:
      'Cópia removida. Recarregue as páginas abertas do player web daqui a um minuto.',
    themeCopyFailed: 'Nenhum arquivo pôde ser salvo, então nenhuma cópia foi feita:',
    themeCopySaveFirst:
      'Salve primeiro suas alterações: a cópia é feita a partir dos textos salvos.',
    themeCopyKinds: {
      stylesheet: 'folha de estilos',
      script: 'script',
      font: 'fonte',
      image: 'imagem',
    },
    themeCopyReasons: {
      private_address: 'em uma rede local, ou redirecionado para ela',
      not_http: 'redirecionado para um endereço que não é http nem https',
      unavailable: 'indisponível',
      timeout: 'demorou demais',
      wrong_type: 'não é um arquivo do tipo com que é carregado',
      too_large: 'maior que o limite de um arquivo',
      total_too_large: 'além do limite da cópia inteira',
      too_many_files: 'além do número de arquivos permitido',
      too_deep: 'citado níveis demais abaixo',
      built_at_run_time:
        'completado por um script quando ele roda, o que nenhuma cópia consegue seguir',
    },
    customCss: 'CSS personalizado',
    customCssHelp:
      'Aplicado a todas as páginas do player web, para todos os usuários, como o CSS personalizado do Jellyfin. Os usuários podem desativá-lo em Configurações › Exibição.',
    customJs: 'JavaScript personalizado',
    customJsHelp:
      'Carregado por todas as páginas do player web, após os scripts dele. Vazio, nada é carregado.',
    customJsWarningTitle: 'Cole apenas código em que você confia',
    customJsWarning:
      'Este script é executado no navegador de todos os usuários que abrem o player web neste servidor, com a conta deles. Ele pode ler o que eles veem e agir por eles.',
    loginDisclaimer: 'Mensagem de login',
    loginDisclaimerHelp:
      'Exibida abaixo do formulário de login do player web. Texto simples, Markdown ou HTML; o player web remove HTML inseguro.',
    addVersionsToOpenPage: 'Adicionar versões à página aberta de um título',
    addVersionsToOpenPageHelp:
      'Adiciona as versões que os complementos encontram ao menu de versões da página de um filme ou episódio enquanto ela está aberta, mantendo a versão escolhida. Quando desativado, uma página aberta antes de qualquer versão ser encontrada ainda lista as primeiras à medida que chegam, e os botões de reprodução dela ainda aguardam uma versão; as demais aparecem quando a página é aberta novamente.',
    homeRows: 'Linhas da tela inicial',
    homeRowsHelp:
      'Linhas que o player web adiciona à página inicial, no lugar delas, para todos os usuários ou para usuários escolhidos. Cada usuário pode desativar uma linha em Minha conta › Linhas da tela inicial; as seções próprias do player web ficam em Configurações › Início. Cada linha deixa de fora o que o usuário não pode ver: títulos acima do controle parental dele, gêneros bloqueados, títulos ocultos e as bibliotecas ocultas para ele.',
    homeRow: {
      loading: 'Carregando as linhas da tela inicial…',
      empty: 'Nenhuma linha na tela inicial',
      emptyHint: 'A página inicial do player web mostra apenas as próprias seções.',
      add: 'Adicionar uma linha',
      addTitle: 'Adicionar uma linha à tela inicial',
      editTitle: (name: string) => `Editar ${name}`,
      edit: 'Editar',
      delete: 'Excluir',
      deleteTitle: (name: string) => `Excluir ${name}?`,
      deleteBody:
        'A linha sai da página inicial de todos os usuários, e a biblioteca dela sai dos outros aplicativos.',
      deleted: (name: string) => `${name} excluída.`,
      saved: (name: string) => `${name} salva.`,
      create: 'Adicionar',
      creating: 'Adicionando…',
      save: 'Salvar',
      saving: 'Salvando…',
      name: 'Nome',
      nameHelp: 'O título da linha na página inicial.',
      becauseNameHelp:
        'Cada linha recebe esse nome, seguido do título a que se refere: “Porque você assistiu Filme de exemplo”.',
      kind: 'Tipo',
      kinds: {
        catalog: 'Um catálogo',
        collection: 'Uma coleção',
        because: 'Porque você assistiu',
        new: 'Novidades nas suas bibliotecas',
      },
      kindHelp: {
        catalog:
          'A primeira página de um catálogo dos complementos do servidor, seja uma biblioteca ou não, como um catálogo de tendências. Usuários que não usam os complementos do servidor, ou dos quais a biblioteca dele está oculta, não a veem.',
        collection:
          'Os títulos de uma coleção: uma criada por usuários, uma lista inteligente exibida como coleção ou uma coleção de um complemento.',
        because:
          'Uma linha para cada um dos últimos filmes e séries que o usuário reproduziu, com os títulos semelhantes a ele, como as recomendações de filmes os apresentam. Nenhuma enquanto Títulos semelhantes estiver desativado.',
        new: 'Os títulos que entraram nos catálogos das bibliotecas do usuário nos últimos 7 dias, dos mais recentes aos mais antigos. O Polyfin os conta a partir do momento em que viu cada título pela primeira vez em cada catálogo.',
      },
      defaultNames: {
        because: 'Porque você assistiu',
        new: 'Novidades nas suas bibliotecas',
      },
      catalog: 'Catálogo',
      noCatalog: 'Os complementos do servidor não têm nenhum catálogo para mostrar.',
      collection: 'Coleção',
      noCollection: 'Ainda não há nenhuma coleção.',
      collectionSources: {
        users: 'Criada por usuários',
        smart: 'Lista inteligente',
        addon: 'Complemento',
      },
      choose: 'Escolher…',
      unknownCatalog: 'Um catálogo que não é mais oferecido',
      unknownCollection: 'Uma coleção que não é mais oferecida',
      count: 'Linhas',
      countHelp: 'Quantos dos últimos filmes e séries reproduzidos ganham uma linha, de 1 a 10.',
      place: 'Posição',
      places: {
        top: 'No topo',
        resume: 'Depois de Continuar assistindo',
        nextup: 'Depois de A seguir',
        bottom: 'No final',
      },
      placeHelp:
        'Onde o player web a exibe. Quando a página inicial de um usuário não mostra essa seção, a linha ocupa a posição anterior.',
      shownTo: 'Exibida para',
      everyone: 'Todos os usuários',
      chosenUsers: 'Usuários escolhidos',
      users: 'Usuários',
      noUser: 'Escolha pelo menos um usuário.',
      shownToUsers: (count: number) => (count === 1 ? '1 usuário' : `${count} usuários`),
      rowsCount: (count: number) => (count === 1 ? '1 linha' : `${count} linhas`),
      library: 'Também uma biblioteca em outros aplicativos',
      libraryHelp:
        'Não é possível dar aos outros aplicativos um novo tipo de seção da tela inicial: isso torna a linha também uma biblioteca, sem tipo, que eles listam entre as bibliotecas do usuário, com uma linha dos títulos mais recentes dela na tela inicial.',
      libraryBadge: 'Biblioteca',
      moveLabel: 'Mover',
    },
    codeSize: (used: number, max: number) =>
      `${used} KB de ${max >= 1024 ? `${max / 1024} MB` : `${max} KB`}`,
    codeKeys: 'Tab insere espaços; pressione Escape e depois Tab para sair do campo.',
  },
  settingsPage: {
    searchHint: 'Por nome ou descrição.',
    noMatch: 'Nenhuma configuração corresponde a esta pesquisa.',
    sections: {
      variables: 'Variáveis de ambiente',
    },
    variablesHelp:
      'Somente leitura. Elas são definidas no contêiner (ambiente do Docker, arquivo compose ou modelo do Unraid) e se aplicam quando o Polyfin inicia: altere-as lá e reinicie o contêiner. As variáveis que antes definiam outras opções são copiadas para as configurações uma única vez, na primeira inicialização desta versão, e não são mais lidas: altere essas nas configurações. Os segredos ficam ocultos, e o endereço do banco de dados mostra apenas o host e o nome do banco.',
    value: 'Valor em uso',
    defaultValue: 'Padrão',
    setValue: 'Definido',
    hidden: 'Oculto',
    empty: 'Vazio',
    notRead: 'Não lida pelo Polyfin',
    searchPlaceholder: 'Pesquisar uma configuração',
    results: (count: number) =>
      count === 1 ? '1 configuração encontrada' : `${count} configurações encontradas`,
    noMatchHelp: 'Tente outra palavra, como o nome de uma variável ou de um serviço.',
    variableHint: 'Variável de ambiente',
    ledes: {
      general:
        'O nome do servidor, seu idioma, como os aplicativos entram e o encontram, e a verificação de novas versões.',
      playback: 'Como o Polyfin escolhe e prepara as versões que reproduz.',
      content:
        'Botões para pular a introdução, títulos semelhantes, imagens geradas, avaliações, letras de músicas, volume uniforme entre as músicas e limites de reprodução.',
      catalogs: 'Quanto o Polyfin lê dos catálogos dos complementos, e com que frequência.',
      security:
        'Complementos e importações dos próprios usuários, contas bloqueadas, dispositivos sem uso e proxies reversos.',
      liveTv: 'Com que frequência as listas de canais e os guias IPTV são baixados novamente.',
      diagnostics: 'O registro detalhado e as variáveis de ambiente em uso.',
      notifications:
        'Onde o Polyfin avisa, para cada usuário, sobre novos episódios, gravações, reproduções e conexões que param, e sobre problemas de integridade, novos usuários, novas versões e contas bloqueadas, e envia o resumo semanal e as retrospectivas do mês e do ano.',
    },
    groups: {
      skip: 'Pular introdução e créditos',
      titlePages: 'Páginas de títulos',
      images: 'Imagens',
      ratings: 'Avaliações',
      music: 'Música',
      thresholds: 'Limites de reprodução',
    },
    leaveTitle: 'Sair sem salvar?',
    leaveBody:
      'As alterações feitas nesta seção ainda não foram salvas. Elas serão perdidas se você sair.',
    leave: 'Sair sem salvar',
    stay: 'Ficar',
    variablesEmpty: 'Nenhuma variável para mostrar',
    variablesEmptyHelp: 'Defina variáveis POLYFIN_ no contêiner e reinicie-o.',
  },
}

export default settings
