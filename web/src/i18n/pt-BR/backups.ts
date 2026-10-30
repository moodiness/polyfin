import type en from '../en/backups'

const backups: typeof en = {
  backups: {
    destinations: {
      title: 'Destinos',
      loading: 'Carregando os destinos…',
      empty: 'Nenhum destino ainda.',
      emptyHint:
        'Adicione um armazenamento compatível com S3, um servidor SFTP ou um servidor WebDAV, para manter os backups longe dos discos deste servidor.',
      add: 'Adicionar destino',
      addTitle: 'Adicionar destino',
      editTitle: (name: string) => `Editar ${name}`,
      kind: 'Tipo',
      kinds: { s3: 'Armazenamento compatível com S3', sftp: 'SFTP', webdav: 'WebDAV' },
      kindHelp: {
        s3: 'Amazon S3, Backblaze B2, Cloudflare R2, MinIO, Wasabi e outros armazenamentos compatíveis com S3. Use um bucket exclusivo ou um prefixo em que só o Polyfin grave.',
        sftp: 'Um servidor em que o Polyfin entra via SSH, como um NAS. A chave do host é salva na primeira conexão e verificada depois.',
        webdav: 'Uma pasta de um servidor WebDAV, como Nextcloud, ownCloud ou um NAS.',
      },
      endpoint: 'Endpoint',
      endpointHelp:
        'O endereço do armazenamento, sem o bucket, como https://s3.eu-west-3.amazonaws.com.',
      bucket: 'Bucket',
      prefix: 'Prefixo da pasta',
      prefixHelp: 'A pasta dentro do bucket, como polyfin. Deixe vazio para a raiz do bucket.',
      region: 'Região',
      regionHelp:
        'Deixe vazio para us-east-1, que a maioria dos armazenamentos aceita. O Cloudflare R2 usa auto.',
      accessKey: 'Chave de acesso',
      secretKey: 'Chave secreta',
      host: 'Host',
      hostHelp: 'O nome ou endereço IP do servidor, sem sftp://.',
      port: 'Porta',
      user: 'Usuário',
      password: 'Senha',
      sftpPasswordHelp: 'Deixe vazio quando o login for feito com uma chave privada.',
      privateKey: 'Chave privada',
      privateKeyHelp:
        'Uma chave privada OpenSSH ou PEM sem frase secreta, cuja chave pública o servidor aceite. Deixe vazio para entrar com a senha.',
      privateKeySaved: 'Há uma chave salva: cole outra para substituí-la.',
      privateKeyRemove: 'Remover a chave salva',
      sftpFolder: 'Pasta',
      sftpFolderHelp:
        'Onde os backups ficam, como /volume1/backups/polyfin. Deixe vazio para a pasta pessoal do usuário.',
      webdavAddress: 'Endereço da pasta',
      webdavAddressHelp:
        'O endereço de uma pasta existente, como https://cloud.example.org/remote.php/dav/files/sam/polyfin/.',
      webdavPasswordHelp: 'Uma senha de aplicativo, quando o servidor oferecer.',
      kept: 'Backups a manter',
      keptHelp: (min: number, max: number, byDefault: number) =>
        `Após cada envio, o Polyfin exclui ali os backups mais antigos além desse número. Outros arquivos nunca são alterados. De ${min} a ${max}; ${byDefault} por padrão.`,
      hostKey: 'Chave do host',
      hostKeyNone: 'Salva na primeira conexão.',
      hostKeyForget: 'Esquecer',
      hostKeyForgetHelp:
        'Esqueça a chave salva quando a chave do servidor tiver mudado de propósito: a chave que ele apresentar na próxima conexão será salva.',
      hostKeyForgotten: 'A chave que o servidor apresentar na próxima conexão será salva.',
      create: 'Adicionar',
      creating: 'Adicionando…',
      save: 'Salvar',
      saving: 'Salvando…',
      created: (name: string) => `${name} adicionado.`,
      saved: (name: string) => `${name} salvo.`,
      test: 'Testar',
      testing: 'Testando…',
      testHelp: 'Grava um arquivo pequeno, lê de volta e o exclui.',
      testOk: (name: string) => `${name} funciona.`,
      testFailed: (name: string) => `${name} não funciona.`,
      edit: 'Editar',
      delete: 'Excluir',
      deleteTitle: (name: string) => `Excluir ${name}?`,
      deleteBody: 'Os backups não serão mais enviados para lá. Os que já estão lá permanecem.',
      deleted: (name: string) => `${name} excluído.`,
      keeps: (count: number) => (count === 1 ? 'mantém 1 backup' : `mantém ${count} backups`),
      status: {
        waiting: 'Nenhum envio ainda',
        working: 'Funcionando',
        failed: 'Falhou',
        stale: 'Desatualizado',
        unreadable: 'Chave ilegível',
      },
      lastUpload: 'Último envio',
      newest: 'Backup mais recente lá',
      none: 'Nenhum ainda',
      unreadable:
        'A senha ou chave não pode ser descriptografada com POLYFIN_SECRET_KEY: digite-a novamente.',
      staleHelp: 'O backup mais recente lá tem mais de dois dias.',
    },
    restore: {
      button: 'Restaurar',
      title: 'Restaurar um backup',
      intro:
        'O banco de dados volta a ser como era quando o backup foi feito. Escolha um backup da pasta ou de um destino, ou envie um arquivo de backup.',
      loading: 'Listando os backups…',
      folder: 'Pasta de backups',
      none: 'Nenhum backup aqui.',
      beforeRestore: 'antes de uma restauração',
      encrypted: 'criptografado',
      choose: 'Restaurar este backup',
      download: 'Baixar',
      downloadDecrypted: 'Baixar descriptografado',
      downloadTitle: (file: string) => `Baixar ${file} descriptografado`,
      downloadBody:
        'Digite a frase secreta com que este backup foi criptografado. O dump é salvo sem criptografia neste computador: guarde-o com o mesmo cuidado que o servidor.',
      passphrase: 'Frase secreta dos backups',
      downloading: 'Verificando a frase secreta…',
      upload: 'Enviar um arquivo de backup',
      uploadHelp:
        'Um arquivo .dump do banco de dados do Polyfin, feito pelo Polyfin ou com pg_dump --format=custom, ou um arquivo .dump.enc criptografado pelo Polyfin.',
      unlockTitle: (file: string) => `Abrir ${file}`,
      unlockBody:
        'Este backup está criptografado. Digite a frase secreta com que ele foi feito: o Polyfin o descriptografa, verifica cada parte dele e depois mostra o que a restauração faz.',
      unlock: 'Abrir',
      unlocking: 'Descriptografando e verificando…',
      checking: 'Verificando o backup…',
      unavailable: {
        pg_restore_missing:
          'O pg_restore não foi encontrado no servidor: instale as ferramentas de cliente do PostgreSQL 18, que a imagem Docker inclui.',
      },
      confirmTitle: (file: string) => `Restaurar ${file}?`,
      made: (date: string) => `Feito em ${date}.`,
      from: (name: string) => `De ${name}.`,
      uploaded: 'Enviado deste computador.',
      steps: [
        'Primeiro, o Polyfin faz backup do banco de dados atual na pasta de backups, como um backup pré-restauração que ele nunca exclui.',
        'Os aplicativos recebem um erro enquanto a restauração é executada, e as reproduções, conversões e gravações em andamento são interrompidas.',
        'Em seguida, o Polyfin reinicia com o banco de dados restaurado: tudo fica como era quando o backup foi feito, incluindo usuários, configurações e histórico de reprodução.',
        'Os aplicativos e administradores que entraram desde então precisam entrar novamente, inclusive você, e os links de stream fornecidos antes deixam de funcionar.',
      ],
      keyTitle: 'Algumas chaves armazenadas não podem ser lidas',
      keyBody:
        'Este backup contém chaves e senhas que a POLYFIN_SECRET_KEY atual não consegue descriptografar. Defina a chave que estava definida quando o backup foi feito e restaure novamente; ou restaure mesmo assim e informe estes itens novamente depois:',
      kinds: {
        settings: (n: number) =>
          n === 1 ? '1 chave das configurações' : `${n} chaves das configurações`,
        tracking: (n: number) =>
          n === 1 ? '1 conexão de acompanhamento' : `${n} conexões de acompanhamento`,
        apps: (n: number) => (n === 1 ? '1 conexão de aplicativo' : `${n} conexões de aplicativos`),
        notifications: (n: number) =>
          n === 1 ? '1 destino de notificação' : `${n} destinos de notificação`,
        shares: (n: number) =>
          n === 1 ? '1 senha de compartilhamento' : `${n} senhas de compartilhamento`,
        destinations: (n: number) => (n === 1 ? '1 destino de backup' : `${n} destinos de backup`),
        certificates: (n: number) =>
          n === 1
            ? '1 chave do certificado Let’s Encrypt'
            : `${n} chaves do certificado Let’s Encrypt`,
        twoStep: (n: number) =>
          n === 1 ? '1 chave de login em duas etapas' : `${n} chaves de login em duas etapas`,
      },
      serverName: (name: string) => `Digite o nome do servidor, ${name}, para confirmar`,
      confirm: 'Restaurar',
      back: 'Outros backups',
      anyway: 'Restaurar mesmo assim',
      starting: 'Fazendo backup do banco de dados atual…',
      restoringTitle: 'Restaurando',
      restoringBody:
        'O Polyfin está restaurando o backup e depois será reiniciado. Esta página será recarregada assim que ele responder.',
      lastRestore: 'Última restauração',
      lastRestoreOk: (file: string, by: string) => `${file}, por ${by}`,
      lastRestoreSetup: (file: string) => `${file}, pela página de configuração`,
      lastRestoreFailed: (file: string) =>
        `Não foi possível restaurar ${file}: o banco de dados continua como estava.`,
    },
  },
}

export default backups
