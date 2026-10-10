import type en from '../en/auth'

const auth: typeof en = {
  setup: {
    title: 'Bienvenue dans Polyfin',
    description:
      'Aucun administrateur n’existe encore. Créez le premier compte pour gérer ce serveur et vous connecter depuis les applications Jellyfin.',
    codeHelp:
      'Par sécurité, Polyfin a inscrit un code d’installation à usage unique dans son journal au démarrage. Lancez docker logs <conteneur> (sur Unraid, ouvrez le journal du conteneur) et repérez la ligne contenant le code d’installation.',
    setupCode: 'Code d’installation',
    setupCodeHint: 'Format XXXX-XXXX. Majuscules et tiret facultatifs.',
    name: 'Nom de l’administrateur',
    password: 'Mot de passe',
    confirmPassword: 'Confirmer le mot de passe',
    submit: 'Créer l’administrateur',
    submitting: 'Création…',
  },
  login: {
    title: 'Connexion',
    description: 'Connectez-vous avec votre compte Polyfin pour continuer.',
    name: 'Nom',
    password: 'Mot de passe',
    submit: 'Se connecter',
    submitting: 'Connexion…',
  },
  invite: {
    title: (server: string) => `Rejoindre ${server}`,
    description:
      'Choisissez le nom et le mot de passe de votre compte. Vous les utiliserez aussi dans les applications Jellyfin.',
    name: 'Votre nom',
    password: 'Mot de passe',
    confirmPassword: 'Confirmer le mot de passe',
    submit: 'Créer mon compte',
    submitting: 'Création…',
    gone: {
      invite_unknown: {
        title: 'Ce lien d’invitation ne fonctionne pas',
        description:
          'Vérifiez que le lien a été copié en entier, ou demandez-en un nouveau à la personne qui vous l’a envoyé.',
      },
      invite_used_up: {
        title: 'Ce lien d’invitation est épuisé',
        description:
          'Il a créé tous les comptes qu’il pouvait. Demandez-en un nouveau à la personne qui vous l’a envoyé.',
      },
      invite_expired: {
        title: 'Ce lien d’invitation a expiré',
        description: 'Demandez-en un nouveau à la personne qui vous l’a envoyé.',
      },
      invite_revoked: {
        title: 'Ce lien d’invitation a été révoqué',
        description:
          'Il ne fonctionne plus. Demandez-en un nouveau à la personne qui vous l’a envoyé.',
      },
    },
  },
}

export default auth
