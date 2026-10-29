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
