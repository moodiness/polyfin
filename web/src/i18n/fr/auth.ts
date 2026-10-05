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
}

export default auth
