import type en from '../en/common'

const common: typeof en = {
  documentTitle: 'Administração do Polyfin',
  header: {
    productName: 'Polyfin',
  },
  language: {
    label: 'Idioma de exibição',
    en: { short: 'EN', name: 'English' },
    fr: { short: 'FR', name: 'Français' },
    de: { short: 'DE', name: 'Deutsch' },
    es: { short: 'ES', name: 'Español' },
    it: { short: 'IT', name: 'Italiano' },
    'pt-BR': { short: 'PT', name: 'Português (Brasil)' },
  },
  common: {
    enterNumber: 'Digite um número.',
    loading: 'Carregando…',
    save: 'Salvar',
    saving: 'Salvando…',
    cancel: 'Cancelar',
    retry: 'Tentar novamente',
    never: 'Nunca',
    colon: ':',
    passwordRule: 'Pelo menos 8 caracteres.',
    nameRule: "De 1 a 64 caracteres: letras, dígitos, espaços e - _ ' . @ +",
    passwordMismatch: 'As duas senhas não coincidem.',
    moveUp: (name: string) => `Mover ${name} para cima`,
    moveDown: (name: string) => `Mover ${name} para baixo`,
    moved: (name: string, position: number, count: number) =>
      `${name} movido para a posição ${position} de ${count}.`,
  },
  errors: {
    generic: 'Algo deu errado. Tente novamente.',
    network: 'Não foi possível acessar o servidor. Verifique se o Polyfin está em execução.',
    unauthenticated: 'Sua sessão expirou. Entre novamente.',
    forbidden: 'Você não tem permissão para fazer isso.',
    cross_origin: 'A solicitação foi rejeitada porque não veio desta página.',
    invalid_setup_code:
      'Este código de configuração não é válido. Copie-o novamente do registro do servidor.',
    invalid_name: "Os nomes devem ter de 1 a 64 caracteres: letras, dígitos, espaços e - _ ' . @ +",
    invalid_password: 'As senhas devem ter pelo menos 8 caracteres.',
    setup_complete: 'A configuração já foi concluída. Entre em vez disso.',
    too_many_attempts: 'Tentativas demais. Aguarde alguns minutos antes de tentar novamente.',
    invalid_credentials: 'Nome ou senha incorretos.',
    account_disabled: 'Esta conta está desativada. Peça a um administrador para ativá-la.',
    wrong_password: 'Sua senha atual está incorreta.',
    invalid_code:
      'Este código não é válido. Digite o código de 6 dígitos que seu aplicativo mostra agora, ou um dos seus códigos de recuperação.',
    two_step_expired: 'O login demorou demais. Digite seu nome e sua senha novamente.',
    two_step_on: 'O login em duas etapas já está ativado.',
    two_step_off: 'O login em duas etapas está desativado para esta conta.',
    two_step_not_started: 'Comece de novo: ative o login em duas etapas para obter uma nova chave.',
    two_step_required:
      'Os administradores devem usar o login em duas etapas neste servidor: ele não pode ser desativado.',
    two_step_own: 'Desative seu próprio login em duas etapas em Minha conta.',
    not_found: 'Este item não existe mais. A lista foi atualizada.',
    not_set:
      'Nada está salvo: foi removido, ou não pode ser descriptografado com POLYFIN_SECRET_KEY. Recarregue a página.',
    not_revealable: 'Esta conexão não tem nenhuma chave para mostrar.',
    unknown_code:
      'Nenhum dispositivo está aguardando com este código. Ele pode ter expirado ou sido digitado incorretamente.',
    quick_connect_disabled: 'O Quick Connect está desativado neste servidor.',
    name_taken: 'Este nome já está sendo usado por outro usuário.',
    invalid_invite_uses:
      'As contas que um link de convite pode criar devem ser um número inteiro de 1 a 100.',
    invalid_invite_expiry: 'Escolha na lista quando o link de convite expira.',
    invalid_invite_model: 'Este usuário não existe mais. Escolha outro.',
    invite_unknown: 'Este link de convite não funciona.',
    invite_used_up: 'Este link de convite já foi totalmente utilizado.',
    invite_expired: 'Este link de convite expirou.',
    invite_revoked: 'Este link de convite foi revogado.',
    share_links_off:
      'Você não pode criar links de compartilhamento: um administrador os desativou para este servidor ou para sua conta.',
    not_shareable: 'Somente um filme ou um episódio que você pode ver pode ser compartilhado.',
    invalid_share_expiry: 'Escolha na lista quando o link expira.',
    invalid_share_viewers: 'Os espectadores simultâneos devem ser um número inteiro de 1 a 5.',
    last_administrator:
      'Este é o último administrador ativado: ele não pode ser excluído, rebaixado nem desativado.',
    admin_cannot_join_group:
      'Administradores não podem entrar em um grupo, pois suas permissões são diferentes. Recarregue a página para ver quem é administrador agora.',
    invalid_group: 'Este grupo não existe mais. Recarregue a página e escolha outro.',
    follows_group:
      'Este usuário segue seu grupo: altere o grupo, ou desative “Seguir o grupo” primeiro.',
    invalid_copy_from: 'Este usuário não existe mais. Escolha outro.',
    invalid_server_name: 'O nome do servidor deve ter de 1 a 64 caracteres.',
    invalid_language: 'Escolha um idioma da lista.',
    invalid_catalog_limit:
      'Os títulos lidos por catálogo devem ser um número inteiro de 100 a 20.000.',
    invalid_channel_limit:
      'Os canais lidos por catálogo de TV ao Vivo devem ser um número inteiro de 100 a 50.000.',
    invalid_played_percent: 'O limite de reproduzido deve ser um número inteiro de 50 a 100.',
    invalid_resume_percent: 'O limite para retomar deve ser um número inteiro de 0 a 50.',
    resume_not_below_played: 'O limite para retomar deve ser menor que o limite de reproduzido.',
    invalid_version_list_minutes:
      'O tempo de retenção das listas de versões deve ser um número inteiro de minutos de 1 a 360.',
    invalid_catalog_refresh_minutes:
      'O tempo de atualização do catálogo deve ser um número inteiro de minutos de 1 a 1.440.',
    invalid_analysis_timeout:
      'O tempo para analisar uma versão deve ser um número inteiro de segundos de 5 a 120.',
    invalid_version_attempts: 'As versões tentadas devem ser um número inteiro de 1 a 10.',
    invalid_version_order:
      'Escolha no máximo quatro critérios para ordenar as versões, um de cada tipo. Recarregue a página.',
    invalid_version_codec_order:
      'A ordem dos codecs de vídeo deve listar cada codec uma vez. Recarregue a página.',
    invalid_version_source_order:
      'A ordem das fontes deve listar fontes do servidor, cada uma uma vez. Recarregue a página.',
    invalid_hide_versions_larger:
      'O tamanho acima do qual as versões são ocultadas deve ser 0, ou um número inteiro de GB de 1 a 1.000.',
    invalid_hidden_version_codecs:
      'Esta lista de codecs ocultos não é suportada. Recarregue a página.',
    invalid_hidden_version_words:
      'Digite no máximo 50 palavras que ocultam versões, uma por linha, cada uma com no máximo 100 caracteres.',
    invalid_hidden_releases:
      'Esta lista de tipos de lançamentos ocultos não é suportada. Recarregue a página.',
    invalid_max_conversions:
      'As conversões de vídeo simultâneas devem ser um número inteiro de 0 a 32.',
    invalid_max_conversion_height: 'Escolha na lista a qualidade máxima do vídeo convertido.',
    invalid_subtitle_ocr: 'Escolha na lista quando as legendas em imagem são convertidas em texto.',
    invalid_subtitle_ocr_tracks:
      'As faixas de legendas em imagem lidas simultaneamente devem ser um número inteiro de 1 a 4.',
    invalid_default_language: 'Escolha um idioma padrão da lista.',
    invalid_encoder_preset: 'Escolha uma velocidade de codificação da lista.',
    invalid_video_quality: 'A qualidade de vídeo deve ser 0, ou um número inteiro de 1 a 51.',
    invalid_hardware_acceleration: 'Escolha uma placa de vídeo da lista.',
    invalid_hardware_decoding_codecs:
      'Esta lista de formatos lidos pela placa de vídeo não é suportada. Recarregue a página.',
    invalid_adaptive_quality: 'Escolha na lista quando oferecer várias qualidades.',
    invalid_tone_mapping_algorithm: 'Escolha um método de mapeamento de tons da lista.',
    invalid_tone_mapping_peak:
      'O brilho máximo deve ser 0, ou um número inteiro de nits de 100 a 10.000.',
    invalid_tone_mapping_desat: 'A dessaturação dos realces deve ser um número de 0 a 10.',
    invalid_processor_tone_mapping_height:
      'Escolha na lista a qualidade máxima do HDR convertido pelo processador.',
    invalid_deinterlace_method: 'Escolha na lista um método de desentrelaçamento.',
    invalid_downmix_algorithm: 'Escolha na lista uma mixagem estéreo.',
    invalid_downmix_boost: 'O volume ao mixar para estéreo deve ser um número de 0,5 a 3.',
    invalid_max_audio_channels: 'Escolha na lista o número máximo de canais de áudio.',
    invalid_audio_bitrate_per_channel:
      'A taxa de bits de áudio por canal deve ser 0 ou um número inteiro de kb/s de 32 a 320.',
    invalid_encoding_threads: 'As threads do processador devem ser um número inteiro de 0 a 64.',
    invalid_ahead_seconds:
      'Os segundos preparados com antecedência devem ser um número inteiro de 30 a 600.',
    parental_control:
      'O controle parental se aplica a esta conta: ela mantém os complementos do servidor, que fornecem as classificações etárias das quais ele depende.',
    invalid_parental_control:
      'Esta configuração de controle parental não é compatível. Recarregue a página.',
    invalid_max_playbacks: 'As reproduções simultâneas devem ser um número inteiro de 0 a 20.',
    invalid_max_bitrate: 'Escolha na lista a qualidade máxima.',
    invalid_sync_play: 'Escolha na lista uma opção de assistir juntos.',
    invalid_manifest_url:
      'Digite o endereço do manifesto de um complemento que comece com https://, http:// ou stremio:// e termine em /manifest.json.',
    addon_exists: 'Este complemento já está instalado aqui.',
    addon_unreachable:
      'Não foi possível acessar o complemento. Verifique o endereço e se o complemento está on-line e tente novamente.',
    invalid_manifest: 'Este endereço não retornou um manifesto de complemento do Stremio.',
    private_network:
      'Este complemento está em um endereço da rede local. Somente administradores podem instalar esse tipo de complemento.',
    invalid_order:
      'A lista de complementos mudou nesse meio-tempo. Ela foi atualizada: tente novamente.',
    invalid_library:
      'Uma biblioteca se refere a um catálogo que não existe mais ou não pode ser navegado. Remova as bibliotecas marcadas como não disponíveis e salve novamente.',
    invalid_library_name: 'Os nomes das bibliotecas devem ter de 1 a 64 caracteres.',
    invalid_library_genre:
      'O gênero de uma biblioteca não é oferecido pelo catálogo dela. Escolha outro na lista e salve novamente.',
    invalid_library_max_items:
      'O máximo de títulos de uma biblioteca deve ser um número inteiro de 1 a 20.000 ou ficar vazio.',
    invalid_smart_list_name: 'O nome de uma lista inteligente deve ter de 1 a 64 caracteres.',
    invalid_smart_list_shown_as: 'Escolha se a lista aparece como biblioteca ou como coleção.',
    invalid_smart_list_catalogs:
      'Escolha de 1 a 50 catálogos dos quais a lista vai buscar títulos.',
    invalid_smart_list_rules:
      'Uma regra está incompleta: dê um valor a cada uma e pelo menos um limite a cada ano ou avaliação.',
    invalid_smart_list_sort: 'Escolha como a lista é ordenada.',
    invalid_smart_list_max_items:
      'O máximo de títulos de uma lista inteligente deve ser um número inteiro de 1 a 20.000 ou ficar vazio.',
    too_many_smart_lists: 'Já existem 100 listas inteligentes aqui. Exclua uma para criar outra.',
    invalid_image: 'Escolha uma imagem JPEG, PNG ou WebP de no máximo 10 MB.',
    invalid_image_url: 'Digite um endereço de imagem que comece com https:// ou http://.',
    image_unreachable:
      'Não foi possível baixar nenhuma imagem deste endereço. Verifique se ele abre uma imagem e tente novamente.',
    image_private_network:
      'Esta imagem está em um endereço da rede local. Somente administradores podem usar esse tipo de endereço.',
    invalid_login_attempts:
      'As senhas incorretas antes de bloquear uma conta devem ser 0 ou um número inteiro de 3 a 20.',
    invalid_inactive_device_days:
      'Os dias antes de desconectar dispositivos sem uso devem ser um número inteiro de 0 a 365.',
    invalid_https_mode:
      'Servir HTTPS deve ser Desligado, Arquivos de certificado ou Let’s Encrypt.',
    invalid_https_port: 'A porta HTTPS deve ser um número inteiro de 1 a 65535.',
    invalid_https_certificate:
      'O certificado deve ser um caminho absoluto no contêiner, como /certs/fullchain.pem.',
    invalid_https_private_key:
      'A chave privada deve ser um caminho absoluto no contêiner, como /certs/privkey.pem.',
    https_certificate_missing: 'Não há nenhum arquivo de certificado neste caminho do contêiner.',
    https_private_key_missing: 'Não há nenhum arquivo de chave privada neste caminho do contêiner.',
    https_certificate_unreadable:
      'O Polyfin não consegue ler este arquivo de certificado, ou ele não contém nenhum certificado PEM. Verifique se o usuário 65532 tem permissão de leitura.',
    https_private_key_unreadable:
      'O Polyfin não consegue ler este arquivo de chave privada, ou ele não contém nenhuma chave RSA ou ECDSA em PEM. Verifique se o usuário 65532 tem permissão de leitura.',
    https_files_mismatch: 'A chave privada não corresponde ao certificado.',
    invalid_acme_hosts:
      'Os nomes de host devem ser nomes públicos como media.example.org, um por linha, sem curingas nem endereços IP, no máximo 100, e pelo menos um para o Let’s Encrypt.',
    invalid_acme_email: 'O e-mail deve ser um endereço de e-mail ou ficar vazio.',
    acme_terms_required: 'Aceite os termos de serviço do Let’s Encrypt para usá-lo.',
    invalid_acme_challenge: 'O desafio deve ser HTTP-01 ou TLS-ALPN-01.',
    invalid_known_proxies:
      'Os proxies conhecidos devem ser endereços IP, redes como 172.18.0.0/16 ou nomes de host como traefik, um por linha, no máximo 64.',
    personal_addons_disabled:
      'Seus próprios complementos foram desativados por um administrador. Eles foram mantidos, mas você não pode adicionar, atualizar nem ativar nenhum.',
    invalid_hidden_libraries: 'A lista de bibliotecas mudou nesse meio-tempo. Recarregue a página.',
    invalid_blocked_genres:
      'Os gêneros devem ter de 1 a 100 caracteres, e no máximo 100 podem ser bloqueados.',
    invalid_access_schedules:
      'Os horários vão de 00:00 a 24:00, e cada fim deve vir depois do seu início (no máximo 50 linhas).',
    outside_allowed_hours:
      'Esta conta não pode ser usada neste horário. Tente novamente durante os horários permitidos.',
    invalid_guide_url:
      'Digite um endereço de guia que comece com https:// ou http://, de no máximo 4.096 caracteres.',
    invalid_app: 'O nome do aplicativo deve ter de 1 a 64 caracteres, sem caracteres especiais.',
    invalid_trickplay_interval:
      'O tempo entre duas miniaturas deve ser um número inteiro de segundos de 5 a 60.',
    invalid_trickplay_width: 'Escolha na lista a largura das miniaturas.',
    invalid_thumbnail_storage_gb:
      'O espaço para imagens deve ser um número inteiro de GB de 1 a 50.',
    invalid_recording_padding:
      'Os minutos antes e depois de uma gravação devem ser números inteiros de 0 a 60.',
    invalid_recording_retention_days:
      'Os dias para manter as gravações devem ser um número inteiro de 0 a 3.650.',
    invalid_convert_recordings_hours:
      'Escolha nas listas dois horários diferentes entre os quais converter as gravações.',
    invalid_quality_group: 'Escolha o grupo de qualidade na lista.',
    invalid_live_tv_refresh_hours:
      'O intervalo de atualização deve ser um número inteiro de horas de 1 a 168.',
    invalid_local_scan_hours:
      'O intervalo de rastreamento deve ser um número inteiro de horas de 0 a 168.',
    invalid_custom_css: 'O CSS personalizado deve ter no máximo 2 MB.',
    invalid_custom_js: 'O JavaScript personalizado deve ter no máximo 2 MB.',
    invalid_web_theme: 'Escolha LumaaGlaass, Nenhum ou Personalizado.',
    theme_copy_running: 'Uma cópia do tema já está sendo feita. Tente de novo em um instante.',
    theme_copy_nothing: 'Os textos personalizados salvos não carregam nada de outro lugar.',
    theme_copy_failed: 'Nenhum arquivo pôde ser salvo, então nenhuma cópia foi feita.',
    custom_texts_changed:
      'Os textos personalizados mudaram enquanto a cópia era feita. Tente de novo.',
    invalid_login_disclaimer: 'A mensagem de login deve ter no máximo 8 KB.',
    invalid_publicmetadb_key:
      'O PublicMetaDB não aceitou esta chave. Copie-a novamente da sua conta PublicMetaDB. Nada foi salvo.',
    publicmetadb_unreachable:
      'Não foi possível acessar o PublicMetaDB para verificar a chave. Nada foi salvo: tente novamente mais tarde.',
    invalid_theintrodb_key:
      'O TheIntroDB não aceitou esta chave. Copie-a novamente da sua conta TheIntroDB. Nada foi salvo.',
    theintrodb_unreachable:
      'Não foi possível acessar o TheIntroDB para verificar a chave, ou ele está limitando as solicitações no momento. Nada foi salvo: tente novamente mais tarde.',
    invalid_mdblist_ratings_key:
      'O MDBList não aceitou esta chave. Copie-a novamente da sua conta MDBList. Nada foi salvo.',
    mdblist_unreachable:
      'Não foi possível acessar o MDBList para verificar a chave. Nada foi salvo: tente novamente mais tarde.',
    invalid_tmdb_key:
      'O TMDB não aceitou esta chave. Copie novamente a chave da API ou o token de acesso de leitura da sua conta TMDB. Nada foi salvo.',
    tmdb_unreachable:
      'Não foi possível acessar o TMDB para verificar a chave. Nada foi salvo: tente novamente mais tarde.',
    invalid_critic_rating_shows: 'Escolha na lista qual nota a avaliação da crítica exibe.',
    invalid_measure_loudness: 'Escolha na lista quais músicas têm o volume medido.',
    invalid_segment_order:
      'A ordem das fontes dos marcadores de pulo deve listar cada fonte uma única vez. Recarregue a página.',
    invalid_trakt_app:
      'O client ID e o secret do Trakt devem ter no máximo 256 caracteres, sem espaços nem caracteres especiais. Copie-os novamente do seu aplicativo Trakt.',
    invalid_simkl_app:
      'O client ID do Simkl deve ter no máximo 256 caracteres, sem espaços nem caracteres especiais. Copie-o novamente do seu aplicativo Simkl.',
    invalid_backup_hour: 'Escolha o horário dos backups na lista.',
    invalid_backups_kept: 'O número de backups mantidos deve ser um número inteiro de 1 a 90.',
    invalid_torrent_route:
      'As streams de torrent precisam de uma única rota: um nome de interface de rede (somente Linux), um proxy SOCKS5 no formato host:porta ou uma configuração do WireGuard.',
    invalid_wireguard_config: 'A configuração do WireGuard não é válida.',
    invalid_torrent_limit: 'Os limites devem ser números inteiros de Mbps de 0 a 10000.',
    invalid_cache_size: 'O espaço em disco deve ser um número inteiro de GB de 1 a 2000.',
    invalid_vaapi_device: 'Escolha a placa de vídeo na lista.',
    invalid_recordings_folder:
      'A pasta de gravações deve ser um caminho absoluto para uma pasta na qual o Polyfin possa gravar.',
    invalid_backup_folder:
      'A pasta de backups deve ser um caminho absoluto para uma pasta na qual o Polyfin possa gravar.',
    invalid_conversion_folder:
      'A pasta de conversão deve ser um caminho absoluto para uma pasta na qual o Polyfin possa gravar.',
    invalid_reading_folder:
      'A pasta de arquivos em leitura deve ser um caminho absoluto para uma pasta na qual o Polyfin possa gravar.',
    invalid_cache_folder:
      'A pasta de cache deve ser um caminho absoluto para uma pasta na qual o Polyfin possa gravar.',
    too_many_destinations: 'Já existem 10 destinos de backup: exclua um primeiro.',
    too_many_home_rows: 'Já existem 20 linhas na tela inicial: exclua uma primeiro.',
    invalid_home_row:
      'Dê à linha um nome com no máximo 100 caracteres e escolha o catálogo ou a coleção dela.',
    invalid_destination_kind: 'Escolha o tipo de destino na lista.',
    invalid_destination_address:
      'Digite o endereço conforme solicitado: um endereço http:// ou https:// sem caminho para armazenamento S3, um nome de host ou endereço IP para um servidor SFTP, o endereço de uma pasta para um servidor WebDAV.',
    invalid_bucket:
      'Digite o nome do bucket: de 3 a 63 letras minúsculas, dígitos, pontos, hifens ou sublinhados.',
    invalid_destination_folder:
      'A pasta não pode subir níveis com “..”, nem conter caracteres de controle, e deve ter no máximo 512 bytes.',
    invalid_region: 'A região é composta de letras minúsculas, dígitos e hifens, como eu-west-3.',
    invalid_port: 'A porta é um número inteiro de 1 a 65535.',
    invalid_destination_user: 'Digite o usuário ou, para armazenamento S3, a chave de acesso.',
    invalid_destination_secret:
      'Digite a senha, a chave secreta para armazenamento S3 ou, para um servidor SFTP, uma chave privada.',
    invalid_private_key:
      'A chave privada deve ser uma chave privada OpenSSH ou PEM sem frase secreta.',
    backups_off: 'Os backups estão desativados: ative-os acima primeiro.',
    pg_restore_missing:
      'O pg_restore não foi encontrado no servidor: instale as ferramentas cliente do PostgreSQL 18, que a imagem Docker inclui.',
    backup_not_found: 'Este backup não existe mais: escolha outro.',
    not_a_backup: 'Este arquivo não é um backup do banco de dados do Polyfin.',
    newer_backup:
      'Este backup foi feito por uma versão mais recente do Polyfin: atualize o Polyfin primeiro.',
    changed_backup:
      'Este backup foi feito por um Polyfin cujas migrações de banco de dados diferem das deste.',
    restore_expired: 'O backup escolhido não está mais pronto: escolha-o novamente.',
    unreadable_secrets:
      'Este backup contém chaves que a POLYFIN_SECRET_KEY não consegue descriptografar: confirme com Restaurar mesmo assim.',
    restoring: 'Uma restauração já está em andamento.',
    wrong_server_name: 'Digite o nome do servidor exatamente como exibido.',
    too_large: 'O arquivo é grande demais.',
    destination_failed: 'Não foi possível ler o destino.',
    restore_failed: 'Não foi possível iniciar a restauração: nada foi alterado.',
    invalid_collection_read_hour:
      'Escolha na lista o horário em que as coleções são lidas, ou Nunca.',
    invalid_collection_library_apps:
      'Escolha na lista os aplicativos que exibem as bibliotecas de coleções por coleção.',
    invalid_remuxdb_url:
      'O endereço do RemuxDB deve ser um endereço web que comece com http:// ou https://.',
    invalid_update_tool_url:
      'O endereço da ferramenta de atualização deve ser um endereço web que comece com http:// ou https://, sem nome de usuário nem fragmento.',
    invalid_update_tool_token:
      'O token da ferramenta de atualização não tem espaços e tem no máximo 1024 caracteres.',
    restart_unconfirmed: 'O Polyfin não sabe se algo o inicia de novo: confirme o aviso antes.',
    no_update: 'O Polyfin Desktop não oferece essa atualização agora.',
    not_desktop: 'O Polyfin não roda no Polyfin Desktop.',
    no_update_tool: 'Nenhuma ferramenta de atualização está definida nas configurações gerais.',
    update_tool_timeout:
      'A ferramenta de atualização não respondeu em 50 segundos: talvez ainda esteja atualizando.',
    update_tool_unreachable: 'Não foi possível alcançar a ferramenta de atualização.',
    invalid_public_address:
      'O endereço público deve ser um endereço web que comece com http:// ou https://, sem parâmetros de consulta.',
    invalid_local_address:
      'O endereço na rede local deve ser um endereço web que comece com http:// ou https://, como http://192.168.1.10:8096, sem parâmetros de consulta.',
    invalid_dlna_name: 'O nome na rede pode ter no máximo 256 bytes.',
    invalid_dlna_user: 'Esse usuário não existe mais. Escolha outro, ou “Ninguém”.',
    invalid_dlna_profile: 'Este não é um perfil DLNA no formato XML do plugin DLNA do Jellyfin.',
    invalid_smtp_host:
      'Digite o nome do host do servidor SMTP, como smtp.example.org, sem espaços.',
    invalid_smtp_port: 'A porta SMTP deve ser um número inteiro de 1 a 65535.',
    invalid_smtp_security: 'Escolha na lista como a conexão SMTP é criptografada.',
    invalid_smtp_account: 'O usuário e a senha SMTP têm até 256 caracteres, em uma linha.',
    invalid_smtp_sender:
      'Digite apenas o endereço do remetente, como polyfin@example.org, e um nome de até 128 caracteres.',
    invalid_weekly_summary_day: 'Escolha na lista o dia do resumo semanal.',
    invalid_weekly_summary_hour: 'Escolha na lista a hora do resumo semanal.',
    invalid_import_every_minutes:
      'Escolha na lista com que frequência as contas conectadas são importadas.',
    invalid_target_kind: 'Escolha um tipo de destino na lista.',
    invalid_target_name: 'Os nomes de destino devem ter de 1 a 64 caracteres.',
    invalid_target_address: 'Digite um endereço web que comece com https:// ou http://.',
    private_target_address:
      'Este endereço está em uma rede local: apenas os destinos de um administrador podem acessá-lo.',
    invalid_email_address: 'Digite um endereço de e-mail, como sam@example.org.',
    email_unavailable:
      'O e-mail precisa de um servidor SMTP: um administrador o configura em Configurações › Notificações.',
    invalid_topic: 'Um tópico do ntfy tem de 1 a 64 letras, dígitos, hifens e sublinhados.',
    invalid_chat:
      'Digite o número do chat, como -1001234567890, ou o nome de um canal público, como @news_example.',
    invalid_token:
      'Digite o token: até 256 caracteres, sem espaços. O de um bot do Telegram se parece com 123456:ABC-DEF.',
    invalid_user_key: 'Uma chave de usuário do Pushover tem apenas letras e dígitos.',
    invalid_events: 'Um dos eventos escolhidos não está disponível para este destino.',
    invalid_reminder_settings: 'Escolha 1, 5, 10, 15, 30 ou 60 minutos.',
    too_many_targets: 'Já existem 20 destinos. Exclua um para adicionar outro.',
    target_unreadable:
      'O endereço ou token deste destino não pode ser descriptografado com POLYFIN_SECRET_KEY. Digite-o novamente.',
    invalid_source_name: 'Os nomes de fonte devem ter de 1 a 64 caracteres.',
    invalid_source_address:
      'Digite um endereço que comece com https:// ou http://, e, para uma conta Xtream Codes, um nome de usuário e uma senha.',
    tuner_added: 'Este sintonizador já é uma fonte do servidor.',
    invalid_folder_name: 'Os nomes de pasta devem ter de 1 a 64 caracteres.',
    invalid_folder_path:
      'Digite o caminho completo da pasta onde o Polyfin é executado, exceto a raiz do sistema: no contêiner, começando com /; no Windows, com a unidade, como D:\\Filmes, ou o compartilhamento, como \\\\nas\\media\\Filmes.',
    invalid_share_address:
      'Digite um endereço como smb://servidor/compartilhamento/pasta, https://servidor/pasta, s3://bucket/prefixo, sftp://servidor/pasta ou rclone://servidor:porta/pasta, sem usuário ou senha: eles têm campos próprios.',
    invalid_share_user: 'Os usuários têm no máximo 256 caracteres, as senhas no máximo 1.024.',
    invalid_share_endpoint:
      'Digite o endpoint como um endereço http:// ou https://, como https://s3.example.org, e uma região de no máximo 64 caracteres sem espaços.',
    invalid_share_key:
      'Cole uma chave privada OpenSSH ou PEM completa e, se houver, a frase secreta que a abre.',
    invalid_rclone_remote:
      'Digite o nome de um dos remotes do rclone, sem dois-pontos nem barra, com no máximo 128 caracteres.',
    invalid_poll_minutes:
      'Digite um número de minutos de 0 a 1.440, ou 0 para procurar alterações apenas nos rastreamentos.',
    invalid_folder_kind: 'Escolha se a pasta contém filmes ou séries.',
    invalid_imdb_id: 'Um identificador do IMDb é tt seguido de dígitos, como tt0063350.',
    invalid_episode:
      'Escreva o episódio como no nome de um arquivo, como S01E05, ou S01E05-E06 para dois.',
    title_search_failed:
      'Falha ao pesquisar nos complementos de metadados. Tente novamente em instantes.',
    invalid_channel_list:
      'Este endereço não retornou uma lista de canais. Verifique-o e se a conta ainda é válida.',
    channel_list_too_large:
      'Esta lista de canais é grande demais (no máximo 100 MB ou 100.000 canais).',
    iptv_login_refused: 'O servidor IPTV recusou este nome de usuário ou senha.',
    invalid_message: 'Escreva uma mensagem de 1 a 500 caracteres.',
    not_controllable:
      'Este aplicativo não aceita controle remoto: não é possível pará-lo nem enviar uma mensagem a ele daqui.',
    remote_control_refused:
      'Sua conta não pode controlar os aplicativos de outros usuários. Outro administrador pode conceder essa permissão na página Usuários.',
    too_soon:
      'Este complemento foi verificado há menos de um minuto. Tente novamente em instantes.',
    invalid_limit: 'Esta lista não pode mostrar tantas entradas.',
    addon_kind_changed:
      'Este endereço agora leva a outro tipo de complemento (de música em vez de vídeo, ou o contrário). Instale-o como um novo complemento.',
    invalid_addon_settings:
      'O complemento recusou estas configurações: um valor não é uma das opções, é longo demais ou está fora do intervalo. Verifique os campos e tente novamente.',
    invalid_options:
      'Estas opções de importação não são aceitas. Recarregue a página e tente novamente.',
    invalid_category_name: 'Os nomes de categoria devem ter de 1 a 64 caracteres.',
    category_not_custom: 'Apenas suas próprias categorias podem ser excluídas.',
    invalid_channel_name: 'Os nomes de canal devem ter de 1 a 100 caracteres.',
    invalid_logo:
      'Digite um endereço de logotipo que comece com https:// ou http://, com no máximo 4.096 caracteres.',
    invalid_description: 'As descrições devem ter no máximo 2.000 caracteres.',
    invalid_category: 'Esta categoria não existe mais. Recarregue a página.',
    invalid_number: 'Digite um número inteiro de 1 a 99.999, ou deixe o número vazio.',
    invalid_move: 'Este canal só pode ser movido dentro da sua categoria. Recarregue a página.',
    invalid_bulk: 'Escolha canais, ou uma palavra-chave de pelo menos 2 caracteres.',
    invalid_streams: 'As streams mudaram nesse meio-tempo. Recarregue o canal.',
    invalid_stream_url:
      'Digite um endereço de stream que comece com https:// ou http://, com no máximo 4.096 caracteres.',
    stream_not_custom: 'Apenas as streams que você adicionou podem ser removidas.',
    too_many_guides: 'Um catálogo aceita no máximo 10 guias.',
    invalid_guide: 'Este guia não existe mais. Recarregue a página.',
    invalid_mode: 'Este modo de mapeamento não é compatível. Recarregue a página.',
    invalid_mapping: 'Este canal do guia não está mais nos guias do catálogo. Pesquise novamente.',
    invalid_jellyfin_address:
      'Digite o endereço do servidor Jellyfin, como http://192.168.1.10:8096.',
    jellyfin_key_refused:
      'O Jellyfin recusou esta chave da API. Crie uma no painel dele, em Chaves da API, e cole-a novamente.',
    jellyfin_key_limited:
      'Esta chave ou conta pode não ter permissão para listar os usuários do servidor. Use uma chave da API do painel dele ou a conta de um administrador.',
    jellyfin_key_owner_only:
      'Esta chave lê apenas os dados de exibição do próprio dono. Digite as senhas dos outros usuários no servidor ou desmarque “Importar dados de exibição” para eles.',
    jellyfin_sign_in_refused: 'O Jellyfin recusou este nome ou senha.',
    jellyfin_sign_in_forbidden:
      'O Jellyfin não permite que esta conta entre: ela pode estar desativada ou fora do horário permitido.',
    jellyfin_password_refused:
      'O Jellyfin recusou esta senha. Deixe em branco se a conta não tiver senha.',
    jellyfin_other_user:
      'Esta senha abriu a conta de outro usuário no servidor. Conecte-se com uma chave da API do painel dele.',
    jellyfin_unreachable:
      'Nada respondeu neste endereço. Verifique o endereço e se o Jellyfin está em execução.',
    not_jellyfin:
      'Um servidor respondeu neste endereço, mas não é o Jellyfin. Verifique o endereço.',
    jellyfin_import_running:
      'Uma importação já está em andamento. Aguarde a conclusão ou interrompa-a.',
    unknown_jellyfin_user:
      'Um usuário do Jellyfin não está mais nesse servidor. Conecte-se novamente para ler os usuários.',
    emby_user_key:
      'Esta chave pertence a um usuário, e o Emby não informa a quem. Crie uma chave da API nas configurações do Emby ou conecte-se com uma conta de usuário.',
    server_import_running:
      'Outra importação está em andamento neste servidor ou para esta conta. Tente novamente em alguns minutos.',
    server_imports_disabled:
      'A importação do seu histórico de exibição de outro servidor ou aplicativo está desativada neste servidor.',
    letterboxd_export_invalid:
      'Este arquivo não é uma exportação do Letterboxd: escolha o arquivo ZIP fornecido pelo Letterboxd, sem alterações. Ele contém watched.csv ou diary.csv.',
    letterboxd_export_too_large:
      'Este arquivo tem mais de 20 MB ou lista mais de 50.000 filmes: escolha o arquivo ZIP fornecido pelo Letterboxd.',
    invalid_nuvio_address:
      'Digite o endereço do backend do Nuvio, como https://api.nuvio.tv, ou deixe em branco para usar o do próprio Nuvio.',
    nuvio_sign_in_refused: 'O Nuvio recusou este e-mail ou senha.',
    nuvio_unreachable:
      'O backend do Nuvio não respondeu. Verifique o endereço e tente novamente mais tarde.',
    not_nuvio:
      'Algo respondeu neste endereço, mas não é um backend do Nuvio. Verifique o endereço ou deixe em branco para usar o do próprio Nuvio.',
    nuvio_rate_limited:
      'O Nuvio pede para aguardar antes de entrar novamente. Tente novamente em alguns minutos.',
    nuvio_backup_unsupported:
      'O Nuvio enviou os dados da conta em um formato que esta versão do Polyfin não lê. O Polyfin lê a versão 1 do backup de conta do Nuvio.',
    stremio_sign_in_refused: 'O Stremio recusou este e-mail ou senha.',
    stremio_pin_refused: 'O Stremio recusou este PIN.',
    stremio_key_refused:
      'O Stremio recusou esta chave de autenticação. Copie-a novamente do aplicativo web do Stremio, com sua conta conectada.',
    stremio_link_expired: 'O código de vinculação expirou ou já foi usado. Solicite outro.',
    stremio_unreachable: 'O Stremio não respondeu. Tente novamente mais tarde.',
    stremio_unexpected:
      'O Stremio respondeu de uma forma que esta versão do Polyfin não lê. Tente novamente mais tarde.',
    app_import_expired:
      'Os dados lidos da conta não são mais mantidos após alguns minutos. Entre novamente.',
    app_import_running: 'Uma importação para esta conta está em andamento. Aguarde a conclusão.',
    app_not_connected: 'Este aplicativo não está mais conectado à conta.',
    app_connection_stopped:
      'O aplicativo recusou o login que o Polyfin mantinha. Entre novamente, com “Manter conectado” marcado.',
    invalid_ends_on: 'Escolha a data de encerramento da conta ou “Nunca”.',
    administrator_cannot_end:
      'A conta de um administrador nunca é encerrada. Torne-o membro primeiro para definir uma data de encerramento.',
    invalid_invite_account_end: 'Escolha na lista quando as contas serão encerradas.',
  },
  status: {
    title: 'Status do servidor',
    description: 'Visão rápida da integridade deste servidor Polyfin.',
    version: 'Versão',
    serverId: 'ID do servidor',
    database: 'Banco de dados',
    databaseReady: 'Pronto',
    databaseUnavailable: 'Indisponível',
    loading: 'Carregando status do servidor…',
    errorTitle: 'Servidor inacessível',
    errorBody:
      'A API de administração não respondeu. Verifique se o Polyfin está em execução e tente novamente.',
    staleWarning: 'A última atualização falhou. Os detalhes abaixo podem estar desatualizados.',
    retry: 'Tentar novamente',
    retrying: 'Tentando novamente…',
    autoRefresh: 'Atualiza automaticamente a cada 10 segundos.',
    updatedAt: (time: string) => `Última atualização às ${time}`,
  },
  devices: {
    empty: 'Nenhum dispositivo conectado.',
    lastActivity: 'Última atividade',
    address: 'Endereço',
    signOut: 'Sair',
    signOutConfirm: (device: string) =>
      `Desconectar “${device}”? O aplicativo precisará entrar novamente.`,
    signedOut: (device: string) => `“${device}” foi desconectado.`,
  },
  stremioTypes: {
    movie: 'Filmes',
    series: 'Séries',
    channel: 'Canais',
    tv: 'TV',
    track: 'Faixas',
    album: 'Álbuns',
    artist: 'Artistas',
    playlist: 'Listas de reprodução',
    other: 'Outros vídeos',
  },
  stremioResources: {
    catalog: 'Catálogos',
    meta: 'Detalhes',
    stream: 'Streams',
    subtitles: 'Legendas',
    addon_catalog: 'Catálogos de complementos',
    search: 'Pesquisa',
    isrc: 'Correspondência de faixas',
    resolve: 'Links de reprodução',
    settings: 'Configurações',
  },
  forbidden: {
    title: 'Acesso negado',
    description: 'Somente administradores podem abrir esta página.',
    home: 'Voltar à página inicial',
  },
  time: {
    justNow: 'agora mesmo',
  },
  ui: {
    on: 'Ligado',
    off: 'Desligado',
    show: 'Mostrar',
    hide: 'Ocultar',
    close: 'Fechar',
    dismiss: 'Dispensar',
    notifications: 'Notificações',
    unsaved: 'Alterações não salvas',
    allSaved: 'Todas as alterações foram salvas',
    discard: 'Descartar alterações',
    searchList: 'Pesquisar na lista',
    noMatch: 'Nenhum resultado.',
    secret: {
      saved: 'Salvo',
      notSet: 'Não definido',
      removing: 'Será removido ao salvar',
      pastePlaceholder: 'Cole aqui',
      replacePlaceholder: 'Cole o novo valor',
      replace: 'Substituir',
      cancelReplace: 'Manter o valor salvo',
      remove: 'Remover',
      keep: 'Manter',
      removeHint: 'Salve para remover ou escolha “Manter”.',
      show: 'Mostrar chave',
      hide: 'Ocultar chave',
      savedHidden: 'Chave salva, oculta',
    },
  },
}

export default common
