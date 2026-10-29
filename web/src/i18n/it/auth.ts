import type en from '../en/auth'

const auth: typeof en = {
  setup: {
    title: 'Ti diamo il benvenuto in Polyfin',
    description:
      'Non esiste ancora nessun amministratore. Crea il primo account per gestire questo server e accedere dalle applicazioni Jellyfin.',
    codeHelp: {
      container:
        'Per sicurezza, all’avvio Polyfin ha scritto nel suo log un codice di configurazione monouso. Esegui docker logs <container> (su Unraid, apri il log del container) e cerca la riga con il codice di configurazione.',
      computer:
        'Per sicurezza, all’avvio Polyfin ha scritto nel suo log un codice di configurazione monouso. Con Polyfin Desktop, scegli Codice di configurazione nel suo menu per copiarlo; altrimenti, cerca la riga con il codice di configurazione nel log di Polyfin.',
    },
    setupCode: 'Codice di configurazione',
    setupCodeHint: 'Formato XXXX-XXXX. Maiuscole e trattino non sono rilevanti.',
    name: 'Nome dell’amministratore',
    password: 'Password',
    confirmPassword: 'Conferma password',
    submit: 'Crea amministratore',
    submitting: 'Creazione…',
  },
  login: {
    title: 'Accedi',
    description: 'Accedi con il tuo account Polyfin per continuare.',
    name: 'Nome',
    password: 'Password',
    submit: 'Accedi',
    submitting: 'Accesso…',
    codeTitle: 'Accesso in due passaggi',
    codeDescription:
      'Inserisci il codice a 6 cifre che la tua applicazione di autenticazione mostra per Polyfin.',
    code: 'Codice',
    codeHelp: 'Hai perso il telefono? Inserisci invece uno dei tuoi codici di recupero.',
    verify: 'Verifica',
    verifying: 'Verifica…',
    restart: 'Accedi con un altro account',
    setupTitle: 'Configura l’accesso in due passaggi',
    setupDescription:
      'Su questo server gli amministratori devono usare l’accesso in due passaggi. Aggiungi il tuo account a un’applicazione di autenticazione sul telefono, poi inserisci il codice che mostra.',
    recoveryTitle: 'Conserva i tuoi codici di recupero',
    continue: 'Continua',
    providerSignIn: (name: string) => `Accedi con ${name}`,
    providerOutcomes: {
      provider_refused: 'L’accesso con il provider di identità è stato annullato.',
      provider_no_account:
        'Nessun account di questo server corrisponde al tuo account presso il provider di identità.',
      account_disabled: 'Questo account è disattivato.',
      outside_allowed_hours: 'Questo account non può accedere a quest’ora.',
      too_many_attempts: 'Troppi tentativi. Riprova più tardi.',
      provider_failed: 'L’accesso con il provider di identità non è riuscito. Riprova.',
    },
  },
  invite: {
    title: (server: string) => `Unisciti a ${server}`,
    description:
      'Scegli il nome e la password del tuo account. Li userai anche nelle applicazioni Jellyfin.',
    name: 'Il tuo nome',
    password: 'Password',
    confirmPassword: 'Conferma password',
    submit: 'Crea il mio account',
    submitting: 'Creazione…',
    gone: {
      invite_unknown: {
        title: 'Questo link di invito non funziona',
        description:
          'Verifica di aver copiato il link per intero, oppure chiedine uno nuovo a chi te lo ha inviato.',
      },
      invite_used_up: {
        title: 'Questo link di invito è esaurito',
        description:
          'Ha già creato tutti gli account previsti. Chiedine uno nuovo a chi te lo ha inviato.',
      },
      invite_expired: {
        title: 'Questo link di invito è scaduto',
        description: 'Chiedine uno nuovo a chi te lo ha inviato.',
      },
      invite_revoked: {
        title: 'Questo link di invito è stato revocato',
        description: 'Non funziona più. Chiedine uno nuovo a chi te lo ha inviato.',
      },
    },
  },
}

export default auth
