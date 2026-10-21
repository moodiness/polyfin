import type en from '../en/notifications'

const notifications: typeof en = {
  notifications: {
    title: 'Notifiche',
    loading: 'Caricamento delle destinazioni…',
    empty: 'Ancora nessuna destinazione.',
    emptyHint: {
      server:
        'Aggiungi una destinazione, come un indirizzo email, una chat Telegram o un argomento ntfy, per essere avvisato di nuovi episodi, registrazioni e problemi rilevati da Stato di salute.',
      own: 'Aggiungi una destinazione, come un indirizzo email, una chat Telegram o un argomento ntfy, per essere avvisato quando esce un nuovo episodio di una serie TV che segui o quando terminano le tue registrazioni.',
    },
    serverTargets: 'Destinazioni del server',
    serverTargetsHelp:
      'Ricevono gli eventi di tutti gli utenti: un messaggio per ogni nuovo episodio, chiunque segua la serie TV, ogni registrazione e ogni riproduzione. Ogni utente può aggiungere le proprie destinazioni in Il mio account › Notifiche.',
    add: 'Aggiungi una destinazione',
    addTitle: 'Aggiungi una destinazione',
    editTitle: (name: string) => `Modifica ${name}`,
    kind: 'Tipo',
    kinds: {
      webhook: 'Webhook',
      discord: 'Discord',
      ntfy: 'ntfy',
      email: 'Email',
      telegram: 'Telegram',
      gotify: 'Gotify',
      pushover: 'Pushover',
    },
    kindHelp: {
      webhook:
        'Polyfin invia ogni evento a questo indirizzo in formato JSON, descritto nella documentazione per gli sviluppatori di applicazioni.',
      discord: 'Polyfin pubblica ogni evento in un canale Discord, come messaggio con un link.',
      ntfy: 'Polyfin pubblica ogni evento in un argomento ntfy, che l’app ntfy mostra sul tuo telefono.',
      email: 'Polyfin invia ogni evento via email a questo indirizzo, in testo semplice e HTML.',
      telegram: 'Polyfin invia ogni evento a una chat Telegram tramite il tuo bot, con un link.',
      gotify: 'Polyfin invia ogni evento a un’applicazione del tuo server Gotify.',
      pushover:
        'Polyfin invia ogni evento ai tuoi dispositivi Pushover, tramite una delle tue applicazioni.',
    },
    emailUnavailable: {
      server:
        'L’email richiede un server SMTP: salvane uno, con un indirizzo del mittente, in Email qui sopra, poi aggiungi la destinazione.',
      own: 'L’email richiede un server SMTP, che un amministratore configura in Impostazioni › Notifiche.',
    },
    name: 'Nome',
    nameHelp: 'Mostrato solo qui, da 1 a 64 caratteri.',
    webhookAddress: 'Indirizzo del webhook',
    discordAddress: 'Indirizzo del webhook Discord',
    addressHelp: {
      webhook:
        'Viene conservato crittografato e non viene più mostrato: viene mostrato solo il suo host.',
      discord:
        'In Discord: impostazioni del canale, Integrazioni, Webhook, Copia URL webhook. Viene conservato crittografato e non viene più mostrato.',
    },
    savedAddress: (host: string) => `Salvato, su ${host}.`,
    ntfyServer: 'Server ntfy',
    ntfyServerHelp: 'Lascia vuoto per https://ntfy.sh.',
    topic: 'Argomento',
    topicHelp:
      'L’argomento a cui ti iscrivi nell’app ntfy. Su un server pubblico chiunque conosca un argomento può leggerlo: scegline uno difficile da indovinare.',
    token: 'Token di accesso',
    tokenHelp:
      'Per un argomento che richiede l’accesso. Viene conservato crittografato e non viene più mostrato.',
    emailAddress: 'Indirizzo email',
    emailAddressHelp: 'Dove vengono inviati i messaggi.',
    chat: 'Chat',
    chatHelp:
      'Il numero della chat, come -1001234567890, o il nome di un canale pubblico, come @news_example. Aggiungi prima il bot alla chat.',
    botToken: 'Token del bot',
    botTokenHelp:
      'Lo fornisce BotFather quando il bot viene creato. Viene conservato crittografato e non viene più mostrato.',
    gotifyServer: 'Server Gotify',
    gotifyServerHelp: 'Il suo indirizzo, come https://gotify.example.org.',
    appToken: 'Token dell’applicazione',
    appTokenHelp: {
      gotify:
        'In Gotify: Apps, Create Application, poi copia il suo token. Viene conservato crittografato e non viene più mostrato.',
      pushover:
        'Il token API di un’applicazione creata su Pushover. Viene conservato crittografato e non viene più mostrato.',
    },
    userKey: 'Chiave utente',
    userKeyHelp:
      'Mostrata nel pannello di controllo di Pushover dopo l’accesso, oppure una chiave di gruppo. Viene conservata crittografata e non viene più mostrata.',
    events: 'Eventi',
    eventsHelp: {
      server: 'Di cosa viene avvisata, per tutti gli utenti.',
      own: 'Di cosa viene avvisata, per quanto ti riguarda.',
    },
    eventNames: {
      new_episode: 'Nuovi episodi',
      recording_finished: 'Registrazione terminata',
      recording_failed: 'Registrazione non riuscita',
      health_problem: 'Problema rilevato',
      health_solved: 'Problema risolto',
      user_joined: 'Nuovo utente',
      new_version: 'Nuova versione',
      playback_started: 'Riproduzione avviata',
      playback_paused: 'Riproduzione in pausa',
      playback_resumed: 'Riproduzione ripresa',
      playback_stopped: 'Riproduzione interrotta',
      weekly_summary: 'Riepilogo settimanale',
      monthly_summary: 'Il mese in sintesi',
      yearly_summary: 'L’anno in sintesi',
      connection_stopped: 'Connessione interrotta',
      account_blocked: 'Account bloccato',
      new_device: 'Accesso da un nuovo dispositivo',
      share_played: 'Link condiviso riprodotto',
      programme_reminder: 'Programma in partenza',
    },
    eventHelp: {
      new_episode:
        'È uscito un episodio di una serie TV guardata o aggiunta ai preferiti: una volta per episodio, mai per quelli già usciti.',
      recording_finished:
        'Una registrazione della Diretta TV è terminata, completa o con una parte mancante.',
      recording_failed: 'Una registrazione della Diretta TV non ha registrato nulla.',
      health_problem:
        'Sistema › Stato di salute ha rilevato un problema, in due controlli consecutivi.',
      health_solved: 'Un problema rilevato da Sistema › Stato di salute è scomparso.',
      user_joined: 'Qualcuno ha creato il proprio account tramite un link di invito.',
      new_version:
        'È uscita una nuova versione di Polyfin, rilevata dal controllo giornaliero: una volta per versione, con le note di rilascio.',
      playback_started:
        'Un video o un brano inizia la riproduzione in un’applicazione Jellyfin, con chi lo riproduce, su quale dispositivo e se viene convertito.',
      playback_paused:
        'Una riproduzione viene messa in pausa. Le posizioni durante la riproduzione non vengono mai inviate.',
      playback_resumed: 'Una riproduzione in pausa riprende.',
      playback_stopped:
        'Una riproduzione si interrompe, oppure la sua applicazione ha smesso di segnalarla da cinque minuti.',
      weekly_summary:
        'Una volta alla settimana, nel giorno e all’ora impostati dal server: le ore guardate, i titoli e i nuovi episodi della settimana. Le destinazioni del server e degli amministratori ricevono la settimana del server, con gli utenti arrivati, i file aggiunti alle cartelle locali e i problemi rilevati; le altre ricevono la propria. Non viene inviato nulla per una settimana senza niente da segnalare.',
      monthly_summary:
        'Il 1° di ogni mese, all’ora del riepilogo settimanale: il contenuto del riepilogo settimanale sul mese appena concluso. Non viene inviato nulla per un mese senza niente da segnalare.',
      yearly_summary:
        'Il 1° gennaio, all’ora del riepilogo settimanale: le ore guardate, i dieci film e le dieci serie TV più guardati e il mese con più ore dell’anno appena concluso. Le destinazioni del server e degli amministratori ricevono l’anno del server, con gli utenti arrivati e i file aggiunti alle cartelle locali; le altre ricevono il proprio. Non viene inviato nulla per un anno senza niente da segnalare.',
      connection_stopped:
        'Una connessione a un’applicazione o un servizio di tracciamento ha rifiutato ciò che Polyfin conservava e si è interrotta: una volta per interruzione, con cosa fare per ricollegarla.',
      account_blocked:
        'Password errate hanno bloccato un account per un certo tempo: una volta per blocco, con la scadenza e l’indirizzo e l’applicazione dell’ultimo tentativo. Le destinazioni del server e degli amministratori vengono avvisate per ogni account, le altre solo per il proprio.',
      new_device:
        'Il tuo account ha effettuato l’accesso da un’applicazione Jellyfin o dal client web su un dispositivo mai usato prima, con l’applicazione, il dispositivo e l’indirizzo. Il primo dispositivo di un account non viene segnalato.',
      share_played:
        'Qualcuno senza account ha avviato un film o un episodio tramite un link di condivisione: al massimo una volta all’ora per link, con il titolo e chi l’ha condiviso. Le destinazioni del server vengono avvisate per ogni link, le altre solo per i propri.',
      programme_reminder:
        'Un programma della Diretta TV per cui hai impostato un promemoria nel lettore web inizia a breve, con l’anticipo indicato in Remind me before, con un link al suo canale.',
    },
    enabled: 'Invia messaggi a questa destinazione',
    create: 'Aggiungi',
    creating: 'Aggiunta…',
    save: 'Salva',
    saving: 'Salvataggio…',
    created: (name: string) => `${name} aggiunta.`,
    saved: (name: string) => `${name} salvata.`,
    test: 'Invia un test',
    testing: 'Invio…',
    testDelivered: (name: string) => `${name} ha ricevuto il messaggio di test.`,
    testFailed: (name: string) => `${name} non ha accettato il messaggio di test.`,
    edit: 'Modifica',
    delete: 'Elimina',
    deleteTitle: (name: string) => `Eliminare ${name}?`,
    deleteBody: 'I messaggi non le verranno più inviati e quelli in attesa verranno scartati.',
    deleted: (name: string) => `${name} eliminata.`,
    status: {
      working: 'Funzionante',
      waiting: 'Ancora nulla inviato',
      off: 'Disattivato',
      refused: 'Rifiutata',
      rejected: 'Messaggio rifiutato',
      unreachable: 'Non raggiungibile',
      unreadable: 'Da reinserire',
    },
    answer: (code: string, email: boolean) => (email ? `SMTP ${code}` : `HTTP ${code}`),
    problem: {
      refused: (answer: string) =>
        `La destinazione ha rifiutato Polyfin (${answer}): controlla l’indirizzo, il token o la chiave.`,
      refusedEmail: (answer: string) =>
        `Il server SMTP ha rifiutato Polyfin (${answer}): controlla utente e password, l’indirizzo del mittente e questo indirizzo.`,
      rejected: (answer: string) => `La destinazione ha rifiutato l’ultimo messaggio (${answer}).`,
      unreachable:
        'Non è stato possibile recapitare i messaggi recenti: la destinazione non ha risposto o ha risposto con errori. Ogni messaggio viene ritentato per un’ora.',
      unreadable:
        'L’indirizzo, il token o la chiave non possono essere decifrati con POLYFIN_SECRET_KEY: non le viene inviato nulla finché non vengono reinseriti.',
    },
    lastSent: 'Ultimo messaggio ',
    nothingSent: 'Nessun messaggio ancora inviato',
    noEvents: 'Nessun evento scelto',
  },
}

export default notifications
