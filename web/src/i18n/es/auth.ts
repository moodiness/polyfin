import type en from '../en/auth'

const auth: typeof en = {
  setup: {
    title: 'Bienvenido a Polyfin',
    description:
      'Todavía no existe ningún administrador. Cree la primera cuenta para gestionar este servidor e iniciar sesión desde las aplicaciones de Jellyfin.',
    codeHelp: {
      container:
        'Por seguridad, Polyfin escribió un código de configuración de un solo uso en su registro al iniciarse. Ejecute docker logs <contenedor> (en Unraid, abra el registro del contenedor) y busque la línea con el código de configuración.',
      computer:
        'Por seguridad, Polyfin escribió un código de configuración de un solo uso en su registro al iniciarse. Con Polyfin Desktop, elija «Código de configuración» en su menú para copiarlo; si no, busque la línea con el código de configuración en el registro de Polyfin.',
    },
    setupCode: 'Código de configuración',
    setupCodeHint: 'Formato XXXX-XXXX. No importan las mayúsculas ni el guion.',
    name: 'Nombre del administrador',
    password: 'Contraseña',
    confirmPassword: 'Confirmar contraseña',
    submit: 'Crear administrador',
    submitting: 'Creando…',
    restoreInstead: 'Restaurar una copia de seguridad en su lugar',
    createInstead: 'Crear un administrador en su lugar',
    restoreTitle: 'Restaurar una copia de seguridad',
    restoreLede:
      'Recupere este Polyfin desde una copia de seguridad de su base de datos: sus administradores, usuarios, ajustes e historial de reproducción.',
    restoreFile: 'Archivo de copia de seguridad',
    restoreFileHelp:
      'Un archivo .dump o .dump.enc hecho por Polyfin, de su carpeta de copias de seguridad o de un destino.',
    restoreChoose: 'Elegir un archivo',
    restoreSend: 'Enviar y comprobar',
    restoreSending: 'Enviando y comprobando…',
    restoreUnlockHelp:
      'Esta copia de seguridad está cifrada: escriba la frase de contraseña con la que se hizo.',
    restoreReady: (file: string) =>
      `${file} es una copia de seguridad de la base de datos de Polyfin que este Polyfin puede restaurar.`,
    restoreSteps: [
      'Polyfin se detiene, restaura la copia de seguridad en su base de datos de una sola vez y luego se inicia de nuevo con ella. Si algo falla, la base de datos se queda como está ahora.',
      'Después, inicie sesión con un administrador de la copia de seguridad.',
    ],
    restoreStart: 'Restaurar',
    restoreRestoring:
      'Polyfin está restaurando la copia de seguridad y luego se inicia de nuevo. Esta página muestra el inicio de sesión en cuanto responde.',
    restoreFailed:
      'No se pudo restaurar la copia de seguridad: Polyfin se ha iniciado de nuevo sin ella. Su registro indica el motivo y contiene un nuevo código de configuración.',
  },
  login: {
    title: 'Iniciar sesión',
    description: 'Inicie sesión con su cuenta de Polyfin para continuar.',
    name: 'Nombre',
    password: 'Contraseña',
    submit: 'Iniciar sesión',
    submitting: 'Iniciando sesión…',
    codeTitle: 'Inicio de sesión en dos pasos',
    codeDescription:
      'Introduzca el código de 6 dígitos que muestra su aplicación de autenticación para Polyfin.',
    code: 'Código',
    codeHelp: '¿Ha perdido el teléfono? Introduzca en su lugar uno de sus códigos de recuperación.',
    verify: 'Verificar',
    verifying: 'Verificando…',
    restart: 'Iniciar sesión con otra cuenta',
    setupTitle: 'Configurar el inicio de sesión en dos pasos',
    setupDescription:
      'Los administradores deben usar el inicio de sesión en dos pasos en este servidor. Añada su cuenta a una aplicación de autenticación en su teléfono y, a continuación, introduzca el código que muestre.',
    recoveryTitle: 'Guarde sus códigos de recuperación',
    continue: 'Continuar',
    providerSignIn: (name: string) => `Iniciar sesión con ${name}`,
    providerOutcomes: {
      provider_refused: 'Se canceló el inicio de sesión con el proveedor de identidad.',
      provider_no_account:
        'Ninguna cuenta de este servidor corresponde a su cuenta del proveedor de identidad.',
      account_disabled: 'Esta cuenta está desactivada.',
      outside_allowed_hours: 'Esta cuenta no puede iniciar sesión a esta hora.',
      too_many_attempts: 'Demasiados intentos. Vuelva a intentarlo más tarde.',
      provider_failed:
        'El inicio de sesión con el proveedor de identidad falló. Vuelva a intentarlo.',
    },
  },
  invite: {
    title: (server: string) => `Unirse a ${server}`,
    description:
      'Elija el nombre y la contraseña de su cuenta. También los usará en las aplicaciones de Jellyfin.',
    name: 'Su nombre',
    password: 'Contraseña',
    confirmPassword: 'Confirmar contraseña',
    submit: 'Crear mi cuenta',
    submitting: 'Creando…',
    gone: {
      invite_unknown: {
        title: 'Este enlace de invitación no funciona',
        description:
          'Compruebe que se ha copiado el enlace completo o pida uno nuevo a la persona que se lo envió.',
      },
      invite_used_up: {
        title: 'Este enlace de invitación está agotado',
        description:
          'Ya ha creado todas las cuentas que permitía. Pida uno nuevo a la persona que se lo envió.',
      },
      invite_expired: {
        title: 'Este enlace de invitación ha caducado',
        description: 'Pida uno nuevo a la persona que se lo envió.',
      },
      invite_revoked: {
        title: 'Este enlace de invitación se ha revocado',
        description: 'Ya no funciona. Pida uno nuevo a la persona que se lo envió.',
      },
    },
  },
}

export default auth
