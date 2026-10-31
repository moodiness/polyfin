import type en from '../en/versions'

const versions: typeof en = {
  versions: {
    title: 'Versions',
    description:
      'Voyez ce que devient chaque flux que les addons listent pour un film ou un épisode, tel qu’un utilisateur se le voit proposer, et épinglez ou bloquez des versions.',
    asUser: 'En tant que',
    asUserHelp:
      'Les versions dépendent de l’utilisateur : ses propres addons, les torrents, son groupe de qualité et sa langue audio.',
    you: (name: string) => `${name} (vous)`,
    find: 'Trouver un titre',
    search: 'Film ou série',
    searchHelp:
      'Cherche dans les bibliothèques que voit cet utilisateur. Une série liste ses épisodes.',
    searchButton: 'Chercher',
    searching: 'Recherche…',
    noTitles: 'Aucun film ni série trouvé.',
    titlesFound: (term: string) => `Titres trouvés pour « ${term} »`,
    pickTitle: (name: string, details: string) => `${name}, ${details}`,
    episodesOf: (name: string) => `Épisodes de ${name}`,
    noEpisodes: 'Aucun épisode.',
    back: 'Retour',
    everywhere: 'Bloqués pour tous les titres',
    everywhereHelp:
      'Aucune version de ces fichiers n’est listée, pour aucun titre. Le blocage d’un seul titre figure sur sa page.',
    noBlocks: 'Aucun fichier n’est bloqué pour tous les titres.',
    backToSearch: 'Versions',
    askAgain: 'Redemander aux addons',
    askedAgain: 'Les addons sont redemandés.',
    askedLately: 'Les addons viennent d’être demandés : réessayez dans quelques secondes.',
    pending: (count: number) =>
      count === 1 ? '1 addon est encore demandé…' : `${count} addons sont encore demandés…`,
    order: 'Ordre des versions',
    orderNone: 'L’ordre des addons : Trier les versions par n’a aucun critère.',
    orderBy: (criteria: string) => `Trier les versions par : ${criteria}.`,
    group: (height: number) => `Groupe de qualité : ${height}p.`,
    keptAll: 'Les règles de masquage cacheraient toutes les versions : aucune n’est cachée.',
    closest:
      'Aucune version ne tient dans le groupe de qualité : les hauteurs les plus proches viennent d’abord.',
    pin: 'Version épinglée',
    noPin:
      'Aucune version n’est épinglée. Une version épinglée vient en premier pour tous les utilisateurs, sur les pages des titres et en lecture, tant qu’un addon la liste ; une version choisie dans le menu des versions est toujours lue.',
    pinnedBy: (name: string) => (name === '' ? 'Épinglée' : `Épinglée par ${name}`),
    pinMissing: 'Aucun addon ne liste ce fichier maintenant : l’ordre habituel s’applique.',
    unpin: 'Désépingler',
    pinAction: 'Épingler',
    pinned: 'Épinglée',
    pinDone: 'Version épinglée.',
    unpinDone: 'Version désépinglée.',
    sources: 'Flux par source',
    sourceStates: {
      known: 'A répondu',
      stale: 'Expirée, redemandée',
      asked: 'En cours de demande',
    },
    embedded: 'Dans la description du titre',
    noStreams: 'N’a listé aucun flux.',
    noSources: 'Aucun addon ne liste de flux pour ce titre.',
    steps: {
      offered: (place: number) => `Version ${place}`,
      nothing: 'Rien à lire',
      torrents: 'Torrents désactivés pour cet utilisateur',
      blocked: 'Bloqué',
      duplicate: 'Même fichier listé avant',
      mirror: 'Miroir',
      failed: 'En échec récemment',
      hidden: 'Caché par une règle',
      group: 'Au-dessus du groupe de qualité',
    },
    stepHelp: {
      nothing: 'Une page web, une vidéo d’un autre site ou un avis : rien que Polyfin lise.',
      torrents:
        'Lire les flux torrent est désactivé, ou l’utilisateur ne peut pas lire de flux torrent (Paramètres › Torrents).',
      duplicate: 'Le même nom de fichier et la même taille, listés avant : une seule version.',
      mirror:
        'Le même fichier d’un autre addon, gardé comme miroir caché : une lecture reprend dessus quand la source de la version échoue.',
      group:
        'Le groupe de qualité de l’utilisateur écarte les versions plus hautes tant qu’une version y tient.',
    },
    rules: {
      elsewhere: (marker: string) => `Nomme un autre titre (${marker})`,
      larger: (gigabytes: string) => `Masquer les versions de plus de ${gigabytes} Go`,
      codec: (codec: string) => `Masquer ces codecs vidéo : ${codec}`,
      release: (kind: string) => `Masquer les sorties de mauvaise qualité : ${kind}`,
      words: (word: string) => `Masquer les versions dont les libellés contiennent « ${word} »`,
      other: (rule: string, detail: string) => (detail === '' ? rule : `${rule} : ${detail}`),
      none: 'Caché par une règle modifiée depuis',
    },
    unstarted:
      'N’a pas démarré récemment : les lectures qui ne choisissent pas de version la passent',
    labelsAsRead: 'Libellés',
    facts: 'Ce que savent les règles',
    analyzed: 'Analysée',
    multi: 'MULTI',
    block: 'Bloquer',
    blockTitle: 'Bloquer ce fichier',
    blockScope: 'Le bloquer pour',
    scopeTitle: 'Ce titre',
    scopeEverywhere: 'Tous les titres',
    scopeHelp:
      'Tous les titres bloque ce nom de fichier et cette taille partout où un addon les liste. Aucune version d’un fichier bloqué n’est listée ni lue ; les règles de masquage ne cachent jamais toutes les versions, un blocage si.',
    reason: 'Raison',
    reasonHelp: (max: number) =>
      `Pourquoi, pour l’administrateur suivant. ${max} caractères au plus.`,
    blocked: 'Bloqué',
    blockedBy: (name: string, reason: string) =>
      [name === '' ? 'Bloqué' : `Bloqué par ${name}`, reason].filter(Boolean).join(' : '),
    blockedEverywhere: 'Pour tous les titres',
    unblock: 'Débloquer',
    blockDone: 'Fichier bloqué.',
    unblockDone: 'Fichier débloqué.',
    titleBlocks: 'Bloqués pour ce titre',
    noTitleBlocks: 'Aucun fichier n’est bloqué pour ce titre.',
    analyze: 'Analyser maintenant',
    analyzeDone: 'Version analysée.',
    analysisFailed: (why: string) => `Elle n’a pas pu être analysée : ${why}`,
    decisions: 'Dernières lectures',
    decisionsHelp:
      'Ce qu’ont essayé les dernières lectures de ce titre, pour tout utilisateur. Gardées jusqu’au redémarrage.',
    noDecisions: 'Aucune lecture depuis le démarrage du serveur.',
    decisionBy: (user: string, device: string, app: string) =>
      [user, device, app].filter(Boolean).join(' · '),
    picked: 'Choisie dans le menu des versions',
    noneChosen: 'Aucune version lue',
    deliveries: {
      directPlay: 'lecture directe',
      directStream: 'flux direct',
      converted: 'convertie',
    },
    outcomes: {
      plays: 'se lit',
      unreadable: 'n’a pas pu être lue',
      unstarted: 'le torrent n’a pas démarré',
      torrentCap: 'autant de torrents tournaient que permis',
      conversionCap: 'autant de conversions tournaient que permis',
      unplayable: 'demande une conversion que l’utilisateur n’a pas, ou dépasse ses limites',
      notAnalyzed: 'pas analysée avant, donc pas essayée',
      passed: 'encore en lecture quand la lecture a continué',
    },
    notFound: 'Ce titre est introuvable pour cet utilisateur.',
  },
}

export default versions
