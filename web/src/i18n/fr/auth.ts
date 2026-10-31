import type en from '../en/auth'

const auth: typeof en = {
  setup: {
    title: 'Bienvenue dans Polyfin',
    description:
      'Aucun administrateur n’existe encore. Créez le premier compte pour gérer ce serveur et vous connecter depuis les applications Jellyfin.',
    codeHelp: {
      container:
        'Par sécurité, Polyfin a inscrit un code d’installation à usage unique dans son journal au démarrage. Lancez docker logs <conteneur> (sur Unraid, ouvrez le journal du conteneur) et repérez la ligne contenant le code d’installation.',
      computer:
        'Par sécurité, Polyfin a inscrit un code d’installation à usage unique dans son journal au démarrage. Avec Polyfin Desktop, choisissez Code d’installation dans son menu pour le copier ; sinon, repérez la ligne contenant le code d’installation dans le journal de Polyfin.',
    },
    setupCode: 'Code d’installation',
    setupCodeHint: 'Format XXXX-XXXX. Majuscules et tiret facultatifs.',
    name: 'Nom de l’administrateur',
    password: 'Mot de passe',
    confirmPassword: 'Confirmer le mot de passe',
    submit: 'Créer l’administrateur',
    submitting: 'Création…',
    restoreInstead: 'Restaurer plutôt une sauvegarde',
    createInstead: 'Créer plutôt un administrateur',
    restoreTitle: 'Restaurer une sauvegarde',
    restoreLede:
      'Ramenez ce Polyfin à partir d’une sauvegarde de sa base de données : ses administrateurs, ses utilisateurs, ses réglages et son historique de visionnage.',
    restoreFile: 'Fichier de sauvegarde',
    restoreFileHelp:
      'Un fichier .dump ou .dump.enc fait par Polyfin, de son dossier des sauvegardes ou d’une destination.',
    restoreChoose: 'Choisir un fichier',
    restoreSend: 'Envoyer et vérifier',
    restoreSending: 'Envoi et vérification…',
    restoreUnlockHelp:
      'Cette sauvegarde est chiffrée : saisissez la phrase de passe avec laquelle elle a été faite.',
    restoreReady: (file: string) =>
      `${file} est une sauvegarde de la base de données de Polyfin que ce Polyfin peut restaurer.`,
    restoreSteps: [
      'Polyfin s’arrête, restaure la sauvegarde dans sa base de données d’un seul coup, puis redémarre dessus. Si quelque chose échoue, la base de données reste telle qu’elle est maintenant.',
      'Connectez-vous ensuite avec un administrateur de la sauvegarde.',
    ],
    restoreStart: 'Restaurer',
    restoreRestoring:
      'Polyfin restaure la sauvegarde, puis redémarre. Cette page affiche la connexion dès qu’il répond.',
    restoreFailed:
      'La sauvegarde n’a pas pu être restaurée : Polyfin a redémarré sans elle. Son journal dit pourquoi, et contient un nouveau code d’installation.',
  },
  login: {
    title: 'Connexion',
    description: 'Connectez-vous avec votre compte Polyfin pour continuer.',
    name: 'Nom',
    password: 'Mot de passe',
    submit: 'Se connecter',
    submitting: 'Connexion…',
    codeTitle: 'Connexion en deux étapes',
    codeDescription:
      'Saisissez le code à 6 chiffres que votre application d’authentification affiche pour Polyfin.',
    code: 'Code',
    codeHelp: 'Téléphone perdu ? Saisissez plutôt l’un de vos codes de secours.',
    verify: 'Vérifier',
    verifying: 'Vérification…',
    restart: 'Se connecter avec un autre compte',
    setupTitle: 'Configurer la connexion en deux étapes',
    setupDescription:
      'Les administrateurs doivent utiliser la connexion en deux étapes sur ce serveur. Ajoutez votre compte à une application d’authentification sur votre téléphone, puis saisissez le code qu’elle affiche.',
    recoveryTitle: 'Gardez vos codes de secours',
    continue: 'Continuer',
    providerSignIn: (name: string) => `Se connecter avec ${name}`,
    providerOutcomes: {
      provider_refused: 'La connexion avec le fournisseur d’identité a été annulée.',
      provider_no_account:
        'Aucun compte de ce serveur ne correspond à votre compte chez le fournisseur d’identité.',
      account_disabled: 'Ce compte est désactivé.',
      outside_allowed_hours: 'Ce compte ne peut pas se connecter à cette heure.',
      outside_local_network: 'Ce compte ne peut se connecter que depuis le réseau local.',
      too_many_attempts: 'Trop de tentatives. Réessayez plus tard.',
      provider_failed: 'La connexion avec le fournisseur d’identité a échoué. Réessayez.',
    },
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
