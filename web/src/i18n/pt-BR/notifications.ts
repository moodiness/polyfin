import type en from '../en/notifications'

const notifications: typeof en = {
  notifications: {
    title: 'Notificações',
    loading: 'Carregando os destinos…',
    empty: 'Nenhum destino ainda.',
    emptyHint: {
      server:
        'Adicione um destino, como um endereço de e-mail, um chat do Telegram ou um tópico do ntfy, para ser avisado sobre novos episódios, gravações e problemas de integridade.',
      own: 'Adicione um destino, como um endereço de e-mail, um chat do Telegram ou um tópico do ntfy, para ser avisado quando sair um novo episódio de uma série que você acompanha ou quando suas gravações terminarem.',
    },
    serverTargets: 'Destinos do servidor',
    serverTargetsHelp:
      'Eles recebem os eventos de todos os usuários: uma mensagem por novo episódio, independentemente de quem acompanha a série, cada gravação e cada reprodução. Cada usuário pode adicionar seus próprios destinos em Minha conta › Notificações.',
    add: 'Adicionar destino',
    addTitle: 'Adicionar destino',
    editTitle: (name: string) => `Editar ${name}`,
    kind: 'Tipo',
    kinds: {
      webhook: 'Webhook',
      discord: 'Discord',
      ntfy: 'ntfy',
      email: 'E-mail',
      telegram: 'Telegram',
      gotify: 'Gotify',
      pushover: 'Pushover',
    },
    kindHelp: {
      webhook:
        'O Polyfin envia cada evento para este endereço em JSON, conforme descrito na documentação para desenvolvedores de aplicativos.',
      discord:
        'O Polyfin envia cada evento para um canal do Discord, como uma mensagem com um link.',
      ntfy: 'O Polyfin publica cada evento em um tópico do ntfy, que o aplicativo ntfy exibe no seu celular.',
      email: 'O Polyfin envia cada evento por e-mail para este endereço, em texto simples e HTML.',
      telegram:
        'O Polyfin envia cada evento para um chat do Telegram por meio do seu bot, com um link.',
      gotify: 'O Polyfin envia cada evento para um aplicativo do seu servidor Gotify.',
      pushover:
        'O Polyfin envia cada evento para seus dispositivos Pushover, por meio de um dos seus aplicativos.',
    },
    emailUnavailable: {
      server:
        'O e-mail requer um servidor SMTP: salve um, com um endereço de remetente, em E-mail acima e depois adicione o destino.',
      own: 'O e-mail requer um servidor SMTP, que um administrador configura em Configurações › Notificações.',
    },
    name: 'Nome',
    nameHelp: 'Exibido somente aqui, de 1 a 64 caracteres.',
    webhookAddress: 'Endereço do webhook',
    discordAddress: 'Endereço do webhook do Discord',
    discordSwitched:
      'Este é um webhook do Discord: o destino passa a ser um destino do Discord, que envia a mensagem própria do Discord.',
    discordAsWebhook:
      'Este é um webhook do Discord: o Polyfin envia a ele a mensagem própria do Discord, como a um destino do Discord.',
    addressHelp: {
      webhook: 'Ele é armazenado criptografado e nunca mais é exibido: somente o host dele é.',
      discord:
        'No Discord: configurações do canal, Integrações, Webhooks, Copiar URL do webhook. Ele é armazenado criptografado e nunca mais é exibido.',
    },
    savedAddress: (host: string) => `Salvo, em ${host}.`,
    ntfyServer: 'Servidor ntfy',
    ntfyServerHelp: 'Deixe em branco para usar https://ntfy.sh.',
    topic: 'Tópico',
    topicHelp:
      'O tópico que você assina no aplicativo ntfy. Qualquer pessoa que conheça um tópico em um servidor público pode lê-lo: escolha um difícil de adivinhar.',
    token: 'Token de acesso',
    tokenHelp:
      'Para um tópico que exige login. Ele é armazenado criptografado e nunca mais é exibido.',
    emailAddress: 'Endereço de e-mail',
    emailAddressHelp: 'Para onde as mensagens vão.',
    chat: 'Chat',
    chatHelp:
      'O número do chat, como -1001234567890, ou o nome de um canal público, como @news_example. Adicione o bot ao chat primeiro.',
    botToken: 'Token do bot',
    botTokenHelp:
      'O BotFather o fornece quando o bot é criado. Ele é armazenado criptografado e nunca mais é exibido.',
    gotifyServer: 'Servidor Gotify',
    gotifyServerHelp: 'O endereço dele, como https://gotify.example.org.',
    appToken: 'Token do aplicativo',
    appTokenHelp: {
      gotify:
        'No Gotify: Apps, Create Application e depois copie o token. Ele é armazenado criptografado e nunca mais é exibido.',
      pushover:
        'O token da API de um aplicativo criado no Pushover. Ele é armazenado criptografado e nunca mais é exibido.',
    },
    userKey: 'Chave do usuário',
    userKeyHelp:
      'Exibida no painel do Pushover após entrar, ou uma chave de grupo. Ela é armazenada criptografada e nunca mais é exibida.',
    events: 'Eventos',
    eventsHelp: {
      server: 'Sobre o que ele é avisado, para todos os usuários.',
      own: 'Sobre o que ele é avisado, a seu respeito.',
    },
    eventNames: {
      server_crashed: 'Falha do servidor',
      playback_failed: 'Falha na reprodução',
      new_episode: 'Novos episódios',
      recording_finished: 'Gravação concluída',
      recording_failed: 'Falha na gravação',
      health_problem: 'Problema de integridade encontrado',
      health_solved: 'Problema de integridade resolvido',
      user_joined: 'Novo usuário',
      new_version: 'Nova versão',
      playback_started: 'Reprodução iniciada',
      playback_paused: 'Reprodução pausada',
      playback_resumed: 'Reprodução retomada',
      playback_stopped: 'Reprodução interrompida',
      weekly_summary: 'Resumo semanal',
      monthly_summary: 'Retrospectiva do mês',
      yearly_summary: 'Retrospectiva do ano',
      connection_stopped: 'Conexão interrompida',
      account_blocked: 'Conta bloqueada',
      new_device: 'Login em um novo dispositivo',
      share_played: 'Link compartilhado reproduzido',
      programme_reminder: 'Programa começando',
      title_available: 'Título para assistir disponível',
    },
    eventHelp: {
      server_crashed:
        'O Polyfin parou por um erro e foi iniciado novamente: avisado ao reiniciar, com o começo do rastreamento do erro, que Sistema › Integridade também mostra.',
      playback_failed:
        'Uma reprodução não conseguiu começar ou foi interrompida: nenhuma versão é reproduzida no aplicativo, um limite a recusou ou o fluxo falhou. Uma vez por hora por usuário e título, com o aplicativo e o motivo.',
      new_episode:
        'Saiu um episódio de uma série reproduzida ou marcada como favorita: uma vez por episódio, nunca para os que já saíram.',
      recording_finished:
        'Uma gravação da TV ao Vivo terminou, completa ou com uma parte faltando.',
      recording_failed: 'Uma gravação da TV ao Vivo não gravou nada.',
      health_problem: 'Sistema › Integridade encontrou um problema, em duas verificações seguidas.',
      health_solved: 'Um problema encontrado por Sistema › Integridade foi resolvido.',
      user_joined: 'Alguém criou a própria conta por meio de um link de convite.',
      new_version:
        'Saiu uma nova versão do Polyfin, encontrada pela verificação diária: uma vez por versão, com as notas da versão.',
      playback_started:
        'Um vídeo ou uma música começa a ser reproduzido em um aplicativo Jellyfin, com quem está reproduzindo, em qual dispositivo e se está sendo convertido.',
      playback_paused:
        'Uma reprodução foi pausada. As posições durante a reprodução nunca são enviadas.',
      playback_resumed: 'Uma reprodução pausada voltou a tocar.',
      playback_stopped:
        'Uma reprodução parou, ou o aplicativo dela deixou de informá-la por cinco minutos.',
      weekly_summary:
        'Uma vez por semana, no dia e na hora definidos pelo servidor: as horas assistidas, os títulos e os novos episódios da semana. Os destinos do servidor e dos administradores recebem a semana do servidor, com os usuários que entraram, os arquivos adicionados às pastas locais e os problemas encontrados; os demais recebem a própria semana. Nada é enviado em uma semana sem nada a relatar.',
      monthly_summary:
        'No dia 1º de cada mês, na hora do resumo semanal: o conteúdo do resumo semanal referente ao mês que terminou. Nada é enviado em um mês sem nada a relatar.',
      yearly_summary:
        'Em 1º de janeiro, na hora do resumo semanal: as horas assistidas, os dez filmes e as dez séries reproduzidos por mais tempo e o mês com mais horas do ano que terminou. Os destinos do servidor e dos administradores recebem o ano do servidor, com os usuários que entraram e os arquivos adicionados às pastas locais; os demais recebem o próprio ano. Nada é enviado em um ano sem nada a relatar.',
      connection_stopped:
        'Uma conexão de aplicativo ou um serviço de acompanhamento recusou o que o Polyfin guardava e foi interrompido: uma vez por interrupção, com o que fazer para conectá-lo novamente.',
      account_blocked:
        'Senhas incorretas bloquearam uma conta por um tempo: uma vez por bloqueio, com até quando e o endereço e o aplicativo da última tentativa. Os destinos do servidor e dos administradores são avisados sobre todas as contas; os demais, sobre a própria conta.',
      new_device:
        'Sua conta entrou por um aplicativo Jellyfin ou pelo cliente web em um dispositivo nunca usado antes, com o aplicativo, o dispositivo e o endereço. O primeiro dispositivo de uma conta não é avisado.',
      share_played:
        'Alguém sem conta iniciou um filme ou um episódio por um link de compartilhamento: no máximo uma vez por hora por link, com o título e quem o compartilhou. Os destinos do servidor são avisados sobre todos os links; os demais, sobre os próprios links.',
      programme_reminder:
        'Um programa da TV ao Vivo para o qual você definiu um lembrete no player web começa em breve, com a antecedência definida em Remind me before, com um link para o canal.',
      title_available:
        'Foram encontradas versões de um título da sua lista “Para assistir” para o qual as verificações anteriores não tinham encontrado nenhuma: uma vez por título. Uma versão encontrada ainda não foi testada.',
    },
    enabled: 'Enviar mensagens para este destino',
    create: 'Adicionar',
    creating: 'Adicionando…',
    save: 'Salvar',
    saving: 'Salvando…',
    created: (name: string) => `${name} adicionado.`,
    saved: (name: string) => `${name} salvo.`,
    test: 'Enviar um teste',
    testing: 'Enviando…',
    testDelivered: (name: string) => `${name} recebeu a mensagem de teste.`,
    testFailed: (name: string) => `${name} não aceitou a mensagem de teste.`,
    testAnswer: (answer: string) => `Resposta: ${answer}`,
    edit: 'Editar',
    delete: 'Excluir',
    deleteTitle: (name: string) => `Excluir ${name}?`,
    deleteBody:
      'As mensagens deixam de ser enviadas para ele, e as que estão aguardando são descartadas.',
    deleted: (name: string) => `${name} excluído.`,
    status: {
      working: 'Funcionando',
      waiting: 'Nada enviado ainda',
      off: 'Desativado',
      refused: 'Recusado',
      rejected: 'Mensagem recusada',
      unreachable: 'Inacessível',
      unreadable: 'Digite novamente',
    },
    answer: (code: string, email: boolean) => (email ? `SMTP ${code}` : `HTTP ${code}`),
    problem: {
      refused: (answer: string) =>
        `O destino recusou o Polyfin (${answer}): verifique o endereço, o token ou a chave.`,
      refusedEmail: (answer: string) =>
        `O servidor SMTP recusou o Polyfin (${answer}): verifique o usuário e a senha, o endereço do remetente e este endereço.`,
      rejected: (answer: string) => `O destino recusou a última mensagem (${answer}).`,
      unreachable:
        'Não foi possível entregar as mensagens recentes: o destino não respondeu ou respondeu com erros. Cada mensagem é tentada novamente durante uma hora.',
      unreadable:
        'O endereço, o token ou a chave não pode ser descriptografado com POLYFIN_SECRET_KEY: nada é enviado a ele até que seja digitado novamente.',
    },
    perMinute: 'Mensagens por minuto, no máximo',
    perMinuteHelp:
      'As mensagens seguintes aguardam a vez, em ordem. Vazio para nenhum limite; de 1 a 60.',
    groupMinutes: 'Agrupar reproduções e novos episódios, em minutos',
    groupMinutesHelp:
      'Reproduções, links compartilhados reproduzidos e novos episódios aguardam esse tempo e depois saem em uma única mensagem. Todo o resto continua saindo na hora. Vazio para enviar cada um na hora; de 1 a 1440.',
    capped: (n: number) => `No máximo ${n} por minuto`,
    grouped: (n: number) => `Agrupadas a cada ${n} min`,
    deliveries: {
      open: 'Envios recentes',
      title: (name: string) => `Envios recentes para ${name}`,
      help: 'As últimas 100 mensagens, da mais recente à mais antiga, guardadas por 30 dias.',
      loading: 'Carregando os envios…',
      empty: 'Nenhuma mensagem enviada ainda.',
      when: 'Quando',
      event: 'Mensagem',
      outcome: 'Resultado',
      test: 'Mensagem de teste',
      grouped: (n: number) => `${n} eventos em uma mensagem`,
      attempts: (n: number) => `${n} tentativas`,
      outcomes: {
        delivered: 'Entregue',
        refused: 'Recusado',
        rejected: 'Mensagem recusada',
        failed: 'Não entregue',
        dropped: 'Descartada',
        unreadable: 'Não pode ser descriptografado',
      },
    },
    preview: {
      title: 'Prévia',
      help: 'Veja um resumo como um destino de email o receberia agora, do servidor ou de um usuário. Nada é enviado.',
      summary: 'Resumo',
      summaries: {
        weekly_summary: 'Resumo semanal',
        monthly_summary: 'Retrospectiva do mês',
        yearly_summary: 'Retrospectiva do ano',
      },
      of: 'De',
      server: 'O servidor',
      show: 'Prévia',
      loading: 'Preparando a prévia…',
      empty: 'Nada a relatar neste período: este resumo não seria enviado.',
      frame: 'O email do resumo',
    },
    lastSent: 'Última mensagem ',
    nothingSent: 'Nenhuma mensagem enviada ainda',
    noEvents: 'Nenhum evento escolhido',
  },
}

export default notifications
