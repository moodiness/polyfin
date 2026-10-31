import type en from '../en/sources'

const sources: typeof en = {
  sources: {
    title: 'Fontes',
    lede: 'Complementos Stremio, complementos Eclipse, fontes IPTV e pastas locais em uma única lista. As fontes do servidor atendem todos os usuários; as fontes próprias de um membro atendem apenas ele.',
    add: 'Adicionar uma fonte',
    listLabel: 'Fontes',
    listNote: 'As fontes próprias dos membros atendem apenas o respectivo dono.',
    loading: 'Carregando as fontes',
    kindFilter: 'Filtrar por tipo',
    all: 'Todas',
    kinds: { stremio: 'Stremio', eclipse: 'Eclipse', iptv: 'IPTV', local: 'Pastas' },
    owner: 'Dono',
    everyOwner: 'Todos os donos',
    server: 'O servidor',
    search: 'Pesquisar nas fontes',
    searchPlaceholder: 'Pesquisar por nome',
    clearFilters: 'Limpar os filtros',
    emptyTitle: 'Nenhuma fonte ainda',
    emptyShared:
      'Adicione um complemento Stremio, um complemento Eclipse, uma fonte IPTV ou uma pasta local: ela atenderá todos os usuários deste servidor.',
    emptyMine:
      'Adicione um complemento Stremio, um complemento Eclipse ou uma fonte IPTV: ela atenderá apenas você, depois das fontes do servidor.',
    noMatchTitle: 'Nenhuma fonte corresponde',
    noMatch: 'Altere o tipo, o dono ou a pesquisa.',
    tag: {
      stremio: 'Stremio',
      eclipse: 'Eclipse',
      m3u: 'M3U',
      xtream: 'Xtream',
      hdhomerun: 'HDHomeRun',
      local: 'Pasta',
    },
    kindName: {
      stremio: 'Complemento Stremio',
      eclipse: 'Complemento Eclipse',
      m3u: 'Lista de reprodução M3U',
      xtream: 'Conta Xtream Codes',
      hdhomerun: 'Sintonizador HDHomeRun',
      local: 'Pasta local',
    },
    ownedBy: (name: string) => `Fonte de ${name}`,
    yours: 'Sua própria fonte',
    active: 'Ativa',
    off: 'Desativada',
    failed: 'Falha no download',
    channelsShown: (count: string) => `${count} canais exibidos`,
    notDownloaded: 'Ainda não baixada',
    moreActions: (name: string) => `Mais ações para ${name}`,
    openPage: 'Abrir a página',
    enabled: 'Ativada',
    enabledHelp:
      'Desativar uma fonte oculta as bibliotecas dela nos aplicativos Jellyfin sem perdê-las.',
    order: 'Ordem',
    position: (position: number, count: number) => `${position} de ${count}`,
    moveUp: (name: string) => `Mover ${name} para cima`,
    moveDown: (name: string) => `Mover ${name} para baixo`,
    moved: (name: string, position: number, count: number) =>
      `${name} movido para a posição ${position} de ${count}.`,
    description: 'Descrição',
    details: 'Detalhes',
    manifestUrl: 'Endereço do manifesto',
    address: 'Endereço',
    version: 'Versão',
    provides: 'Fornece',
    types: 'Tipos',
    catalogs: 'Catálogos',
    catalogCount: (count: number) => (count === 1 ? '1 catálogo' : `${count} catálogos`),
    lastRefresh: 'Última atualização',
    channels: 'Canais',
    notImported: 'Não importados',
    shownOf: (shown: string, total: string) => `${shown} exibidos de ${total}`,
    vod: 'Filmes e séries',
    vodShort: (movies: string, series: string) => `${movies} filmes e ${series} séries importados`,
    lastFetch: 'Último download',
    nextFetch: 'Próximo download',
    never: 'Nunca',
    figures: {
      entries: 'Entradas na fonte',
      shown: 'Canais exibidos',
      mapped: 'Com guia',
      unmapped: 'Sem guia',
    },
    openSource: 'Abrir a fonte',
    iptvHint:
      'O que importar, categorias, canais, números e guias são definidos na página da fonte.',
    refresh: 'Atualizar',
    refreshLabel: (name: string) => `Atualizar ${name}`,
    refreshed: (name: string) => `${name}: o manifesto foi baixado novamente.`,
    refreshedList: (name: string) => `${name}: a lista foi baixada novamente.`,
    replace: 'Substituir o endereço',
    newManifestUrl: 'Novo endereço do manifesto',
    replaceHint:
      'Use isto depois de reconfigurar o complemento na página de configuração dele. As bibliotecas dele são mantidas.',
    replaceSubmit: 'Substituir',
    replaced: 'O endereço do manifesto foi substituído.',
    language: 'Idioma das descrições',
    languageHelp:
      'Deixe em Nenhum no único complemento de metadados do servidor, seja qual for o idioma definido na página de configuração dele: um complemento com idioma descreve os títulos apenas para os usuários que escolheram esse idioma, a menos que nenhum complemento deixado em Nenhum os descreva. Para que os usuários leiam os títulos em outros idiomas, adicione o mesmo complemento de metadados mais uma vez para cada idioma, configurado nesse idioma na página de configuração dele, e indique o idioma aqui somente nessas cópias extras: os usuários que escolherem esse idioma dos metadados verão os nomes, sinopses e imagens dele. Um complemento com idioma e sem biblioteca própria apenas descreve títulos.',
    languageNone: 'Nenhum',
    languageSaved: (name: string) => `${name}: o idioma das descrições foi salvo.`,
    edit: 'Editar a conta',
    saved: 'Fonte salva.',
    settings: 'Configurações',
    remove: 'Remover',
    removeTitle: (name: string) => `Remover ${name}?`,
    removeShared:
      'As bibliotecas dela vão desaparecer dos aplicativos Jellyfin de todos os usuários. Isso não pode ser desfeito.',
    removeMine:
      'As bibliotecas dela vão desaparecer dos seus aplicativos Jellyfin. Isso não pode ser desfeito.',
    removed: (name: string) => `${name} foi removida.`,
    readOnly: (name: string) =>
      `Esta é a fonte pessoal de ${name}. Somente essa pessoa pode alterá-la, na página Minhas fontes.`,
    back: 'Fontes',
    backMine: 'Minhas fontes',
    notFoundTitle: 'Fonte não encontrada',
    notFound: 'Esta fonte não existe ou não é uma que você possa gerenciar.',
  },
  sourceAdd: {
    title: 'Adicionar uma fonte',
    kind: 'Tipo de fonte',
    kinds: {
      stremio: 'Complemento Stremio',
      music: 'Complemento Eclipse',
      iptv: 'Fonte IPTV',
      tuner: 'Sintonizador HDHomeRun',
      local: 'Pasta',
    },
    stremioHelp:
      'Um complemento Stremio traz catálogos, metadados e streams. Cole o link de instalação dele.',
    musicHelp:
      'Um complemento de música Eclipse traz músicas, audiolivros ou podcasts. Cole o endereço do manifesto dele.',
    manifestUrl: 'Endereço do manifesto',
    manifestUrlPlaceholder: 'https://…/manifest.json',
    manifestUrlHint:
      'Copie o link de instalação na página de configuração do complemento. Ele costuma conter suas configurações ou chaves: o Polyfin o mantém privado e só exibe uma forma abreviada.',
    install: 'Instalar',
    installed: (name: string) => `${name} foi instalado.`,
    installedNoLibraries: (name: string) =>
      `${name} foi instalado. Nenhum dos catálogos dele é uma biblioteca ainda: ative-os em Conteúdo › Bibliotecas.`,
    installedNoLibrariesMine: (name: string) =>
      `${name} foi instalado. Nenhum dos catálogos dele é uma biblioteca ainda: ative-os em Minhas fontes.`,
    installedLibraryLimit: (name: string, limit: number) =>
      `${name} foi instalado. O servidor já tem ${limit} bibliotecas, o máximo que o Polyfin cria automaticamente, então nenhum dos catálogos dele virou uma: ative-os em Conteúdo › Bibliotecas.`,
    installedLibraryLimitMine: (name: string, limit: number) =>
      `${name} foi instalado. Você já tem ${limit} bibliotecas, o máximo que o Polyfin cria automaticamente, então nenhum dos catálogos dele virou uma: ative-os em Minhas fontes.`,
    installedMyMusic: (name: string) =>
      `${name} foi instalado. Ele não tem catálogos próprios, então a biblioteca dele é Minha música, que começa vazia: ela lista o que cada usuário ouve ou marca como favorito. Encontre a música dele com a busca dos aplicativos.`,
    iptvHelp:
      'Importe diretamente os canais de uma lista de reprodução M3U ou de uma conta Xtream Codes. Eles aparecem na TV ao Vivo como um catálogo de TV, que você pode ordenar em Bibliotecas. A lista é baixada novamente no intervalo definido em Configurações › TV ao Vivo.',
    steps: 'Etapas',
    stepAccount: 'Conta',
    stepImport: 'O que importar',
    name: 'Nome',
    account: 'Conta',
    m3u: 'Lista de reprodução M3U',
    xtream: 'Conta Xtream Codes',
    playlistUrl: 'Endereço da lista de reprodução',
    playlistUrlHint:
      'O endereço M3U fornecido pelo seu provedor. Ele pode conter seus dados de login: nunca é exibido por completo.',
    playlistUrlOrFile: 'Endereço ou arquivo da lista de reprodução',
    playlistUrlOrFileHint:
      'O endereço M3U fornecido pelo seu provedor, ou o caminho de um arquivo do servidor, como /iptv/custom.m3u, ou C:\\IPTV\\custom.m3u no Windows. Com Docker, o arquivo precisa estar em um volume montado, com seu caminho dentro do contêiner. Um endereço pode conter seus dados de login: nunca é exibido por completo.',
    server: 'Endereço do servidor',
    serverHint: 'Por exemplo https://servidor:8080, como fornecido pelo seu provedor.',
    username: 'Nome de usuário',
    password: 'Senha',
    keepHint: 'Deixe em branco para manter o atual.',
    passwordKeepHint: 'Deixe em branco para manter a senha atual.',
    userAgent: 'User agent (opcional)',
    userAgentHint:
      'Em branco, os canais são solicitados como um navegador da web os solicita. Informe o que seu provedor pede quando ele só atende certos players: a lista e os canais passam a ser solicitados com ele. Um user agent que a lista dá a um canal tem prioridade.',
    providerGuide: 'Usar o guia de programação do provedor',
    providerGuideHelp:
      'Os servidores Xtream Codes publicam um guia para a conta: ele preenche a programação dos canais.',
    guideUrl: 'Endereço do guia de programação (XMLTV, opcional)',
    guideUrlHint:
      'Arquivo XMLTV simples ou compactado. Você também pode adicioná-lo ou alterá-lo depois em Bibliotecas.',
    guideUrlOrFile: 'Endereço ou arquivo do guia de programação (XMLTV, opcional)',
    guideUrlOrFileHint:
      'Arquivo XMLTV simples ou compactado, pelo endereço ou pelo caminho no servidor. Você também pode adicioná-lo ou alterá-lo depois em Bibliotecas.',
    next: 'Próximo: escolher o que importar',
    back: 'Voltar para a conta',
    import: 'Importar',
    added: (name: string, parts: string[]) => `${name} foi adicionada: ${parts.join(', ')}.`,
    channels: (count: string) => `${count} canais`,
    movies: (count: string) => `${count} filmes`,
    series: (count: string) => `${count} séries`,
    tunerHelp:
      'Um sintonizador HDHomeRun na sua rede traz os canais que ele recebe por antena ou cabo. Eles aparecem na TV ao Vivo como os de uma fonte IPTV, e o aparelho reproduz tantos canais ao mesmo tempo quanto o número de sintonizadores que possui.',
    findTuners: 'Procurar sintonizadores',
    findTunersHelp:
      'Pergunta à rede local quais sintonizadores estão nela; isso leva alguns segundos.',
    finding: 'Consultando a rede local…',
    foundTitle: 'Sintonizadores na rede',
    tunerCount: (count: number, formatted: string) =>
      count === 1 ? '1 sintonizador' : `${formatted} sintonizadores`,
    addTuner: 'Adicionar',
    addTunerLabel: (name: string) => `Adicionar ${name}`,
    tunerAdded: 'Adicionado',
    openTuner: (name: string) => `Abrir ${name}`,
    noTuners: 'Nenhum sintonizador respondeu.',
    noTunersHelp:
      'No Docker, o Polyfin só detecta sintonizadores com a rede do host (network_mode: host). Você também pode adicionar um sintonizador pelo endereço.',
    byAddress: 'Adicionar pelo endereço',
    tunerAddress: 'Endereço do sintonizador',
    tunerAddressHint: 'O endereço IP dele, como 192.168.1.20, ou o endereço http://.',
    tunerAddressInvalid:
      'Digite o endereço IP do sintonizador, como 192.168.1.20, ou o endereço http:// dele.',
    tunerName: 'Nome (opcional)',
    tunerNameHint: 'Se ficar em branco, será usado o nome do próprio sintonizador.',
    favoritesOnly: 'Somente os canais favoritos do sintonizador',
    favoritesOnlyHelp:
      'Mantém os canais marcados como favoritos no sintonizador, no aplicativo ou na página web dele, e deixa os outros de fora.',
  },
  localFolders: {
    help: {
      container:
        'Uma pasta montada no contêiner do Polyfin (somente leitura é suficiente). Os vídeos dela são associados a títulos pelos nomes e reproduzidos como versões desses títulos, ao lado das streams dos complementos; uma pasta de músicas se torna uma biblioteca de músicas. O Polyfin é executado como o usuário 65532, que precisa ter permissão de leitura nela.',
      computer:
        'Uma pasta do computador em que o Polyfin é executado, ou de um disco ou compartilhamento que ele acessa (somente leitura é suficiente). Os vídeos dela são associados a títulos pelos nomes e reproduzidos como versões desses títulos, ao lado das streams dos complementos; uma pasta de músicas se torna uma biblioteca de músicas. A conta com que o Polyfin é executado precisa ter permissão de leitura nela.',
    },
    location: 'Local',
    locations: {
      local: 'Pasta local',
      smb: 'Compartilhamento SMB',
      webdav: 'Pasta WebDAV',
      s3: 'Bucket S3',
      sftp: 'Pasta SFTP',
      rclone: 'Remote do rclone',
    },
    shareHelp: {
      smb: 'Uma pasta compartilhada via SMB (compartilhamento de arquivos do Windows) por um NAS ou outro computador, que o próprio Polyfin lê, sem montá-la. Os vídeos dela são associados a títulos pelos nomes e reproduzidos como versões desses títulos, ao lado das streams dos complementos; uma pasta de músicas se torna uma biblioteca de músicas.',
      webdav:
        'Uma pasta que um servidor compartilha via WebDAV, que o próprio Polyfin lê, sem montá-la. Os vídeos dela são associados a títulos pelos nomes e reproduzidos como versões desses títulos, ao lado das streams dos complementos; uma pasta de músicas se torna uma biblioteca de músicas.',
      s3: 'Um prefixo de um bucket de armazenamento compatível com S3, que o próprio Polyfin lê, sem montá-lo. Os vídeos dele são associados a títulos pelos nomes e reproduzidos como versões desses títulos, ao lado das streams dos complementos, lidos pelo cache dos arquivos em leitura; uma pasta de músicas se torna uma biblioteca de músicas.',
      sftp: 'Uma pasta de um servidor acessado via SSH, que o próprio Polyfin lê, sem montá-la. Os vídeos dela são associados a títulos pelos nomes e reproduzidos como versões desses títulos, ao lado das streams dos complementos, lidos pelo cache dos arquivos em leitura; uma pasta de músicas se torna uma biblioteca de músicas.',
      rclone:
        'Uma pasta de um dos armazenamentos que o rclone acessa, lida por meio de uma instância do rclone executando rclone rcd --rc-serve, como um contêiner ao lado do Polyfin. Os vídeos dela são associados a títulos pelos nomes e reproduzidos como versões desses títulos, ao lado das streams dos complementos, lidos pelo cache dos arquivos em leitura; uma pasta de músicas se torna uma biblioteca de músicas.',
    },
    address: 'Endereço',
    addressHint: {
      smb: 'smb:// seguido do servidor, do compartilhamento e da pasta dentro dele, se houver. Adicione :porta após o servidor para usar uma porta diferente de 445.',
      webdav:
        'O endereço WebDAV da pasta. Prefira https://, pois http:// envia a senha sem criptografia.',
      s3: 's3:// seguido do bucket e do prefixo dentro dele, se houver. Os objetos dele são listados como pastas por /.',
      sftp: 'sftp:// seguido do servidor e do caminho completo da pasta nele. Adicione :porta após o servidor para usar uma porta diferente de 22.',
      rclone: '',
    },
    addressPlaceholder: {
      smb: 'smb://nas.local/media/filmes',
      webdav: 'https://nas.local/dav/filmes',
      s3: 's3://media/filmes',
      sftp: 'sftp://nas.local/srv/media/filmes',
      rclone: '',
    },
    user: 'Usuário',
    userHint: {
      smb: 'Vazio para um compartilhamento aberto a convidados. DOMÍNIO\\usuário para uma conta de domínio.',
      webdav: 'Vazio para uma pasta aberta a todos.',
      s3: 'Ambos vazios para um bucket que qualquer pessoa pode ler.',
      sftp: 'A conta com que o Polyfin entra.',
      rclone: 'O usuário do controle remoto do rclone (--rc-user), vazio se não houver.',
    },
    password: 'Senha',
    passwordHint: 'O Polyfin nunca a exibe novamente.',
    sftpPasswordHint:
      'Vazio quando o login é feito pela chave privada. O Polyfin nunca a exibe novamente.',
    accessKey: 'Chave de acesso',
    secretKey: 'Chave secreta',
    endpoint: 'Endpoint',
    endpointHint:
      'O endereço do serviço S3 do armazenamento. O Polyfin indica o bucket no caminho, como todo armazenamento compatível com S3 aceita, exceto nos endpoints da própria Amazon.',
    region: 'Região',
    regionHint: 'Vazio para us-east-1, que a maioria dos armazenamentos compatíveis com S3 aceita.',
    privateKey: 'Chave privada',
    privateKeyHint:
      'Uma chave privada OpenSSH ou PEM, no lugar da senha ou junto com ela. O Polyfin nunca a exibe novamente.',
    privateKeySaved: 'Há uma chave salva. Cole outra para substituí-la.',
    privateKeyNotSent:
      'A chave salva não é enviada a outro servidor: cole-a novamente, ou ela será removida ao salvar.',
    secretNotSent:
      'O valor salvo não é enviado a outro servidor: digite-o novamente, ou ele será removido ao salvar.',
    privateKeyRemove: 'Remover a chave salva',
    passphrase: 'Frase secreta',
    passphraseHint: 'Somente para uma chave privada que tenha uma.',
    hostKey: 'Chave do host',
    hostKeyHint:
      'Salva na primeira conexão. O Polyfin recusa o servidor quando ele apresenta outra.',
    hostKeyNone: 'Salva na primeira conexão',
    hostKeyOffered: (fingerprint: string) =>
      `O servidor agora apresenta outra chave do host, ${fingerprint}, e o Polyfin a recusa. Aceite-a somente se o servidor foi reinstalado ou se a chave dele foi alterada intencionalmente.`,
    acceptHostKey: 'Aceitar a nova chave',
    hostKeyAccepted: 'Nova chave do host aceita. O Polyfin rastreia a pasta novamente.',
    rcloneInstance: 'Endereço do rclone',
    rcloneInstanceHint:
      'Onde o rclone rcd --rc-serve responde, acessado via http://, como http://rclone:5572 para um contêiner ao lado do Polyfin.',
    rcloneRemote: 'Remote',
    rcloneRemoteHint: 'Um dos remotes da configuração do rclone.',
    listRemotes: 'Listar os remotes',
    remotesListed: (count: number, figure: string) =>
      count === 1 ? 'O rclone tem um remote.' : `O rclone tem ${figure} remotes.`,
    noRemotes: 'O rclone não tem nenhum remote na configuração.',
    remotesFailed: {
      unreachable: 'O rclone não responde neste endereço.',
      refused: 'O rclone recusa este usuário ou senha.',
    } as Record<string, string>,
    rcloneFolder: 'Caminho no remote',
    rcloneFolderHint: 'A pasta do remote a ser lida, vazio para a raiz.',
    pollMinutes: 'Procurar alterações a cada (minutos, 0 = só nos rastreamentos)',
    pollMinutesHint: {
      s3: 'As pastas do bucket são listadas novamente nesse intervalo para encontrar os arquivos adicionados e removidos. Os provedores podem cobrar por cada listagem. 30 por padrão.',
      sftp: 'A pasta e suas subpastas são listadas novamente nesse intervalo para encontrar os arquivos adicionados e removidos. 2 por padrão.',
      rclone:
        'As pastas do remote são listadas novamente nesse intervalo para encontrar os arquivos adicionados e removidos. 2 por padrão.',
    },
    name: 'Nome',
    nameHint: 'O nome da biblioteca nos aplicativos Jellyfin.',
    path: { container: 'Caminho no contêiner', computer: 'Caminho' },
    pathHint: {
      container: 'Onde a pasta está montada no contêiner do Polyfin, não o caminho dela no host.',
      linux: 'O caminho completo da pasta no computador em que o Polyfin é executado.',
      macos:
        'O caminho completo da pasta no Mac em que o Polyfin é executado; as pastas de um disco externo ficam em /Volumes.',
      windows:
        'O caminho completo da pasta no PC em que o Polyfin é executado: em uma unidade, como D:\\Filmes, ou em um compartilhamento, como \\\\nas\\media\\Filmes.',
    },
    pathPlaceholder: {
      container: '/media/filmes',
      linux: '/srv/media/filmes',
      macos: '/Users/eu/Movies',
      windows: 'D:\\Filmes',
    },
    kind: 'Contém',
    kinds: { movies: 'Filmes', shows: 'Séries', music: 'Música', homevideos: 'Vídeos caseiros' },
    kindHelp: {
      movies: 'Um arquivo por filme, sozinho ou em uma pasta própria, com o nome do filme e o ano.',
      shows:
        'Uma pasta por série, com a temporada e o episódio no nome de cada arquivo, como S01E02.',
      music:
        'Arquivos de áudio lidos pelas tags ou, na falta delas, pelos nomes, como Artista/Álbum/01 Título.flac, agrupados em álbuns e artistas.',
      homevideos:
        'Cada vídeo é um título próprio, com o nome do arquivo, ou da pasta quando está sozinho nela, e datado pelo arquivo NFO, pela gravação ou pelo arquivo. Nenhum complemento de metadados é consultado.',
    },
    kindNouns: {
      movies: 'filmes',
      shows: 'séries',
      music: 'música',
      homevideos: 'vídeos caseiros',
    },
    changeKind: 'Alterar',
    changeKindTitle: (name: string) => `Alterar o conteúdo de ${name}?`,
    changeKindScan: (from: string, to: string) =>
      `Contém ${from}; passará a conter ${to}. A pasta e a biblioteca mantêm os identificadores, o endereço e as configurações, e o Polyfin a rastreia novamente como ${to}, em segundo plano.`,
    changeKindFollows:
      'O que os usuários fizeram com cada vídeo (estado de reprodução, ponto de retomada e favorito) acompanha o vídeo para o novo título: as marcas de reproduzido e os favoritos são copiados, e o título anterior os mantém; o ponto de retomada acompanha o arquivo, e o título anterior o perde.',
    changeKindNothing:
      'Nada é transferido: músicas e vídeos são outros arquivos. O que os usuários fizeram com os títulos continua vinculado a eles e deixa de ser exibido.',
    changeKindBack:
      'Os vídeos caseiros ficam guardados por 30 dias: se a pasta voltar a conter vídeos caseiros nesse período, eles serão listados como os mesmos títulos, com o que os usuários fizeram com eles.',
    linksDropped: (count: number, figure: string) =>
      count === 1
        ? 'a associação feita manualmente'
        : `as ${figure} associações feitas manualmente`,
    episodesDropped: (count: number, figure: string) =>
      count === 1
        ? 'os episódios atribuídos ao título de um disco'
        : `os episódios atribuídos a ${figure} títulos de discos`,
    changeKindDrops: (parts: string[]) =>
      `Removidos, pois foram feitos para o conteúdo atual: ${parts.join(' e ')}.`,
    useArtwork: 'Usar as imagens da pasta',
    useArtworkHelp:
      'Pôsteres, imagens de fundo e logos ao lado dos arquivos, como poster.jpg ou Filme (2001)-fanart.jpg, são exibidos no lugar dos complementos de metadados para os respectivos títulos, e as imagens dos episódios no lugar das capturas. Desativado, são exibidas as dos complementos.',
    readExtras: 'Ler extras e mídias de tema',
    readExtrasHelp:
      'Extras, trailers locais e músicas de tema ao lado dos títulos, nomeados como no Jellyfin, como Extras/Making of.mkv, Filme (2001)-trailer.mkv ou theme.mp3, aparecem nos recursos especiais dos títulos, são reproduzidos pelo botão Trailer ou ao fundo das páginas. Desativado, o próximo rastreamento os ignora.',
    add: 'Adicionar a pasta',
    added: (name: string) => `${name} foi adicionada. O primeiro rastreamento foi iniciado.`,
    edit: 'Editar a pasta',
    save: 'Salvar',
    cancel: 'Cancelar',
    saved: 'Pasta salva.',
    movedHint:
      'Um novo caminho esquece os arquivos encontrados no anterior, incluindo as associações.',
    movedShareHint:
      'Um novo endereço esquece os arquivos encontrados no anterior, incluindo as associações.',
    scan: 'Rastrear agora',
    scanLabel: (name: string) => `Rastrear ${name} agora`,
    scanStarted: (name: string) => `${name}: o rastreamento foi iniciado.`,
    scanning: 'Rastreando',
    unreadable: 'Não pode ser lida',
    figuresTitle: 'Arquivos',
    files: 'Arquivos de vídeo',
    matched: 'Associados a um título',
    unmatched: 'Não associados',
    extras: 'Extras',
    tracks: 'Faixas',
    albums: 'Álbuns',
    artists: 'Artistas',
    gained: 'Com ganho',
    measured: 'Medidas',
    leftOut: 'Ignorados',
    videos: 'Vídeos',
    datedBy: {
      nfo: 'Datados por um arquivo NFO',
      created: 'Datados pela gravação',
      file: 'Datados pelo arquivo',
    },
    matchedBy: (parts: string[]) => `Correspondidos: ${parts.join(', ')}.`,
    matchMethods: {
      name: (count: string) => `${count} por um identificador no nome`,
      nfo: (count: string) => `${count} por um arquivo NFO`,
      search: (count: string) => `${count} por uma pesquisa`,
      year: (count: string) => `${count} por uma pesquisa, de um ano de diferença`,
      numerals: (count: string) => `${count} por uma pesquisa, com os números lidos da mesma forma`,
      yearNumerals: (count: string) =>
        `${count} por uma pesquisa, de um ano de diferença e com os números lidos da mesma forma`,
      link: (count: string) => `${count} manualmente`,
      tmdb: (count: string) =>
        `${count} por uma pesquisa no TMDB, porque as pesquisas dos complementos de metadados falharam`,
    },
    holds: 'Conteúdo',
    lastScan: 'Último rastreamento',
    lastAttempt: 'Última tentativa',
    never: 'Ainda não',
    errorTitle: 'O Polyfin não consegue ler esta pasta',
    errorTitleShare: 'O Polyfin não consegue ler este compartilhamento',
    errors: {
      missing:
        'O caminho não existe onde o Polyfin é executado: verifique se a pasta, ou o disco ou compartilhamento em que ela está, está presente e, no Docker, se está montada no contêiner.',
      unreadable:
        'A pasta existe, mas o Polyfin não consegue listá-la: dê à conta com a qual ele é executado (usuário 65532 no Docker) permissão para lê-la e abrir suas pastas.',
      not_folder: 'Este caminho é um arquivo, não uma pasta.',
    } as Record<string, string>,
    shareErrors: {
      missing: 'O compartilhamento, ou a pasta nele, não existe: verifique o endereço.',
      unreadable:
        'O usuário do compartilhamento não pode listar esta pasta: verifique as permissões dele no servidor.',
      not_folder: 'Este endereço é um arquivo, não uma pasta.',
      unreachable:
        'O Polyfin não consegue acessar o compartilhamento: verifique se o servidor está ligado, e o servidor e a porta do endereço.',
      refused:
        'O compartilhamento recusa o usuário ou a senha: verifique-os e digite-os novamente.',
      host_key_changed:
        'O servidor SFTP apresenta uma chave de host diferente da salva na primeira conexão, e o Polyfin a recusa. Se a mudança for esperada, aceite a nova chave em Editar a pasta.',
    } as Record<string, string>,
    keptNote:
      'Os arquivos encontrados antes permanecem na biblioteca até que a pasta possa ser lida novamente.',
    unmatchedTitle: 'Arquivos sem correspondência',
    unmatchedHelp:
      'Nenhum título foi encontrado para estes arquivos. Renomeie-os com o título e o ano, ou adicione um arquivo NFO com o identificador do título e depois rastreie; ou vincule-os ao identificador IMDb presente no endereço IMDb do título (tt seguido de dígitos). Um vínculo sobrevive a novos rastreamentos; em uma pasta de séries, ele abrange a série inteira e, para um disco Blu-ray ou um DVD, o disco inteiro, ou um filme de um disco que contém vários. Os títulos do disco de uma série recebem o episódio conforme indicado no nome de um arquivo, como S01E05, ou S01E05-E06 para dois; os episódios sugeridos para um disco seguem os dos discos anteriores da temporada.',
    noUnmatched: 'Todos os arquivos correspondem a um título.',
    leftOutTitle: 'Faixas deixadas de fora',
    leftOutHelp:
      'Nem as tags nem os nomes destes arquivos indicam um título ou um álbum. Adicione tags a eles, ou mova-os para uma pasta Artista/Álbum, e depois rastreie.',
    noLeftOut: 'Todas as faixas estão em um álbum.',
    shownOf: (shown: string, total: string) => `${shown} exibidos de ${total}.`,
    reasons: {
      unreadable_name: 'O nome não indica nenhum título ou número de episódio',
      disc_title: 'Outro filme do disco: vincule-o ao seu título',
      disc_episode: 'Um título do disco de uma série: informe o episódio',
      lone_extra: 'Um extra sem o seu título ao lado: vincule-o para torná-lo um título',
      not_found: 'Os complementos de metadados não conhecem nenhum título com este nome',
      no_search: 'Nenhum complemento do servidor descreve e pesquisa títulos deste tipo',
      ambiguous: 'Vários títulos têm este nome: adicione o ano a ele, ou vincule-o',
      other_year: 'Os títulos com este nome são de outros anos',
      search_failed: 'Não foi possível pesquisar nos complementos de metadados',
      no_title: 'As tags e o nome não indicam nenhum título',
      no_album: 'As tags e as pastas não indicam nenhum álbum',
    } as Record<string, string>,
    searchFailedBy: (addon: string, failure: string) =>
      failure === '' ? `A pesquisa de ${addon} falhou` : `A pesquisa falhou: ${addon} ${failure}`,
    searchFailures: {
      unreachable: 'não pôde ser acessado',
      timeout: 'não respondeu a tempo',
      private_network: 'está em uma rede local',
      rate_limited: 'pediu para diminuir o ritmo (requisições demais)',
      server_error: 'respondeu com um erro do servidor',
      http_error: 'recusou a requisição',
      invalid_response: 'enviou uma resposta que o Polyfin não consegue ler',
    } as Record<string, string>,
    noSearchTitle: (count: number, figure: string) =>
      count === 1
        ? 'Nenhum complemento consegue pesquisar o título de 1 arquivo'
        : `Nenhum complemento consegue pesquisar os títulos de ${figure} arquivos`,
    noSearchHelp:
      'A associação precisa de um complemento do servidor, em Conteúdo › Fontes, que descreva esses títulos e tenha um catálogo de pesquisa. Os complementos próprios dos usuários, em Minhas fontes, não contam. Depois de adicionar um, rastreie a pasta.',
    searchFailedTitle: (count: number, figure: string) =>
      count === 1
        ? 'Não foi possível pesquisar o título de 1 arquivo'
        : `Não foi possível pesquisar os títulos de ${figure} arquivos`,
    searchRetry: (at: string) =>
      `O Polyfin pesquisa de novo sozinho, a próxima vez em ${at}, depois com menos frequência enquanto a pesquisa falhar, e a cada rastreamento.`,
    searchIdentifier: (shows: boolean) =>
      shows
        ? 'Para associar uma série sem pesquisa, adicione o identificador do IMDb ao nome da pasta dela, como em “Série (2010) [imdbid-tt0123456]”, e depois escaneie.'
        : 'Para associar um filme sem pesquisa, adicione o identificador do IMDb ao nome do arquivo dele, ou da própria pasta, como em “Filme (2010) [imdbid-tt0123456]”, e depois escaneie.',
    searchHelp:
      'A associação precisa de um complemento do servidor que descreva esses títulos e tenha um catálogo de pesquisa. Se as pesquisas dele continuarem falhando, verifique-o em Sistema › Integridade, ou associe os arquivos manualmente abaixo.',
    read: (title: string) => `Lido como “${title}”`,
    showFolder: (unit: string) => `Pasta da série: ${unit}`,
    discTitle: (disc: string, title: number) => `${disc} · título ${title}`,
    disc: {
      bluray: (unit: string) => `Disco Blu-ray: ${unit}`,
      dvd: (unit: string) => `DVD: ${unit}`,
      bluray_image: (unit: string) => `Imagem de Blu-ray: ${unit}`,
      dvd_image: (unit: string) => `Imagem de DVD: ${unit}`,
    },
    minutes: (minutes: number) => `${minutes} min`,
    episodeLabel: (title: string) => `Episódio de ${title}`,
    episodePlaceholder: 'S01E01',
    giveEpisode: 'Salvar',
    episodesGiven: (count: number) =>
      count === 1 ? 'O título foi numerado.' : `${count} títulos foram numerados.`,
    suggestion: (disc: string, first: string, last: string) =>
      first === last
        ? `Sugerido para ${disc}: ${first}`
        : `Sugeridos para ${disc}: ${first} a ${last}`,
    confirmSuggestion: 'Usar estes episódios',
    imdbId: 'Identificador IMDb',
    imdbPlaceholder: 'tt0000000',
    link: 'Vincular',
    linkLabel: (path: string) => `Identificador IMDb para ${path}`,
    linked: (unit: string, id: string) => `${unit} está vinculado a ${id}.`,
    findTitle: 'Encontrar o título',
    searchLabel: (path: string) => `Pesquisa de título para ${path}`,
    searchQuery: 'Título',
    searchYear: 'Ano',
    search: 'Pesquisar',
    searching: 'Pesquisando nos complementos de metadados…',
    noTitles:
      'Os complementos de metadados não encontraram nenhum título com este nome que tenha um identificador IMDb. Altere a pesquisa ou digite o identificador.',
    titlesFound: (query: string) => `Títulos encontrados para “${query}”`,
    titleKinds: { movie: 'Filme', show: 'Série' },
    pickTitle: (name: string, details: string) => `Vincular a ${name}, ${details}`,
    unlistedTitle: (count: number, figure: string) =>
      count === 1 ? '1 episódio está só nos arquivos' : `${figure} episódios estão só nos arquivos`,
    unlistedHelp:
      'O addon de metadados que descreve as séries deles não os lista, como quando numera de outra forma as temporadas ou os especiais. Eles aparecem mesmo assim, com o nome do arquivo, e são reproduzidos.',
    linkNotDescribedShow:
      'Nenhum addon de metadados do servidor descreve este identificador como uma série. Verifique se é o da série, não o de um episódio.',
    linkNotDescribedMovie:
      'Nenhum addon de metadados do servidor descreve este identificador como um filme. Verifique se é o do filme.',
    linksTitle: 'Vinculados manualmente',
    unlink: 'Remover o vínculo',
    unlinkLabel: (unit: string) => `Remover o vínculo de ${unit}`,
    unlinked: (unit: string) =>
      `O vínculo de ${unit} foi removido: ele volta a ser identificado pelo nome ou pelo arquivo NFO.`,
    removeEpisodeLabel: (title: string) => `Remover o episódio de ${title}`,
    episodeRemoved: (title: string) =>
      `O episódio de ${title} foi removido: o nome do disco define a numeração, ou ele volta a ser sugerido.`,
    looserTitle: 'Identificados por uma regra mais flexível',
    looserHelp:
      'Nenhum título tem exatamente o nome e o ano destes arquivos, então o Polyfin os identificou com o único título que uma regra mais flexível deixa: um do ano anterior ou seguinte, ou um com o mesmo nome depois de lidos os números da mesma forma, como 2 para II ou Part 1 para Part One. Confira-os. Desvincule um errado: ele passa a ser identificado só pelo nome e ano exatos, e fica sem correspondência até que você o vincule ao título.',
    looserRules: {
      year: 'Um título de um ano de diferença',
      numerals: 'Números lidos da mesma forma',
      year_numerals: 'Um título de um ano de diferença, números lidos da mesma forma',
    } as Record<string, string>,
    matchedTo: (title: string) => `Identificado como “${title}”`,
    fileCount: (count: number, figure: string) =>
      count === 1 ? '1 arquivo' : `${figure} arquivos`,
    looserUnlinkLabel: (unit: string) => `Desvincular ${unit} do título`,
    looserUnlinked: (unit: string) =>
      `${unit} foi desvinculado: ele passa a ser identificado só pelo nome e ano exatos.`,
    openPage: 'Abrir a página da pasta',
  },
  mySources: {
    lede: 'Adicione suas próprias fontes Stremio, Eclipse e IPTV e escolha quais dos catálogos delas aparecem como bibliotecas nos seus aplicativos Jellyfin.',
    preferenceTitle: 'As fontes do servidor',
    useShared: 'Usar as fontes do servidor',
    useSharedHelp:
      'Ativado: você recebe as fontes e bibliotecas do servidor, seguidas das suas. Desativado: você recebe apenas suas próprias fontes e bibliotecas.',
    preferenceSaved: 'Preferência salva.',
    parentalControl:
      'O controle parental se aplica à sua conta: seus aplicativos Jellyfin mostram apenas as fontes e bibliotecas do servidor, pois elas fornecem as classificações etárias em que ele se baseia. Suas próprias fontes são mantidas, mas não são usadas.',
    personalAddonsOff:
      'Um administrador desativou suas próprias fontes: seus aplicativos Jellyfin mostram apenas as fontes e bibliotecas do servidor. Suas próprias fontes são mantidas, mas não são usadas.',
    listTitle: 'Suas fontes',
  },
  usedBy: {
    title: 'Usada por',
    help: 'Quem recebe esta fonte nos seus aplicativos Jellyfin: suas bibliotecas, seus títulos, versões, legendas e canais. Todos inclui os usuários criados depois; com Somente estes grupos e usuários, um novo usuário não recebe nada dela até que o grupo dele ou ele mesmo seja marcado.',
    everyone: 'Todos',
    listed: 'Somente estes grupos e usuários',
    groups: 'Grupos',
    users: 'Usuários',
    members: (count: number) => (count === 1 ? '1 membro' : `${count} membros`),
    noGroups: 'Nenhum grupo ainda.',
    nobody: 'Ninguém usa esta fonte até você marcar um grupo ou um usuário.',
    selfLeftOut: 'Você ainda gerencia esta fonte aqui, mas seus próprios aplicativos não a usarão.',
    ownAddons:
      'Um usuário que pode adicionar as próprias fontes ainda pode adicionar o mesmo complemento para si: desative “Pode adicionar os próprios complementos” na página dele ou na do grupo dele para impedir isso.',
    save: 'Salvar',
    cancel: 'Cancelar',
    saved: (name: string) => `${name}: quem a usa foi salvo.`,
    chip: (names: string) => `Usada por ${names}`,
    chipNone: 'Não usada por ninguém',
  },
  music: {
    content: { music: 'Música', audiobook: 'Audiolivros', podcast: 'Podcasts' } as Record<
      string,
      string
    >,
    library: {
      music: 'Biblioteca de música',
      audiobook: 'Biblioteca de livros',
      podcast: 'Biblioteca de música',
    } as Record<string, string>,
    settings: 'Configurações',
    settingsTitle: 'Configurações do complemento',
    settingsHelp:
      'Definidas pelo complemento e enviadas em cada solicitação que o Polyfin faz a ele. Podem conter os dados da sua conta: somente você, ou os administradores no caso dos complementos do servidor, pode vê-las.',
    noSettings: 'Este complemento não tem configurações.',
    defaultValue: (value: string) => `Padrão: ${value}`,
    defaultEmpty: 'Padrão: vazio',
    on: 'Ativado',
    off: 'Desativado',
    range: (min: string, max: string) => `De ${min} a ${max}.`,
    atLeast: (min: string) => `No mínimo ${min}.`,
    atMost: (max: string) => `No máximo ${max}.`,
    stepOf: (step: string) => `Em incrementos de ${step}.`,
    maxLength: (count: number) => `No máximo ${count} ${count === 1 ? 'caractere' : 'caracteres'}.`,
    tooLong: (count: number) =>
      `Use no máximo ${count} ${count === 1 ? 'caractere' : 'caracteres'}.`,
    belowMin: (min: string) => `Digite ${min} ou mais.`,
    aboveMax: (max: string) => `Digite ${max} ou menos.`,
    offStep: (step: string) => `Use um múltiplo de ${step}.`,
    resetDefaults: 'Usar os valores padrão',
    saved: 'Configurações salvas. O complemento as recebe a partir da próxima solicitação.',
  },
}

export default sources
