import type en from '../en/auth'

const auth: typeof en = {
  setup: {
    title: 'Bem-vindo ao Polyfin',
    description:
      'Ainda não existe nenhum administrador. Crie a primeira conta para gerenciar este servidor e entrar pelos aplicativos Jellyfin.',
    codeHelp: {
      container:
        'Por segurança, o Polyfin registrou um código de configuração de uso único no log ao iniciar. Execute docker logs <container> (no Unraid, abra o log do container) e procure a linha com o código de configuração.',
      computer:
        'Por segurança, o Polyfin registrou um código de configuração de uso único no log ao iniciar. No Polyfin Desktop, escolha Código de configuração no menu para copiá-lo; caso contrário, procure a linha com o código de configuração no log do Polyfin.',
    },
    setupCode: 'Código de configuração',
    setupCodeHint: 'Formato XXXX-XXXX. Maiúsculas, minúsculas e hífen não importam.',
    name: 'Nome do administrador',
    password: 'Senha',
    confirmPassword: 'Confirmar senha',
    submit: 'Criar administrador',
    submitting: 'Criando…',
  },
  login: {
    title: 'Entrar',
    description: 'Entre com sua conta Polyfin para continuar.',
    name: 'Nome',
    password: 'Senha',
    submit: 'Entrar',
    submitting: 'Entrando…',
    codeTitle: 'Login em duas etapas',
    codeDescription:
      'Digite o código de 6 dígitos que seu aplicativo autenticador mostra para o Polyfin.',
    code: 'Código',
    codeHelp: 'Perdeu o celular? Digite um dos seus códigos de recuperação.',
    verify: 'Verificar',
    verifying: 'Verificando…',
    restart: 'Entrar com outra conta',
    setupTitle: 'Configurar login em duas etapas',
    setupDescription:
      'Os administradores devem usar o login em duas etapas neste servidor. Adicione sua conta a um aplicativo autenticador no seu celular e digite o código que ele mostrar.',
    recoveryTitle: 'Guarde seus códigos de recuperação',
    continue: 'Continuar',
    providerSignIn: (name: string) => `Entrar com ${name}`,
    providerOutcomes: {
      provider_refused: 'O login com o provedor de identidade foi cancelado.',
      provider_no_account:
        'Nenhuma conta deste servidor corresponde à sua conta no provedor de identidade.',
      account_disabled: 'Esta conta está desativada.',
      outside_allowed_hours: 'Esta conta não pode entrar neste horário.',
      too_many_attempts: 'Tentativas demais. Tente novamente mais tarde.',
      provider_failed: 'O login com o provedor de identidade falhou. Tente novamente.',
    },
  },
  invite: {
    title: (server: string) => `Entrar em ${server}`,
    description:
      'Escolha o nome e a senha da sua conta. Você também vai usá-los nos aplicativos Jellyfin.',
    name: 'Seu nome',
    password: 'Senha',
    confirmPassword: 'Confirmar senha',
    submit: 'Criar minha conta',
    submitting: 'Criando…',
    gone: {
      invite_unknown: {
        title: 'Este link de convite não funciona',
        description:
          'Verifique se o link foi copiado por inteiro ou peça um novo à pessoa que o enviou.',
      },
      invite_used_up: {
        title: 'Este link de convite se esgotou',
        description: 'Ele já criou todas as contas que podia. Peça um novo à pessoa que o enviou.',
      },
      invite_expired: {
        title: 'Este link de convite expirou',
        description: 'Peça um novo à pessoa que o enviou.',
      },
      invite_revoked: {
        title: 'Este link de convite foi revogado',
        description: 'Ele não funciona mais. Peça um novo à pessoa que o enviou.',
      },
    },
  },
}

export default auth
