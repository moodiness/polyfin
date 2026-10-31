// Polyfin (MIT License). Served at /web/polyfin.js and loaded by the
// index.html of jellyfin-web, which Polyfin ships unmodified. It follows the
// client's routes for three things, adds a style for a fourth, mends a link
// for a fifth, closes a duplicate error for a sixth, leaves libraries out of
// the menus for a seventh, gives members a link to the admin app for an
// eighth, shows a title's three scores for a ninth, adds entries to the
// item menus for a tenth, adds the server's home rows to the home page for
// an eleventh, offers to share a movie or an episode for a twelfth, adds
// reminders to a programme's page for a thirteenth, drops the answers
// jellyfin-web keeps when another user signs in for a fourteenth, lists
// the user's recent searches on the search page for a fifteenth,
// shows titles' ratings on their posters for a sixteenth, writes how a
// torrent starts under jellyfin-web's spinner for a seventeenth, offers
// the identity provider's sign-in on the sign-in page for an eighteenth,
// and gives the page each user's web player theme for a nineteenth.
//
// It is written in ES5 and runs before jellyfin-web's scripts and their
// polyfills, so that it works on every browser jellyfin-web 12.4 is built
// for, down to Chrome 27: it uses no syntax newer than ES5 (a test checks
// it), writes addresses by hand rather than with URL or URLSearchParams,
// and falls back where a later API (WeakMap, WeakSet, isConnected) is
// missing.
//
// First, it sends the pages of jellyfin-web that need a Jellyfin server's
// administration, which Polyfin does not have, to Polyfin's admin app. It
// watches the route only, never the page, so that a new jellyfin-web keeps
// working with it. jellyfin-web 12.4 routes after the "#", matched without
// regard to case as its router does, "#!/" being an older form:
//   #/dashboard and every #/dashboard/… page (users, libraries, playback,
//     Live TV, devices, activity, logs, API keys, scheduled tasks, plugins…);
//   #/metadata, the metadata manager;
//   #/configurationpage, plugin configuration pages;
//   #/wizard/…, the startup wizard: Polyfin is set up in its admin app.
// A dashboard page with a match in the admin app opens that page; any other
// opens the admin app's home. Every link of the script to the admin app,
// these included, carries ?from=web: the admin app, opened from the web
// client signed in as another user than its own session's, signs that
// session out and takes the web client's sign-in, so that a browser shows
// one user, as Jellyfin's dashboard, part of jellyfin-web, does.
//
// Second, on a title's page (#/details?id=…), it adds the versions of the
// addons that answer after the page opened. Item details list the versions
// Polyfin knows when they are asked, while it asks the other addons in the
// background, and asks each addon again 10 seconds after its first answer,
// then 30 seconds later while its answers grow; /Polyfin/Items/{id}/Versions
// tells how many addons it still asks (Pending) and how many versions
// details list now (Count). Polyfin also pushes it, as it changes, on the
// socket jellyfin-web keeps open, in a PolyfinVersions message whose Data
// names the page's identifier (ItemId): the script hears it through
// ApiClient's message event. It asks at once as the route changes, then
// every second while addons are pending, every 10 seconds once a push came,
// the socket then telling the changes,
// for at most 90 seconds: a first answer, which takes at most 15 seconds,
// and both follow-ups, 10 and 30 seconds later and as long each, end within
// 85. When the page lists fewer versions than Count, or a different number
// once no addon is pending, it asks for the title's details and lists their
// versions in the page's version menu, in place, keeping the version picked.
// It reloads the page's details instead, as jellyfin-web does when the page
// is shown again, when the version picked is gone or no longer the same
// version, since the track menus describe it. The first version carries the
// title's own identifier, and each source its version's own identifier as
// its ETag: when a version listed first takes the title's identifier, or
// Polyfin names the version picked otherwise, the version picked, by
// jellyfin-web or the user, is found by its ETag, else as the one source
// with its name, and its size when both are known, and picked under its
// new identifier before the reload, which jellyfin-web then keeps. It never
// changes the page while a video plays or while the version menu has focus,
// where it could be open: it tries again at the next answer, or as soon as
// the menu loses focus. When the answers say AddVersions false (Settings ›
// Web player › Add versions to an open title's page, off), it only brings
// a menu that lists the placeholder to the versions: the others show when
// the page opens again. Any error just stops it.
//
// Third, on the same page, it holds the play buttons (.btnPlay, .btnResume,
// .btnReplay) until a version is known, as /Polyfin/Items/{id}/Versions and
// its push tell (Known, the placeholder left out). While none is known and
// addons are asked (Pending), they are disabled, greyed, with a spinner and
// the tooltip "Looking for sources…"; as soon as one is known, they are given
// back as jellyfin-web made them. Until the first answer, they are held as
// searching when the version menu lists only the placeholder, a single
// source under the title's own identifier; the first answer may come before
// the title's details asked any addon, so it never tells that no source is
// available. Once no addon is left to ask, they stay disabled and a line
// under them says that no source is available, with a
// "Try again" button: it posts to /Polyfin/Items/{id}/Versions/Search, which
// has Polyfin ask the title's addons again, then follows the versions anew;
// asked too soon, Polyfin answers 429 with Retry-After, and the button waits
// that long. A MutationObserver on the page holds the buttons again when
// jellyfin-web renders it anew. Its words follow jellyfin-web's language, the
// document's lang (English, French, German, Spanish, Italian or Brazilian
// Portuguese, which any Portuguese gets; English otherwise). It never changes
// the page during a video, and gives everything back when the route leaves
// the title.
//
// Fourth, it keeps a long name from running under the arrow of the menus of
// a title's page: stream addons name versions with every detail, and
// jellyfin-web leaves no room for the arrow in those menus. Their text ends
// with "…" before it; the open menu still lists whole names.
//
// Fifth, the title of a collection library's "Recently Added" row on the
// home page opens the library on the screen its user chose (Settings › Home
// › Default screen), as the library's links in the header and the menu do.
// jellyfin-web, which calls a library without a CollectionType mixed, opens
// every library's Suggestions from that title (#/mixed?…&tab=1), and a
// collection library has no suggestions of its own. The script drops the
// tab from such a link as it is pressed or clicked, before jellyfin-web
// follows it: pressed covers a link opened in a new tab, clicked one
// followed with the keyboard.
//
// Sixth, it closes the generic error jellyfin-web shows on top of a refused
// playback's own error. When a title's PlaybackInfo answers with an
// ErrorCode, such as the RateLimitExceeded of a user playing on as many
// devices as allowed, jellyfin-web 12.4 explains the code, then rejects the
// playback without a reason, and its rejection handler shows "There was an
// error processing the request" over it: the user meets the wrong error
// first. Of two alerts with the same title shown within a second of each
// other, the first still open, the script hides the second at once, with
// its backdrop, and closes it once it is open, as its button would.
//
// Seventh, it leaves out of jellyfin-web's menus the libraries chosen to be
// hidden there on the Libraries page of Polyfin's admin app, by an
// administrator for the server's libraries or by the user for their own;
// each keeps its row on the home page, and other apps list it. Once a user
// is signed in, it asks /Polyfin/UserViews, which lists the user's views as
// /UserViews does, in its order, each with HideInMenus, and keeps one style
// that hides the links of each such library: the buttons of the top bar
// (jellyfin-web 12.4's UserViewNav, MUI Buttons in the AppBar whose address
// holds the view's identifier), its More menu (#user-view-overflow-menu),
// the side menu of that layout (an MUI Drawer, used below 900 pixels wide
// instead of the buttons) and the legacy layout's side menu
// (.libraryMenuOptions, by data-itemid). Other links to the library, such as
// its home row's title, the tabs of its page or the search button on that
// page, are left alone. The top bar shows the first 3 views, 5 from 1200
// pixels wide and 8 from 1536 (MUI's lg and xl breakpoints), or all of them
// when only one more would go under More, its button listing the rest; the
// views come after config.json's menuLinks, none in the one Polyfin ships.
// Where every view More would list is hidden, its button is hidden too. It
// asks again whenever the user signed in changes, which a route change goes
// with; jellyfin-web may restore the user after the first route, so after
// each route change it looks for the user every second for 30 seconds.
// jellyfin-web draws its menus before the answer comes, so the script keeps
// the last answer in the browser's storage, with the user it was for, and
// applies it as soon as it starts: it runs from the page's head, before
// jellyfin-web's deferred scripts. A user's first load in a browser may
// still show those libraries until the answer comes; every later load hides
// them at once, then asks again and keeps the new answer. The rules kept
// stay while the user signed in is the one they were for; another user's
// are dropped as soon as that user is known, and signing out drops the
// rules but keeps the answer. Storage that fails, or holds anything else,
// counts as none. Any error just stops it.
//
// Eighth, it gives a user signed in who is not an administrator, a member,
// a "Polyfin" entry where jellyfin-web 12.4 shows administrators Dashboard:
// it opens the admin app at /admin/ in the same tab, where the admin app
// signs the member in with the web client's token, as it does an
// administrator. It is named "Polyfin" in every language and drawn as
// Dashboard is, without the Administration heading over it:
//   the default layout's user menu (#app-user-menu), an MUI Menu, as a copy
//     of its Settings item after a divider of its own, before the divider
//     over Quick Connect, Select server and Sign out;
//   the legacy layouts' side menu (.mainDrawer), first in its user section
//     (.userMenuOptions): after the libraries without a heading, where
//     Dashboard is, it would read as one of them;
//   the settings page (#/mypreferencesmenu), which the user menu's Settings
//     and the legacy layouts' user button open, in a section of its own
//     before the user section (.userSection).
// The legacy layouts are Desktop (Legacy), Mobile (Legacy) and TV. The
// default layout's side menu has no Dashboard. It is a plain link:
// jellyfin-web's emby-linkbutton would route it within jellyfin-web. The
// script asks jellyfin-web's client (ApiClient.getCurrentUser) whether the
// user is an administrator whenever the user signed in changes, as for the
// seventh, and keeps the last member in the browser's storage, so that the
// entry shows as soon as jellyfin-web draws a menu. A MutationObserver on
// the page puts it back as jellyfin-web draws its menus anew. It never shows
// where Dashboard shows, nor before sign-in, and goes as soon as the user
// signed in is another, signing out included. Any error just stops it.
//
// Ninth, on a title's page, it shows the title's three scores in the first
// line of its details (.itemMiscInfo-primary): its IMDb rating, its
// critics' score and its audience score, labelled, as
// /Polyfin/Items/{id}/Ratings answers them while the server has an MDBList
// key for ratings (Settings › Content). jellyfin-web shows only
// CommunityRating, with a star, and CriticRating, with its own icon: the
// script hides both in that line while its scores show, and gives them back
// as the route leaves the title. The answer may come while MDBList is
// asked about the title (Pending): it asks again 3 seconds later, 10 times
// at most. A MutationObserver on the page puts the scores back as
// jellyfin-web renders the line anew. An answer without a score, or no
// answer, changes nothing; nor does anything during a video. Its words
// follow jellyfin-web's language, as for the third. Any error just stops
// it.
//
// Tenth, it adds Polyfin's own entries to jellyfin-web's item menu, the
// action sheet a card's ⋮ button or a right click on a card opens, and the
// ⋮ button of a title's page (.btnMoreCommands):
//   "Remove from Continue Watching", for a card of a Continue Watching row:
//     a row whose cards all have a resume point, as /UserItems/Resume
//     lists them, the libraries' own rows included. It deletes
//     /Polyfin/UserItems/{id}/Resume and takes the card off the row;
//   "Hide this title", for a movie, a home video or a series, or a season
//     or an episode of one, that the user has not hidden, as
//     /Polyfin/UserItems/Hidden lists them. It posts to
//     /Polyfin/UserItems/{id}/Hidden, takes the cards of the title off the
//     page and says so in a toast, as jellyfin-web's own;
//   "Show this title again", in its place for a title the user hid, its
//     series hidden counting for a season or an episode, which deletes
//     /Polyfin/UserItems/{id}/Hidden.
// As the button is pressed or the card right-clicked, before jellyfin-web
// opens its menu, the script asks for the item (ApiClient.getItem), the
// hidden titles and, for a card with a resume point, Continue Watching;
// the first action sheet that opens within 10 seconds, no other click
// coming between, gets the entries once the answers come. An entry closes
// the menu as Back does, then acts. jellyfin-web 12.4 keeps the answers of
// its rows for a minute, in memory and in the browser's IndexedDB, and
// draws a row from such an answer, asking nothing, when the change's
// UserDataChanged message refreshes it or when the page loads again within
// that minute. The script keeps what its entries took off in the
// browser's session storage, for the server and user, for 2 minutes, and
// has every row's answers (their fetchData) leave out, meanwhile, the item
// taken off Continue Watching from rows whose items all resume, unless
// played since its last play then, and the titles hidden, with what is in
// them, from every row. Nothing is kept longer: an older answer is asked
// again, and Polyfin's leaves them out. Its words follow jellyfin-web's
// language, as the third's. Any error just stops it.
//
// Eleventh, it adds to jellyfin-web's home page the rows the administrator
// chose under Settings › Web player › Home rows, as
// /Polyfin/HomeRows answers them for the user signed in: each with its
// Name, its Place and its first Items, the rows the user turned off, or
// without a title, left out. Each time the home page shows (#/home, its
// .homeSectionsContainer in a page that is not hidden), it asks again,
// with the user's home sections, as jellyfin-web reads them from the
// display preferences "usersettings" of the client "emby" (homesection0 to
// homesection9, jellyfin-web's defaults for those not set, the TV layout
// starting with the library tiles when they lack); then it puts each row
// at its place, in the answer's order: Top before the first section,
// AfterResume after the section of Continue Watching (resume), AfterNextUp
// after Next Up's (nextup), Bottom after the last; a row after a section
// the user's home page does not show takes the place before. jellyfin-web
// fills the sections .section0 to .section9 it made first, in place: the
// rows go between them. A row is drawn as jellyfin-web draws a library's
// row of latest items, with its classes, so that its styles, scroll
// buttons and item actions apply: the row's name opens its list
// (#/list?parentId= the row's Id), each card a poster with the title's
// played mark, or count of unplayed episodes, and progress, opening the
// title's page. When jellyfin-web draws the home page anew, which empties
// it, a MutationObserver on the page puts the rows back from the last
// answer. Every time it draws them, it leaves out the titles the item
// menus hid meanwhile, as the tenth keeps them in the session storage for
// the user signed in (a series taking its items with it), and a row left
// without a title. Nothing else changes, nor anything while the answer is awaited.
// Any error just stops it.
//
// Twelfth, on a movie's or an episode's page, it adds a "Share" button to
// the page's buttons (.mainDetailButtons), before its ⋮ button
// (.btnMoreCommands), drawn as jellyfin-web draws them, for a user who may
// give the title through a share link. As the route opens a title, it asks
// /Polyfin/Items/{id}/Share, which answers Shareable true when the server
// allows share links (Settings › Security › Allow share links), the user
// may share titles, and the title is a movie or an episode they see, with
// Url, the admin app's My account › Shared links with the title chosen.
// The button opens that address in the same tab, where the admin app signs
// the user in with the web client's token, as for the eighth. It asks every
// second until jellyfin-web made its client, 10 times at most. A
// MutationObserver on the page puts the button back as jellyfin-web renders
// the page anew; nothing changes during a video, and the button goes as the
// route leaves the title. Its word follows jellyfin-web's language, as for
// the third. Any error just stops it.
//
// Thirteenth, on the page of a Live TV programme that has not started, it
// adds Polyfin's reminders after the record buttons (.recordingFields,
// which jellyfin-web draws on every title page and fills for programmes):
// "Remind me", which has Polyfin tell the user, on their own notification
// targets, a few minutes before the programme starts, and a menu button
// whose menu offers "Remind me of every airing": the programmes of the
// same title on the same channel at the same time of day, within ten
// minutes, as a series timer finds them. A programme with a reminder shows
// "Reminder set" instead, which removes it, or the series reminder that
// made it: a line under the button then tells every airing at about which
// time. When no target of the user chose Programme starting, and their
// apps do not show reminders, a line says so, with a link to My account ›
// Notifications in the admin app. Jellyfin has no reminder: the script asks
// Polyfin's /Polyfin/Reminders/Programs/{id}, which answers how the
// programme stands, and an empty 204 for an item that is not a programme
// of the user's guide or for a user without Live TV, who get no button; it
// posts to /Polyfin/Reminders to set one, with the page's language, which
// the message is written in, and deletes /Polyfin/Reminders/{id} to remove
// one. It asks once jellyfin-web has drawn the page's recording fields,
// and a MutationObserver puts the buttons back as jellyfin-web renders the
// page anew. Its words follow jellyfin-web's language, as the third's. It
// never changes the page during a video, and takes the buttons off as the
// route leaves the programme. Any error just stops it.
//
// Fourteenth, it drops what jellyfin-web keeps of a user's answers once
// another user signs in. jellyfin-web 12.4 keeps its answers in a React
// Query cache for a minute before asking again, and for 24 hours in all,
// in memory and in the browser's IndexedDB (database keyval-store, store
// keyval, key jellyfin-query-cache), which a later load restores. Some of
// them are keyed without the user: the search page's Suggestions
// (SearchSuggestions) and results (Search), a library's Suggestions tab
// (SuggestionSectionWithItems: its Continue Watching, Latest and Next Up),
// MovieRecommendations, filters and studios. Its own Sign out clears the
// cache; a session ended otherwise, such as one an administrator signs out
// or a browser that lost its sign-in, does not, and the next user to sign
// in would see the last one's answers, titles their parental control
// forbids included, until jellyfin-web asks again. The script keeps in
// the browser's storage the user, as server and user, the kept answers are
// for. As it starts, before jellyfin-web, when the user its saved
// credentials sign in (jellyfin_credentials, the server used last) is
// another, it deletes the copy in IndexedDB, so that jellyfin-web restores
// nothing. Whenever the route changes, as it does when a user signs in on
// jellyfin-web's sign-in page, it looks at the user jellyfin-web's client
// (ApiClient) signed in, and when that is another, it deletes that copy and
// empties the cache in memory, through jellyfin-web's own LANGUAGE_CHANGE
// listeners on the document (jellyfin-web clears the cache as the language
// changes). Signing out changes nothing, nor does the same user signing in
// again. When jellyfin-web lacks any of these, the rest still applies, and
// nothing else changes. Any error just stops it.
//
// Fifteenth, on jellyfin-web's search page (#/search, its #searchPage), it
// lists the user's recent searches while the field is empty: up to 10,
// newest first, between the field (.searchFields) and the Suggestions
// (.searchSuggestions), drawn as those are, under a "Recent searches"
// heading. Each is a link that searches it again, with a button that
// removes it, and "Clear all" removes them all. jellyfin-web 12.4 keeps the
// field in the route (#/search?query=…, replaced as the user types, by
// useSearchParam), and shows what the field's search found half a second
// after the last key (useDebounceValue): its results (.searchResults), a
// line that nothing was found (.noItemsMessage), or the Suggestions for an
// empty field. As jellyfin-web draws results with a card, the script notes
// the route's search as shown, for the user signed in; a line that nothing
// was found drops it. It keeps the search shown as the user leaves it:
// opening a result or any other page, clearing the field, typing a search
// that neither starts with it nor is its start, or closing the tab. A
// search typed on, or back, thus replaces the one before, and a search
// that found nothing is never kept. A search kept again moves to the top,
// the same words in other capitals counting as one. A link opens the
// search page on its search, in the page's library if any (parentId,
// collectionType), as jellyfin-web's own "Retry with a global search" link
// opens #/search?query=…: jellyfin-web fills the field from the route and
// searches. The searches stay in the browser, nothing goes to the server:
// in its storage, under polyfinRecentSearches/ then the server and the
// user (server/user, as the fourteenth names the user), so that another
// user signing in never sees them. Signing out keeps them for the user's
// next sign-in in this browser, as jellyfin-web keeps the settings it
// stores in the browser for a user (appSettings, keyed by the user's ID)
// through its own Sign out. Storage that fails, or holds anything else,
// counts as none. Its words follow jellyfin-web's language, as the
// third's. Any error just stops it.
//
// Sixteenth, while the administrator turned it on (Settings › Web player ›
// Show ratings on posters), it shows a movie's or a series' ratings over
// its poster, at the top on the start side, as the pointer comes to its
// card or the focus does, the TV layout's included: its CommunityRating
// with a star, then its critics' and audience scores, in percent, each
// with an icon of its own, without words, those it lacks left out. Cards
// are jellyfin-web 12.4's (.card[data-id] with data-type Movie or
// Series, the poster in .cardScalable); others, and lists, get nothing.
// The first card met that is not known asks /Polyfin/Items/Ratings once
// for it and the other title cards of the page not known, those of its
// row first, 100 at most, which answers Enabled and the ratings of those
// with some; another card coming meanwhile waits for that answer. Every
// card answered gets its ratings at once, hidden by the style until the
// pointer or the focus comes, so that a row shows them without asking
// again; a card jellyfin-web draws anew gets them back as the pointer or
// the focus comes. Answers are kept for a minute, for the user signed in,
// and forgotten when another signs in. Turned off, nothing shows, and the
// server is asked again a minute later at the earliest, as is a server
// that did not answer; a server turning it off takes the ratings shown
// away. Nothing changes during a video. Any error just stops it.
//
// Seventeenth, while jellyfin-web's loading spinner shows (its .docspinner,
// .mdlSpinnerActive while it turns), it writes under it how the torrent the
// play waits for starts: "Finding peers: 12 found, 3 connected", then
// "Receiving the start: 2.4 MB from 3 peers", or why it did not start. It
// asks /Polyfin/Playback/Starting as the spinner shows, then every second
// after each answer, and stops as the spinner hides, the line going with
// it. Polyfin answers for the play of this browser's own session, the
// torrents its PlaybackInfo tries, then the one it chose while the player
// reads its first segments, and an empty Stage for anything else, which
// shows no line: a play of a link, a page loading. jellyfin-web's spinner
// takes no text (loading.ts), and the Jellyfin API tells a play nothing
// before it starts. Its words follow jellyfin-web's language, as the
// third's. Any error just stops it.
//
// Eighteenth, on jellyfin-web's sign-in page (#/login, its #loginPage), it
// offers to sign in with the identity provider the administrator set up
// (Settings › Security › Identity provider), as /admin/api/status names it:
// a "Sign in with …" button first among the page's buttons
// (.readOnlyContent), drawn as jellyfin-web's own, which opens the
// provider's sign-in at the server's public address (the provider's start,
// with ?app=web). The provider sends the browser back to /web/ with
// ?polyfinProvider=signed-in, and a ticket in a cookie the script cannot
// read. The script takes the parameter out of the address at once, waits
// for jellyfin-web to draw its first page, by which time it has saved its
// own connection to the server, then trades the ticket at
// /admin/api/provider/web, as jellyfin-web's device (ApiClient's app,
// device name, device ID and version, else those jellyfin-web keeps in
// the browser), for a Jellyfin sign-in of that device. It keeps the
// sign-in where jellyfin-web reads it (jellyfin_credentials), in this
// server's entry, as the admin app's sign-in to the web client does, and
// opens /web/ again, signed in. Any other value of the parameter, and a
// trade refused, is what went wrong, said under the button. It looks for
// the page as the route changes, for 5 seconds. Its words follow
// jellyfin-web's language, as the third's. Without fetch or storage it
// does nothing, and any error just stops it.
//
// Nineteenth, it gives the page the web player theme of the user, which
// an administrator, or the user under My account, chooses in the admin
// app: the server's, LumaaGlaass, jellyfin-web's own look (none) or the
// Custom theme. The page describes the themes in its head, as JSON
// (#polyfinWebThemes): the server's theme, the stylesheet and the script
// of each theme that has some, and the cookie that tells the server the
// theme shown. As it starts, before jellyfin-web, the script picks the
// theme: the one it kept for the user jellyfin-web's saved credentials
// sign in (as the fourteenth finds them), else, signed out, the one of the
// user last signed in in this browser, else the server's. It keeps, in the
// browser's storage (polyfinWebTheme), each user's own theme, empty for the
// server's, up to 20 users, the last first. A browser that cannot run
// LumaaGlaass, without CSS's :has() or the syntax of ES2021, which the
// theme's stylesheet and script need, gets jellyfin-web's own look instead.
// It writes the theme's stylesheet in the head while the page is parsed,
// so that the page shows the theme from its first paint, unless the user
// turned off the server's CSS in jellyfin-web's display settings
// (disableCustomCss, kept in the browser for the user); and the theme's
// script, deferred, which runs after jellyfin-web's own scripts, as the
// server's custom script did. It names the theme in the cookie, so that
// Jellyfin's branding, which jellyfin-web applies after its own CSS, gives
// the same theme's CSS, imported from the same address; jellyfin-web's
// SDK asks for it through fetch without the browser's cookies, so those
// requests, to the branding alone, go with the cookies of the page's own
// server. Once jellyfin-web shows that CSS, loaded, the script takes its
// own stylesheet away, and jellyfin-web keeps the theme where it always
// did, after its own CSS, and turns it off with the user's display
// settings. When the stylesheet differs from the one shown at the last
// load in this browser, it deletes the answers jellyfin-web kept in
// IndexedDB, as the fourteenth does, which would show the CSS of the theme
// before. Once jellyfin-web has a user signed in, as the route changes, or
// as a load signed in starts, looked for every half second for half a
// minute, it asks /Polyfin/WebTheme, once for each user signed in, and
// keeps their own theme. When that theme is another than the page shows:
// if the theme shown has a script, which cannot be taken back, it reloads
// the page, once for that user and theme in the tab, and only if the
// browser's storage kept the theme, which the load then shows; else it
// changes the stylesheet at once, turns off jellyfin-web's copy of the one
// before, and adds the new theme's script. Signing out keeps the theme.
// Storage that fails counts as none, and any error just stops it.
;(function () {
  var themes = null
  try {
    var element = document.getElementById('polyfinWebThemes')
    themes = element && JSON.parse(element.textContent)
  } catch (error) {}
  if (!themes || !themes.Themes || typeof themes.Cookie !== 'string') return
  var storageKey = 'polyfinWebTheme'
  var reloadKey = 'polyfinWebThemeReload'
  var mostUsers = 20
  var linkId = 'polyfinWebThemeCss'
  function quietly(listener) {
    return function (event) {
      try {
        listener(event)
      } catch (error) {}
    }
  }
  // jellyfin-web asks for the branding through fetch without the
  // browser's cookies (its SDK's axios sends credentials: 'omit'), which
  // would leave out the one naming the theme: those requests go with the
  // cookies of the page's own server. Its XMLHttpRequests send them anyway.
  var branding = /\/Branding\/(?:Configuration|Css(?:\.css)?)(?:[?#]|$)/i
  if (typeof window.fetch === 'function') {
    var originalFetch = window.fetch
    window.fetch = function (input, init) {
      var address = typeof input === 'string' ? input : input && input.url
      if (typeof address === 'string' && branding.test(address)) {
        var options = {}
        for (var name in init || {}) options[name] = init[name]
        options.credentials = 'same-origin'
        init = options
      }
      return originalFetch.call(window, input, init)
    }
  }
  // kept is what the browser keeps: Users, [server/user, theme] with the
  // last first, the theme empty for the server's; Last, the user last
  // signed in; Applied, the stylesheet the page showed, null when unknown.
  function kept() {
    try {
      var value = JSON.parse(window.localStorage.getItem(storageKey))
      if (value && Object.prototype.toString.call(value.Users) === '[object Array]') return value
    } catch (error) {}
    return { Users: [], Last: '', Applied: null }
  }
  // keep stores value, and tells whether the browser kept it.
  function keep(value) {
    try {
      var text = JSON.stringify(value)
      window.localStorage.setItem(storageKey, text)
      return window.localStorage.getItem(storageKey) === text
    } catch (error) {
      return false
    }
  }
  function ownTheme(value, user) {
    for (var i = 0; i < value.Users.length; i++) {
      if (value.Users[i] && value.Users[i][0] === user) return String(value.Users[i][1] || '')
    }
    return ''
  }
  function setOwnTheme(value, user, theme) {
    var users = [[user, theme]]
    for (var i = 0; i < value.Users.length && users.length < mostUsers; i++) {
      if (value.Users[i] && value.Users[i][0] !== user) users.push(value.Users[i])
    }
    value.Users = users
    value.Last = user
  }
  // savedUser is the user jellyfin-web's saved credentials sign in on the
  // server it used last, as server/user; "" when none.
  function savedUser() {
    try {
      var servers = JSON.parse(window.localStorage.getItem('jellyfin_credentials') || '{}').Servers || []
      var last = null
      for (var i = 0; i < servers.length; i++) {
        var server = servers[i]
        if (!server || !server.AccessToken || !server.UserId) continue
        if (!last || (server.DateLastAccessed || 0) > (last.DateLastAccessed || 0)) last = server
      }
      return last ? last.Id + '/' + last.UserId : ''
    } catch (error) {
      return ''
    }
  }
  var runsLumaaGlaass = null
  // shown is the theme the page shows for theme: LumaaGlaass where the
  // browser runs it, the Custom theme, else jellyfin-web's own look.
  function shown(theme) {
    if (theme === 'lumaaglaass' && runsLumaaGlaass === null) {
      runsLumaaGlaass = false
      try {
        runsLumaaGlaass =
          !!(window.CSS && CSS.supports && CSS.supports('selector(:has(a))')) &&
          typeof new Function('a', 'b', 'a ||= b?.c ?? 1; return a') === 'function'
      } catch (error) {}
    }
    if (theme === 'lumaaglaass' && runsLumaaGlaass) return theme
    return theme === 'custom' ? theme : 'none'
  }
  function filesOf(theme) {
    var files = themes.Themes[theme] || {}
    return { css: typeof files.Css === 'string' ? files.Css : '', js: typeof files.Js === 'string' ? files.Js : '' }
  }
  // cssOff tells whether the user, by their ID, "" signed out, turned off
  // the server's CSS in jellyfin-web's display settings, which jellyfin-web
  // keeps in the browser for them (appSettings).
  function cssOff(id) {
    try {
      return window.localStorage.getItem((id ? id + '-' : '') + 'disableCustomCss') === 'true'
    } catch (error) {
      return false
    }
  }
  function name(theme) {
    try {
      document.cookie = themes.Cookie + '=' + theme + '; path=/; samesite=lax'
    } catch (error) {}
  }
  // forgetKept deletes jellyfin-web's copy of its answers in IndexedDB, as
  // the fourteenth does.
  function forgetKept() {
    try {
      var request = indexedDB.open('keyval-store')
      request.onupgradeneeded = function () {
        request.transaction.abort()
      }
      request.onerror = function (event) {
        event.preventDefault()
      }
      request.onsuccess = function () {
        var database = request.result
        try {
          if (database.objectStoreNames.contains('keyval')) {
            database.transaction('keyval', 'readwrite').objectStore('keyval').delete('jellyfin-query-cache')
          }
        } finally {
          database.close()
        }
      }
    } catch (error) {}
  }
  function escaped(text) {
    return text.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
  }
  // copies are the style elements jellyfin-web shows the CSS at address in,
  // as Jellyfin's branding imports it.
  function copies(address) {
    var found = []
    var styles = document.getElementsByTagName('style')
    for (var i = 0; i < styles.length; i++) {
      if ((styles[i].textContent || '').indexOf("url('" + address + "')") >= 0) found.push(styles[i])
    }
    return found
  }
  // loaded tells whether the stylesheet a style element imports came.
  function loaded(style) {
    try {
      var rule = style.sheet && style.sheet.cssRules[0]
      return !!(rule && rule.styleSheet && rule.styleSheet.cssRules.length > 0)
    } catch (error) {
      return false
    }
  }
  var link = null
  var handing = null
  // handOver takes the script's stylesheet away once jellyfin-web shows
  // the same, loaded; it looks again every quarter of a second, for two
  // minutes.
  function handOver(tries) {
    clearTimeout(handing)
    if (!link) return
    var found = copies(link.getAttribute('href'))
    for (var i = 0; i < found.length; i++) {
      if (loaded(found[i])) {
        if (link.parentNode) link.parentNode.removeChild(link)
        link = null
        return
      }
    }
    if (tries < 480) {
      handing = setTimeout(
        quietly(function () {
          handOver(tries + 1)
        }),
        250
      )
    }
  }
  // setStylesheet shows the stylesheet at address, "" for none, in place
  // of the script's own: at the end of the page, after jellyfin-web's own
  // CSS, where jellyfin-web puts the branding's.
  function setStylesheet(address) {
    if (link && link.getAttribute('href') === address) return
    if (link && link.parentNode) link.parentNode.removeChild(link)
    link = null
    if (!address) return
    link = document.createElement('link')
    link.rel = 'stylesheet'
    link.id = linkId
    link.setAttribute('href', address)
    ;(document.body || document.head).appendChild(link)
    handOver(0)
  }

  var value = kept()
  var saved = savedUser()
  var user = saved || String(value.Last || '')
  var theme = shown((user && ownTheme(value, user)) || themes.Theme)
  var files = filesOf(theme)
  var off = cssOff(saved ? saved.slice(saved.indexOf('/') + 1) : '')
  // What the page shows: its theme, stylesheet and script.
  var applied = { theme: theme, css: files.css, js: files.js }
  name(theme)
  if (value.Applied !== files.css) forgetKept()
  value.Applied = files.css
  if (saved) value.Last = saved
  keep(value)
  // A load that shows the theme a reload was for ends that reload's mark.
  try {
    if (window.sessionStorage.getItem(reloadKey) === user + ' ' + theme) window.sessionStorage.removeItem(reloadKey)
  } catch (error) {}
  if (files.css && !off) {
    if (document.readyState === 'loading') {
      document.write('<link rel="stylesheet" id="' + linkId + '" href="' + escaped(files.css) + '">')
      link = document.getElementById(linkId)
      handOver(0)
    } else {
      setStylesheet(files.css)
    }
  }
  if (files.js) {
    if (document.readyState === 'loading') {
      document.write('<script src="' + escaped(files.js) + '" defer></' + 'script>')
    } else {
      var script = document.createElement('script')
      script.src = files.js
      document.head.appendChild(script)
    }
  }

  // The user whose theme was asked last, as server/user.
  var asked = ''
  // show shows theme, which the page did not show, for user, by their ID.
  function show(user, id, theme, value) {
    var files = filesOf(theme)
    if (applied.js) {
      // A theme's script cannot be taken back: the page loads again, and
      // shows the theme from its start, once for this user and theme.
      var reload = user + ' ' + theme
      var reloaded = ''
      try {
        reloaded = window.sessionStorage.getItem(reloadKey)
      } catch (error) {}
      if (reloaded === reload || !keep(value)) return
      try {
        window.sessionStorage.setItem(reloadKey, reload)
      } catch (error) {}
      name(theme)
      location.reload()
      return
    }
    // jellyfin-web's copy of the CSS before is turned off until it asks
    // again, and the next load forgets what it kept.
    name(theme)
    value.Applied = null
    keep(value)
    var before = copies(applied.css)
    for (var i = 0; i < before.length; i++) {
      if (before[i].sheet) before[i].sheet.disabled = true
    }
    setStylesheet(cssOff(id) ? '' : files.css)
    if (files.js) {
      var script = document.createElement('script')
      script.src = files.js
      document.head.appendChild(script)
    }
    applied = { theme: theme, css: files.css, js: files.js }
  }
  // follow asks the theme of the user jellyfin-web signed in, once for
  // each, and shows it when the page shows another.
  function follow() {
    var api = window.ApiClient
    var id = api && api.getCurrentUserId()
    if (!id) return
    var user = (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + id
    if (user === asked) return
    asked = user
    api.getJSON(api.getUrl('Polyfin/WebTheme')).then(
      quietly(function (answer) {
        if (asked !== user || !answer || typeof answer.Theme !== 'string') return
        var value = kept()
        setOwnTheme(value, user, typeof answer.UserTheme === 'string' ? answer.UserTheme : '')
        var theme = shown(answer.Theme)
        if (theme !== applied.theme) {
          show(user, id, theme, value)
          return
        }
        keep(value)
        // The user who signed in may have turned off the server's CSS.
        if (cssOff(id)) setStylesheet('')
      }),
      function () {}
    )
  }
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      quietly(follow)()
      return result
    }
  })
  addEventListener('hashchange', quietly(follow))
  addEventListener('popstate', quietly(follow))
  // A load signed in may move no route: jellyfin-web's user is looked for
  // every half second, for half a minute, until one is found.
  var looks = 0
  ;(function look() {
    quietly(follow)()
    if (!asked && looks++ < 60) setTimeout(look, 500)
  })()
})()
;(function () {
  var menus = document.createElement('style')
  menus.textContent =
    'select.detailTrackSelect.emby-select-withcolor{padding-right:2.2em!important;text-overflow:ellipsis}'
  document.head.appendChild(menus)
  var adminRoute = /^#!?\/(dashboard|metadata|configurationpage|wizard)(?:[/?#]|$)/i
  var titleRoute = /^#!?\/details\?(?:[^#]*&)?id=([0-9a-f]{32})(?:[&#]|$)/i
  // The first prefix of the dashboard page's path that matches wins.
  var pages = [
    ['users', 'users'],
    ['devices', 'users'],
    ['libraries', 'libraries'],
    ['livetv/recordings', 'settings/recordings'],
    ['livetv', 'live-tv'],
    ['playback/transcoding', 'settings/conversion'],
    ['playback/resume', 'settings/content'],
    ['playback/trickplay', 'settings/thumbnails'],
    ['playback', 'settings/playback'],
    ['branding', 'settings/web-player'],
    ['settings', 'settings/general'],
    ['backups', 'settings/backups'],
    ['logs', 'system/logs'],
    ['tasks', 'system/schedule'],
    ['keys', 'system/api-keys'],
    ['apikeys', 'system/api-keys'],
  ]
  // The admin page for a route: "" (home) unless it is a dashboard page with a match. A user's
  // dashboard pages name the user with ?userId=, which opens that user. A tuner's page names it
  // with ?id=, the identifier of its source among the server's, which opens that source; without
  // one, the page adds a tuner, which opens Add a source on its HDHomeRun tuner tab.
  function target(hash) {
    if (!/^#!?\/dashboard(?:[/?#]|$)/i.test(hash)) return ''
    var path = hash.replace(/^#!?\/dashboard\/?/i, '').split(/[?#]/)[0].toLowerCase()
    var user = /[?&]userId=([0-9a-f-]+)/i.exec(hash)
    // The admin app names users by 32 lowercase hexadecimal digits.
    if (user && /^users(\/|$)/.test(path)) return 'users/' + user[1].replace(/-/g, '').toLowerCase()
    if (/^livetv\/tuner(\/|$)/.test(path)) {
      var tuner = /[?&]id=([0-9a-f]{32})(?:[&#]|$)/i.exec(hash)
      return tuner ? 'sources/shared/' + tuner[1].toLowerCase() : 'sources?add=hdhomerun'
    }
    for (var i = 0; i < pages.length; i++) {
      var prefix = pages[i][0]
      if (path === prefix || path.indexOf(prefix + '/') === 0) return pages[i][1]
    }
    return ''
  }
  // Addresses are read and written by hand, never with URL: the script runs
  // before jellyfin-web's polyfills, and browsers jellyfin-web is built for
  // lack URL (before Chrome 32) or its searchParams (before Chrome 51).
  //
  // webFolder is the address of the web client's folder, /web/ or a folder
  // under Polyfin's base path, with its last slash.
  function webFolder() {
    var page = location.href.split('#')[0].split('?')[0]
    return page.slice(0, page.lastIndexOf('/') + 1)
  }
  // resolved is address as the page resolves it: one with a scheme as it
  // is, one from the server's root, such as /admin/…, on the page's server,
  // and any other in the web client's folder.
  function resolved(address) {
    if (/^[a-z][a-z0-9+.-]*:/i.test(address)) return address
    var root = /^([a-z][a-z0-9+.-]*:)\/\/[^/?#]*/i.exec(location.href)
    if (root && address.indexOf('//') === 0) return root[1] + address
    if (root && address.charAt(0) === '/') return root[0] + address
    return webFolder() + address
  }
  // adminAddress is the address of the admin app's page path ("" for its
  // home), which '../admin/' leads to from the web client's folder, never
  // above the server's root.
  function adminAddress(path) {
    var folder = webFolder()
    var root = /^[a-z][a-z0-9+.-]*:\/\/[^/]*\//i.exec(folder)
    var above = folder.replace(/[^/]+\/$/, '')
    return (root && above.length < root[0].length ? root[0] : above) + 'admin/' + path
  }
  // An address of the admin app, marked as opened from the web client
  // (?from=web): the admin app then shows the user signed in here, and
  // signs out a session of another user it had in this browser.
  function fromWebClient(address) {
    var at = address.indexOf('#')
    var rest = at < 0 ? address : address.slice(0, at)
    var join = rest.indexOf('?') < 0 ? '?' : /[?&]$/.test(rest) ? '' : '&'
    return rest + join + 'from=web' + (at < 0 ? '' : address.slice(at))
  }
  // marks is a WeakSet (kind "set") or a WeakMap ("map") of the page's
  // elements, or, before Chrome 36, which has neither, the same kept on
  // each element under a property named after name: either way, an element
  // takes what was kept for it when it goes.
  function marks(kind, name) {
    if (kind === 'set' && typeof WeakSet === 'function') return new WeakSet()
    if (kind === 'map' && typeof WeakMap === 'function') return new WeakMap()
    var key = '__polyfin' + name
    return {
      has: function (element) {
        return Object.prototype.hasOwnProperty.call(element, key)
      },
      get: function (element) {
        return Object.prototype.hasOwnProperty.call(element, key) ? element[key] : undefined
      },
      set: function (element, value) {
        element[key] = value
        return this
      },
      add: function (element) {
        element[key] = true
        return this
      },
    }
  }
  // attached tells whether node is in the page, as isConnected does from
  // Chrome 54 on.
  function attached(node) {
    if (typeof node.isConnected === 'boolean') return node.isConnected
    return document.documentElement.contains(node)
  }
  // The title whose versions are followed, the next time they are asked,
  // and how many times the script began asking, the latest only going on.
  var title = ''
  var timer = null
  var polls = 0
  // The title's last answer, the count the page was last updated or
  // reloaded for, and whether its details are being asked.
  var latest = null
  var updatedFor = 0
  var asking = false
  // Whether Polyfin pushed the title's progress, and the ApiClient whose
  // messages the script hears.
  var pushed = false
  var heard = null
  // The options this script added to the version menu, and the size and
  // the ETag of the version each option stood for when the script last
  // listed it. Polyfin gives each source the identifier of its version as
  // its ETag, the first source's included, which carries the title's.
  // jellyfin-web fills the menu with new options, which forgets them all:
  // given, the sources the script was last given by identifier, tells them
  // again for an option jellyfin-web listed under a source's identifier and
  // name.
  var added = marks('set', 'Added')
  var sizes = marks('map', 'Size')
  var tags = marks('map', 'Tag')
  var given = {}
  // The version picked among those added, until jellyfin-web lists it: the
  // observer that waits for jellyfin-web's options, and the identifier of
  // the version picked.
  var picking = null
  // shownPage returns the title page shown, unless a video is.
  function shownPage() {
    if (document.querySelector('.videoPlayerContainer')) return null
    return document.querySelector('.page.itemDetailPage:not(.hide)')
  }
  // reload reloads a title page's details as jellyfin-web does when the page
  // is shown again: it asks for them, lists their versions keeping the one
  // picked, and fills the track menus.
  function reload(page) {
    page.dispatchEvent(new CustomEvent('viewbeforehide', { detail: {} }))
    page.dispatchEvent(new CustomEvent('viewshow', { detail: { isRestored: false } }))
  }
  // stale tells whether an option no longer stands for a source with its
  // identifier: the title's own identifier passes from the placeholder to
  // the first version, then to whichever version comes first, the version
  // it leaves taking an identifier of its own. When the script knows the
  // option's ETag, a version keeping its ETag as its name or size changes
  // is the same; otherwise jellyfin-web names an option after its source,
  // and the name (see named) and the size the script knows tell.
  function stale(option, source) {
    if (tags.has(option) && source.ETag) return tags.get(option) !== source.ETag
    return !named(option.textContent, source.Name) || (sizes.has(option) && sizes.get(option) !== source.Size)
  }
  // named tells whether an option's text names a source: its name, which
  // may have gained, lost or changed the ending in parentheses that says
  // how the app plays it, such as " (direct play)", since jellyfin-web
  // listed it (Settings › Playback › Say in version names how they play).
  function named(text, name) {
    var noted = / \([^()]*\)$/
    return (
      text === name ||
      (name.indexOf(text + ' (') === 0 && noted.test(name)) ||
      (text.indexOf(name + ' (') === 0 && noted.test(text)) ||
      (noted.test(text) && noted.test(name) && text.replace(noted, '') === name.replace(noted, ''))
    )
  }
  // remember notes the size and the ETag of the source an option stands for.
  function remember(option, source) {
    sizes.set(option, source.Size)
    if (source.ETag) tags.set(option, source.ETag)
    else tags.delete(option)
  }
  // place lists sources in the version menu, in their order, keeping the
  // version picked, and tells whether it could. An option jellyfin-web
  // listed is kept while it stands for the same source, as jellyfin-web
  // knows that source's tracks, and named as the source is now. When the
  // version picked lost its identifier, to another or because Polyfin names
  // it otherwise now, it cannot: it is picked under its new one (see keep).
  function place(menu, sources) {
    var picked = menu.options[menu.selectedIndex]
    var kept = {}
    for (var i = 0; i < menu.options.length; i++) {
      var listed = menu.options[i]
      var known = given[listed.value]
      if (!sizes.has(listed) && known && named(listed.textContent, known.Name)) remember(listed, known)
      kept[listed.value] = listed
    }
    given = {}
    sources.forEach(function (source) {
      given[source.Id] = source
    })
    var found = false
    var options = sources.map(function (source) {
      var option = kept[source.Id]
      if (option && stale(option, source)) option = null
      if (!option) option = sourceOption(source)
      else if (option.textContent !== source.Name) option.textContent = source.Name
      if (option === picked) found = true
      remember(option, source)
      return option
    })
    if (!found) {
      if (picked) keep(menu, picked, sources)
      return false
    }
    options.forEach(function (option, i) {
      if (menu.options[i] !== option) menu.insertBefore(option, menu.options[i] || null)
    })
    while (menu.options.length > options.length) menu.removeChild(menu.options[options.length])
    menu.value = picked.value
    var container = menu.closest('.selectSourceContainer')
    if (container) container.classList.toggle('hide', options.length < 2)
    return true
  }
  // sourceOption makes an option for a source, as jellyfin-web does.
  function sourceOption(source) {
    var option = document.createElement('option')
    option.value = source.Id
    option.textContent = source.Name
    added.add(option)
    remember(option, source)
    return option
  }
  // keep picks, in place of the option picked, the version it stood for
  // when its identifier no longer names it: the title's identifier passed
  // from it to a version listed first, or Polyfin names it otherwise now.
  // That version is the source with its ETag, when the script knows it,
  // else the one source with its name, and its size when both are known,
  // under another identifier; several such sources tell nothing. The
  // page's details are then reloaded, and jellyfin-web keeps the identifier
  // picked when it lists it, where it would pick the version that took the
  // old one, or the first. Without such a source, as when the placeholder
  // gives way to the first version, the option stays, and jellyfin-web
  // picks the version that took its identifier, or the first. A version
  // picked among those added is then made again under its new identifier
  // (see pick).
  function keep(menu, picked, sources) {
    var tag = tags.get(picked)
    var size = sizes.get(picked)
    var found = null
    var alike = []
    for (var i = 0; i < sources.length; i++) {
      var source = sources[i]
      if (source.Id === picked.value) continue
      if (tag && source.ETag === tag) found = found || source
      var sized = !(size > 0 && source.Size > 0 && size !== source.Size)
      if (named(picked.textContent, source.Name) && sized) alike.push(source)
    }
    if (!found && alike.length === 1) found = alike[0]
    if (!found) return
    menu.insertBefore(sourceOption(found), picked)
    menu.removeChild(picked)
    menu.value = found.Id
    if (picking) picking.value = found.Id
  }
  // update brings the title page to the versions Polyfin knows when an
  // answer tells it lists fewer, or a different number once no addon is
  // pending. A hidden or empty version menu counts as one. When Polyfin
  // adds no version to an open page (AddVersions false), only a menu that
  // lists the placeholder, named after the title, is brought to them.
  function update(progress) {
    var id = title
    var page = shownPage()
    if (!id || !page || asking || progress.Count < 1 || progress.Count === updatedFor) return
    var adding = progress.AddVersions !== false
    var menu = page.querySelector('.selectSource')
    if (!adding && (!menu || !placeholder(page))) return
    var listed = !menu || menu.closest('.hide') ? 1 : Math.max(menu.options.length, 1)
    if (progress.Count < listed ? progress.Pending > 0 : progress.Count === listed) return
    if (!menu) {
      reload(page)
      updatedFor = progress.Count
      return
    }
    if (document.activeElement === menu) return
    var api = window.ApiClient
    asking = true
    api.getItem(api.getCurrentUserId(), id).then(
      function (item) {
        asking = false
        if (title !== id || shownPage() !== page || page.querySelector('.selectSource') !== menu) return
        if (document.activeElement === menu) return
        if (adding || (placeholder(page) && menu.options[0].textContent === item.Name)) {
          if (!place(menu, item.MediaSources || [])) reload(page)
        }
        updatedFor = progress.Count
      },
      function () {
        asking = false
      }
    )
  }
  // listen hears the messages of jellyfin-web's ApiClient, as its message
  // event gives them: jellyfin-web's events keep an object's listeners in
  // its _callbacks, by event, and call each with the event then the message.
  function listen() {
    var api = window.ApiClient
    if (!api || api === heard) return
    heard = api
    var callbacks = (api._callbacks = api._callbacks || {})
    ;(callbacks.message = callbacks.message || []).push(function (event, message) {
      try {
        hear(message)
      } catch (error) {}
    })
  }
  // hear takes a pushed progress of the title followed as an answer.
  function hear(message) {
    if (!message || message.MessageType !== 'PolyfinVersions' || !message.Data) return
    var progress = message.Data
    if (!title || String(progress.ItemId).toLowerCase() !== title) return
    pushed = true
    unsure = false
    take(progress)
  }
  // take brings the page to an answer.
  function take(progress) {
    latest = progress
    if (early) early.disconnect()
    early = null
    hold()
    update(progress)
  }
  // The words the script adds to a page, in jellyfin-web's language, which
  // it gives the document; English for any other.
  var words = {
    en: {
      searching: 'Looking for sources…',
      none: 'No source is available for this title.',
      again: 'Try again',
      removeResume: 'Remove from Continue Watching',
      hide: 'Hide this title',
      show: 'Show this title again',
      hidden: 'This title is hidden from your lists.',
      shown: 'This title is back in your lists.',
    },
    fr: {
      searching: 'Recherche des sources…',
      none: 'Aucune source disponible pour ce titre.',
      again: 'Réessayer',
      removeResume: 'Retirer de « Continuer de regarder »',
      hide: 'Masquer ce titre',
      show: 'Afficher de nouveau ce titre',
      hidden: 'Ce titre est masqué de vos listes.',
      shown: 'Ce titre est de retour dans vos listes.',
    },
    de: {
      searching: 'Quellen werden gesucht…',
      none: 'Für diesen Titel ist keine Quelle verfügbar.',
      again: 'Erneut versuchen',
      removeResume: 'Aus „Weiterschauen“ entfernen',
      hide: 'Diesen Titel ausblenden',
      show: 'Diesen Titel wieder einblenden',
      hidden: 'Dieser Titel ist in deinen Listen ausgeblendet.',
      shown: 'Dieser Titel ist wieder in deinen Listen.',
    },
    es: {
      searching: 'Buscando fuentes…',
      none: 'No hay ninguna fuente disponible para este título.',
      again: 'Reintentar',
      removeResume: 'Quitar de «Seguir viendo»',
      hide: 'Ocultar este título',
      show: 'Volver a mostrar este título',
      hidden: 'Este título está oculto en sus listas.',
      shown: 'Este título vuelve a estar en sus listas.',
    },
    it: {
      searching: 'Ricerca delle origini…',
      none: 'Nessuna origine disponibile per questo titolo.',
      again: 'Riprova',
      removeResume: 'Rimuovi da «Continua a guardare»',
      hide: 'Nascondi questo titolo',
      show: 'Mostra di nuovo questo titolo',
      hidden: 'Questo titolo è nascosto dai tuoi elenchi.',
      shown: 'Questo titolo è di nuovo nei tuoi elenchi.',
    },
    pt: {
      searching: 'Procurando fontes…',
      none: 'Nenhuma fonte disponível para este título.',
      again: 'Tentar novamente',
      removeResume: 'Remover de “Continuar assistindo”',
      hide: 'Ocultar este título',
      show: 'Mostrar este título novamente',
      hidden: 'Este título está oculto nas suas listas.',
      shown: 'Este título voltou para as suas listas.',
    },
  }
  function say(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (words[language] || words.en)[key]
  }
  // The play buttons held, each with what the script changed as it was
  // before and the tooltip it gave; the line telling that no source is
  // available; the page watched for jellyfin-web's renders, and the document
  // until the first answer; whether the latest answer may have come before
  // the title's details asked any addon; and when the title's addons may be
  // asked again.
  var held = []
  var note = null
  var watched = null
  var watcher = null
  var early = null
  var unsure = false
  var againAt = 0
  var againTimer = null
  // waiting tells what the play buttons of page wait for: "searching" while
  // no version is known and addons are asked, "none" once none is left to
  // ask, "" when a version is known or the item is not a movie or an
  // episode, which counts no source at all. The first answer for a title
  // opened is asked as the route changes, before its details may have asked
  // any addon: no source then counts as searching until the next answer.
  // Before any answer, the buttons wait as searching when the version menu
  // lists only the placeholder, as item details do while no version is
  // known: a single source under the title's own identifier.
  function waiting(page) {
    var progress = latest
    if (!progress) return placeholder(page) ? 'searching' : ''
    if (!(progress.Count >= 1) || typeof progress.Known !== 'number' || progress.Known > 0) return ''
    return progress.Pending > 0 || unsure ? 'searching' : 'none'
  }
  function placeholder(page) {
    var menu = page.querySelector('.selectSource')
    return !!menu && menu.options.length === 1 && String(menu.options[0].value).toLowerCase() === title
  }
  // hold brings the play buttons of the title page shown to the latest
  // answer: disabled, with a tooltip, greyed and a spinner while addons are
  // searched; disabled, with a line under them and a button to search again,
  // when no source is available; as jellyfin-web made them otherwise. It
  // watches the page, so that jellyfin-web's renders find them held again,
  // and changes only what differs. It never changes the page during a video.
  function hold() {
    try {
      var page = shownPage()
      if (!page) return
      var state = waiting(page)
      if (!state && !held.length && !note) return
      watch(page)
      if (!state) return release()
      style()
      var buttons = page.querySelectorAll('.btnPlay, .btnResume, .btnReplay')
      var tooltip = say(state === 'searching' ? 'searching' : 'none')
      held = held.filter(function (entry) {
        if (Array.prototype.indexOf.call(buttons, entry.button) >= 0) return true
        restore(entry)
        return false
      })
      Array.prototype.forEach.call(buttons, function (button) {
        holdButton(button, tooltip, state === 'searching')
      })
      if (state === 'none' && buttons.length) tell(buttons[0])
      else if (note) {
        note.remove()
        note = null
      }
    } catch (error) {}
  }
  function holdButton(button, tooltip, spinning) {
    var entry = null
    for (var i = 0; i < held.length; i++) if (held[i].button === button) entry = held[i]
    if (!entry) {
      entry = {
        button: button,
        title: button.getAttribute('title'),
        disabled: button.hasAttribute('disabled'),
        aria: button.getAttribute('aria-disabled'),
        tooltip: null,
      }
      held.push(entry)
    } else if (button.getAttribute('title') !== entry.tooltip) {
      // jellyfin-web rendered the page again, with its own tooltip.
      entry.title = button.getAttribute('title')
    }
    entry.tooltip = tooltip
    if (button.getAttribute('title') !== tooltip) button.setAttribute('title', tooltip)
    if (!button.hasAttribute('disabled')) button.setAttribute('disabled', '')
    if (button.getAttribute('aria-disabled') !== 'true') button.setAttribute('aria-disabled', 'true')
    if (!button.classList.contains('polyfinHeld')) button.classList.add('polyfinHeld')
    var spinner = button.querySelector('.polyfinSpinner')
    if (spinning && !spinner) {
      spinner = document.createElement('span')
      spinner.className = 'polyfinSpinner'
      spinner.setAttribute('aria-hidden', 'true')
      button.appendChild(spinner)
    } else if (!spinning && spinner) spinner.remove()
  }
  // restore gives a button back as jellyfin-web made it.
  function restore(entry) {
    var button = entry.button
    if (entry.title === null) button.removeAttribute('title')
    else if (button.getAttribute('title') === entry.tooltip) button.setAttribute('title', entry.title)
    if (!entry.disabled) button.removeAttribute('disabled')
    if (entry.aria === null) button.removeAttribute('aria-disabled')
    else button.setAttribute('aria-disabled', entry.aria)
    button.classList.remove('polyfinHeld')
    var spinner = button.querySelector('.polyfinSpinner')
    if (spinner) spinner.remove()
  }
  // release restores every button held and removes the line.
  function release() {
    var entries = held
    held = []
    entries.forEach(restore)
    if (note) note.remove()
    note = null
  }
  // tell shows, under the play buttons, that no source is available, with a
  // button that asks the addons again. The buttons share a row with the
  // title's name, which the line goes under, aligned with them.
  function tell(button) {
    var row = button.closest('.detailRibbon') || button.closest('.mainDetailButtons') || button.parentNode
    if (!note) {
      note = document.createElement('div')
      note.className = 'polyfinNoSource padded-left padded-right'
      note.appendChild(document.createElement('span'))
      var again = document.createElement('button')
      again.setAttribute('type', 'button')
      again.className = 'raised emby-button polyfinAgain'
      again.addEventListener('click', function () {
        try {
          searchAgain()
        } catch (error) {}
      })
      note.appendChild(again)
    }
    var text = note.firstChild
    var again = note.lastChild
    if (text.textContent !== say('none')) text.textContent = say('none')
    if (again.textContent !== say('again')) again.textContent = say('again')
    var wait = Date.now() < againAt
    if (again.hasAttribute('disabled') !== wait) {
      if (wait) again.setAttribute('disabled', '')
      else again.removeAttribute('disabled')
    }
    if (row.nextSibling !== note) row.parentNode.insertBefore(note, row.nextSibling)
  }
  // searchAgain has Polyfin ask the title's addons again, then follows its
  // versions anew. Asked too soon, the button waits as long as Polyfin says.
  function searchAgain() {
    var id = title
    if (!id || Date.now() < againAt) return
    var api = window.ApiClient
    api.ajax({ type: 'POST', url: api.getUrl('Polyfin/Items/' + id + '/Versions/Search'), dataType: 'json' }).then(
      function (progress) {
        if (title !== id) return
        unsure = false
        take(progress)
        poll(id, 1000)
      },
      function (response) {
        var seconds = Number(response && response.headers && response.headers.get('Retry-After'))
        if (title !== id || !(seconds > 0)) return
        againAt = Date.now() + seconds * 1000
        hold()
        clearTimeout(againTimer)
        againTimer = setTimeout(hold, seconds * 1000)
      }
    )
  }
  // watch has jellyfin-web's renders of the page shown find its buttons held.
  function watch(page) {
    if (watched === page) return
    if (watcher) watcher.disconnect()
    watched = page
    watcher = new MutationObserver(hold)
    var changes = { childList: true, subtree: true, attributes: true, attributeFilter: ['title', 'disabled', 'class'] }
    watcher.observe(page, changes)
  }
  // style adds the look of the buttons held and of the line, once.
  function style() {
    if (document.getElementById('polyfinStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinStyle'
    sheet.textContent =
      '.polyfinHeld{position:relative;opacity:.45;cursor:default}' +
      '.polyfinSpinner{position:absolute;top:50%;left:50%;width:1.1em;height:1.1em;margin:-.55em 0 0 -.55em;box-sizing:border-box;' +
      'border:.15em solid currentColor;border-right-color:transparent;border-radius:50%;animation:polyfinSpin .8s linear infinite}' +
      '@keyframes polyfinSpin{to{transform:rotate(360deg)}}' +
      '.polyfinNoSource{display:flex;flex-wrap:wrap;justify-content:flex-end;align-items:center;gap:.5em 1em;margin:.5em 0}' +
      '.polyfinAgain[disabled]{opacity:.45;cursor:default}'
    document.head.appendChild(sheet)
  }
  function followVersions() {
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === title) return
    clearTimeout(timer)
    timer = null
    title = id
    latest = null
    updatedFor = 0
    given = {}
    pushed = false
    if (picking) picking.observer.disconnect()
    picking = null
    release()
    if (watcher) watcher.disconnect()
    watcher = null
    watched = null
    if (early) early.disconnect()
    early = null
    unsure = true
    clearTimeout(againTimer)
    againTimer = null
    againAt = 0
    if (!id) return
    // Until the first answer, the page is looked for as jellyfin-web renders
    // it, in case it lists only the placeholder.
    if (document.body) {
      early = new MutationObserver(hold)
      early.observe(document.body, { childList: true, subtree: true })
    }
    hold()
    poll(id, 0)
  }
  // poll asks for the title's versions for 90 seconds, from scratch, first
  // after delay, then every second, or every 10 once a push came.
  function poll(id, delay) {
    clearTimeout(timer)
    timer = null
    var round = ++polls
    var until = Date.now() + 90000
    var answers = 0
    function ask() {
      timer = null
      if (title !== id || round !== polls || Date.now() >= until) return
      var api = window.ApiClient
      // jellyfin-web may not have made its client yet, on a page opened at
      // a title's address.
      if (!api) return next()
      listen()
      api
        .getJSON(api.getUrl('Polyfin/Items/' + id + '/Versions'))
        .then(function (progress) {
          if (title !== id || round !== polls) return
          answers++
          if (answers > 1) unsure = false
          take(progress)
          // The page asks for its details as the route changes: the first
          // answers may come before Polyfin asks any addon.
          if (progress.Pending > 0 || answers < 3) next()
        })
        .catch(function () {})
    }
    function next() {
      timer = setTimeout(attempt, pushed ? 10000 : 1000)
    }
    function attempt() {
      try {
        ask()
      } catch (error) {}
    }
    if (delay) timer = setTimeout(attempt, delay)
    else attempt()
  }
  // jellyfin-web keeps its own copy of the versions it listed, and fills the
  // track menus from it as soon as a version is picked, before it asks for
  // that version's details: a version this script added is not in it. Such a
  // pick goes to jellyfin-web only once it lists that version: the page's
  // details are reloaded, which keeps the pick, and it is made again when
  // jellyfin-web has filled the menu anew, under the version's identifier
  // then, which keep follows when the title's identifier leaves it.
  function pick(event) {
    var menu = event.target
    var option = menu && menu.options && menu.options[menu.selectedIndex]
    var page = option && added.has(option) && menu.closest('.itemDetailPage')
    if (!page) return
    event.stopPropagation()
    if (picking) picking.observer.disconnect()
    var entry = { observer: null, value: menu.value }
    entry.observer = new MutationObserver(function () {
      var picked = menu.options[menu.selectedIndex]
      // This script's own changes leave the pick among its options.
      if (picked && added.has(picked)) return
      entry.observer.disconnect()
      if (picking === entry) picking = null
      if (picked && picked.value === entry.value) menu.dispatchEvent(new Event('change', { bubbles: true }))
    })
    picking = entry
    entry.observer.observe(menu, { childList: true })
    reload(page)
  }
  // A version menu that had focus is updated as soon as it loses it.
  function leave(event) {
    var menu = event.target
    if (latest && menu && menu.matches && menu.matches('.selectSource')) update(latest)
  }
  // quietly runs a listener, an error leaving the event to jellyfin-web.
  function quietly(listener) {
    return function (event) {
      try {
        listener(event)
      } catch (error) {}
    }
  }
  function follow() {
    if (adminRoute.test(location.hash)) {
      // Replaced, so that Back returns to the page before.
      location.replace(fromWebClient(adminAddress(target(location.hash))))
      return
    }
    try {
      followVersions()
    } catch (error) {}
    try {
      followViews()
    } catch (error) {}
    try {
      followScores()
    } catch (error) {}
    try {
      followShare()
    } catch (error) {}
  }
  // The user signed in whose views were asked for, the style hiding the
  // views they hide from the menus, and until when the script looks for a
  // user signed in after a route change.
  var viewer = ''
  var viewSheet = null
  var viewTimer = null
  var viewsUntil = 0
  // The views the top bar shows before its More button, at the widths of
  // MUI's breakpoints that UserViewNav follows: below lg (1200px), from lg
  // to xl (1536px), from xl. The ranges end as MUI's own queries do.
  var barWidths = [
    [3, '(max-width:1199.95px)'],
    [5, '(min-width:1200px) and (max-width:1535.95px)'],
    [8, '(min-width:1536px)'],
  ]
  // Where the last answer is kept, as {user, items}: the user it was for
  // and its Items, all of them, in order, as the More button needs.
  var savedViewsKey = 'polyfinUserViews'
  // savedViews returns the answer kept, or null when there is none, storage
  // fails or it holds anything else.
  function savedViews() {
    try {
      var saved = JSON.parse(window.localStorage.getItem(savedViewsKey))
      if (saved && typeof saved.user === 'string' && saved.user && Array.isArray(saved.items)) return saved
    } catch (error) {}
    return null
  }
  function saveViews(user, items) {
    try {
      window.localStorage.setItem(savedViewsKey, JSON.stringify({ user: user, items: items }))
    } catch (error) {}
  }
  // The user whose views the style hides now: the answer kept hides them
  // as the script starts, before jellyfin-web draws its menus.
  var hiddenFor = ''
  try {
    var saved = savedViews()
    if (saved) {
      hideViews(saved.items)
      hiddenFor = saved.user
    }
  } catch (error) {}
  // followViews asks for the views of the user signed in when the user
  // changes (see the header), forgetting those of the user before.
  function followViews() {
    viewsUntil = Date.now() + 30000
    lookForViewer()
  }
  function lookForViewer() {
    clearTimeout(viewTimer)
    viewTimer = null
    var api = window.ApiClient
    var user = (api && api.getCurrentUserId()) || ''
    if (user !== viewer) {
      viewer = user
      // The rules kept stay for their own user until the answer comes.
      if (user !== hiddenFor) {
        hiddenFor = ''
        hideViews([])
      }
      placeEntries()
      if (user) {
        askViews(api, user)
        askRole(api, user)
      }
    }
    if (!user && Date.now() < viewsUntil) viewTimer = setTimeout(quietly(lookForViewer), 1000)
  }
  // askViews asks for the views of user, hides them and keeps the answer,
  // unless another user signed in meanwhile.
  function askViews(api, user) {
    api
      .getJSON(api.getUrl('Polyfin/UserViews'))
      .then(function (views) {
        if (viewer !== user) return
        var items = (views && views.Items) || []
        hideViews(items)
        hiddenFor = user
        saveViews(user, items)
      })
      .catch(function () {})
  }
  // hideViews has the style hide the links of the views with HideInMenus,
  // and the More button at the widths where it would list only such views.
  // An identifier goes in as a JSON string, whose escaped quotes and
  // backslashes CSS reads the same way.
  function hideViews(views) {
    function hides(view) {
      return !!view && view.HideInMenus === true
    }
    var rules = ''
    views.forEach(function (view) {
      if (!hides(view)) return
      var id = JSON.stringify(String(view.Id))
      rules +=
        '.MuiAppBar-root a.MuiButton-root[href*=' + id + '],' +
        '#user-view-overflow-menu a[href*=' + id + '],' +
        '.MuiDrawer-root a[href*=' + id + '],' +
        '.libraryMenuOptions a[data-itemid=' + id + ']{display:none!important}'
    })
    barWidths.forEach(function (width) {
      var shown = width[0]
      if (views.length > shown + 1 && views.slice(shown).every(hides)) {
        rules += '@media ' + width[1] + '{.MuiAppBar-root button[aria-controls="user-view-overflow-menu"]{display:none!important}}'
      }
    })
    if (!viewSheet) {
      if (!rules) return
      viewSheet = document.createElement('style')
      document.head.appendChild(viewSheet)
    }
    if (viewSheet.textContent !== rules) viewSheet.textContent = rules
  }
  // The member the "Polyfin" entry is for (see the header), as the last
  // answer kept says, and where that answer is kept, as {user}.
  var savedMemberKey = 'polyfinMember'
  var memberFor = ''
  try {
    var savedMember = JSON.parse(window.localStorage.getItem(savedMemberKey))
    if (savedMember && typeof savedMember.user === 'string') memberFor = savedMember.user
  } catch (error) {}
  // askRole asks jellyfin-web's client whether user is an administrator,
  // keeps the answer, the member or none, unless another user signed in
  // meanwhile, and shows or takes away the entry.
  function askRole(api, user) {
    function plain(id) {
      return String(id).replace(/-/g, '').toLowerCase()
    }
    api
      .getCurrentUser()
      .then(function (current) {
        if (viewer !== user || !current || !current.Policy || plain(current.Id) !== plain(user)) return
        memberFor = current.Policy.IsAdministrator === true ? '' : user
        try {
          if (memberFor) window.localStorage.setItem(savedMemberKey, JSON.stringify({ user: memberFor }))
          else window.localStorage.removeItem(savedMemberKey)
        } catch (error) {}
        placeEntries()
      })
      .catch(function () {})
  }
  // What marks the script's entries, each also with the class of its kind;
  // where jellyfin-web's Dashboard links; and Dashboard's icon in the user
  // menu, MUI's.
  var entryClass = 'polyfinEntry'
  var dashboardLink = 'a[href="#/dashboard"]'
  var dashboardIcon = 'M3 13h8V3H3zm0 8h8v-6H3zm10 0h8V11h-8zm0-18v6h8V3z'
  // placeEntries shows the entry in each menu drawn where jellyfin-web shows
  // administrators Dashboard, while the user signed in is the member, and
  // takes away every other. It changes only what differs: its own changes
  // call it again. While no entry is shown and the user is no member, it
  // reads nothing of the page, which changes often during a video.
  var shown = false
  function placeEntries() {
    try {
      var api = window.ApiClient
      var user = (api && api.getCurrentUserId()) || ''
      var member = user !== '' && user === memberFor
      if (!member && !shown) return
      var kept = []
      if (member) {
        // The legacy layout's side menu, first in its user section: right
        // after the libraries, without a heading, it would read as one.
        each('.mainDrawer .userMenuOptions', function (section) {
          var heading = section.querySelector('.sidebarHeader')
          if (!heading || section.parentNode.querySelector(dashboardLink)) return
          var entry = heading.nextElementSibling
          if (!entry || !entry.classList.contains('polyfinDrawerEntry')) {
            entry = drawerEntry()
            section.insertBefore(entry, heading.nextSibling)
          }
          kept.push(entry)
        })
        // The settings page, before its user section.
        each('.userSection', function (section) {
          if (!section.parentNode.querySelector(dashboardLink)) kept.push(before(section, 'polyfinSettingsEntry', settingsEntry))
        })
        // The default layout's user menu, before the divider over
        // Quick Connect, Select server and Sign out, after one of its own.
        each('#app-user-menu', function (menu) {
          var settings = menu.querySelector('a[href="#/mypreferencesmenu"]')
          var icon = menu.querySelector('svg[data-testid="LogoutIcon"]')
          var divider = icon && icon.closest('.MuiMenuItem-root')
          while (divider && !(divider.classList.contains('MuiDivider-root') && !divider.classList.contains(entryClass))) {
            divider = divider.previousElementSibling
          }
          if (!settings || !divider || menu.querySelector(dashboardLink)) return
          var item = before(divider, 'polyfinMenuEntry', function () {
            return menuEntry(settings)
          })
          if (!item) return
          kept.push(
            item,
            before(item, 'polyfinMenuDivider', function () {
              var line = divider.cloneNode(true)
              line.classList.add(entryClass, 'polyfinMenuDivider')
              return line
            })
          )
        })
      }
      each('.' + entryClass, function (entry) {
        if (kept.indexOf(entry) < 0) entry.remove()
      })
      shown = kept.length > 0
    } catch (error) {}
  }
  function each(selector, run) {
    Array.prototype.forEach.call(document.querySelectorAll(selector), run)
  }
  // before returns the entry of a kind right before node, made by make
  // unless it is there; null when make makes none.
  function before(node, kind, make) {
    var previous = node.previousElementSibling
    if (previous && previous.classList.contains(kind)) return previous
    var entry = make()
    if (entry) node.parentNode.insertBefore(entry, node)
    return entry
  }
  // element makes an element with classes and a text or children.
  function element(tag, className, content) {
    var made = document.createElement(tag)
    made.className = className
    if (typeof content === 'string') made.textContent = content
    else content.forEach(made.appendChild, made)
    return made
  }
  // A link to the admin app, as its dashboard routes open it. A plain link:
  // jellyfin-web's emby-linkbutton would open it within jellyfin-web.
  function adminLink(className, content) {
    var link = element('a', className, content)
    link.setAttribute('href', fromWebClient(adminAddress('')))
    return link
  }
  function materialIcon(className) {
    var icon = element('span', 'material-icons ' + className + ' dashboard', '')
    icon.setAttribute('aria-hidden', 'true')
    return icon
  }
  // The entry as the legacy side menu draws Dashboard.
  function drawerEntry() {
    return adminLink(entryClass + ' polyfinDrawerEntry navMenuOption lnkMediaFolder emby-button', [
      materialIcon('navMenuOptionIcon'),
      element('span', 'navMenuOptionText', 'Polyfin'),
    ])
  }
  // The entry as the settings page draws Dashboard, in a section of its own
  // without the Administration heading.
  function settingsEntry() {
    var body = element('div', 'listItemBody', [element('div', 'listItemBodyText', 'Polyfin')])
    var link = adminLink('emby-button listItem-border', [
      element('div', 'listItem', [materialIcon('listItemIcon listItemIcon-transparent'), body]),
    ])
    link.setAttribute('style', 'display:block;margin:0;padding:0')
    var section = 'verticalSection verticalSection-extrabottompadding'
    return element('div', entryClass + ' polyfinSettingsEntry ' + section, [link])
  }
  // The entry as the user menu draws Dashboard: a copy of its Settings
  // item, MUI's own, with Dashboard's icon.
  function menuEntry(settings) {
    var item = settings.cloneNode(true)
    var text = item.querySelector('.MuiListItemText-primary')
    var path = item.querySelector('svg path')
    if (!text || !path) return null
    item.classList.add(entryClass, 'polyfinMenuEntry')
    item.classList.remove('Mui-focusVisible')
    item.setAttribute('href', fromWebClient(adminAddress('')))
    text.textContent = 'Polyfin'
    path.setAttribute('d', dashboardIcon)
    path.parentNode.removeAttribute('data-testid')
    var ripple = item.querySelector('.MuiTouchRipple-root')
    if (ripple) ripple.textContent = ''
    return item
  }
  // jellyfin-web draws its menus anew, which the script, loaded in the
  // head, follows once the body exists.
  function watchMenus() {
    if (typeof MutationObserver !== 'function' || !document.body) return
    new MutationObserver(placeEntries).observe(document.body, { childList: true, subtree: true })
  }
  // The link pressed or clicked, if it opens a collection library on
  // Suggestions, without the tab: the library opens on its chosen screen.
  function chosenScreen(event) {
    var link = event.target && event.target.closest && event.target.closest('a[href]')
    var href = link ? link.getAttribute('href') : ''
    if (!/^#!?\/mixed\?/i.test(href)) return
    var chosen = href.replace(/([?&])tab=1(&|$)/, function (match, before, after) {
      return after ? before : ''
    })
    if (chosen !== href) link.setAttribute('href', chosen)
  }
  // The last alert shown, and how close a second alert with the same title
  // follows it to be the duplicate one.
  var alerted = null
  var duplicateWithin = 1000
  // closeDuplicate hides and closes a dialog container that repeats, as an
  // alert of the same title opened just before and still open, the alert
  // before it (see the header).
  function closeDuplicate(container) {
    var dialog = container.querySelector && container.querySelector('.dialog')
    var title = dialog && dialog.querySelector('.formDialogHeaderTitle')
    var buttons = dialog ? dialog.querySelectorAll('.btnOption') : []
    if (!title || buttons.length !== 1) return
    var shown = { container: container, title: title.textContent, at: Date.now() }
    var before = alerted
    if (!before || !attached(before.container) || shown.at - before.at > duplicateWithin || before.title !== shown.title) {
      alerted = shown
      return
    }
    container.style.visibility = 'hidden'
    var backdrop = container.previousElementSibling
    if (backdrop && backdrop.classList.contains('dialogBackdrop')) backdrop.style.visibility = 'hidden'
    // Closed once open, as a click on its button closes it, so that
    // jellyfin-web undoes its opening, history entry included; at most 2
    // seconds later all the same.
    var waited = 0
    ;(function close() {
      if (!attached(container)) return
      if (dialog.classList.contains('opened') || waited >= 2000) {
        buttons[0].click()
        return
      }
      waited += 50
      setTimeout(close, 50)
    })()
  }
  // Alerts and item menus are dialog containers jellyfin-web adds to the
  // body, which the script, loaded in the head, observes once it exists.
  function watchDialogs() {
    if (typeof MutationObserver !== 'function' || !document.body) return
    new MutationObserver(
      quietly(function (records) {
        for (var i = 0; i < records.length; i++) {
          var added = records[i].addedNodes || []
          for (var j = 0; j < added.length; j++) {
            if (added[j].classList && added[j].classList.contains('dialogContainer')) {
              // Each on its own: one failing leaves the other to run.
              quietly(closeDuplicate)(added[j])
              quietly(extendMenu)(added[j])
            }
          }
        }
      })
    ).observe(document.body, { childList: true })
  }
  // The title whose scores are asked for, its last answer holding one,
  // the next time they are asked, and the page watched for jellyfin-web's
  // renders of its first line of details.
  var rated = ''
  var scores = null
  var scoresTimer = null
  var scoresWatcher = null
  var scoresWatched = null
  var scoreWords = {
    en: { imdb: 'IMDb rating', critics: 'Critics', criticsTitle: 'Critics’ score', audience: 'Audience', audienceTitle: 'Audience score', percent: '%' },
    fr: { imdb: 'Note IMDb', critics: 'Critiques', criticsTitle: 'Score des critiques', audience: 'Public', audienceTitle: 'Score du public', percent: '\u00a0%' },
    de: { imdb: 'IMDb-Bewertung', critics: 'Kritiker', criticsTitle: 'Kritikerwertung', audience: 'Publikum', audienceTitle: 'Publikumswertung', percent: '\u00a0%' },
    es: { imdb: 'Valoración de IMDb', critics: 'Crítica', criticsTitle: 'Puntuación de la crítica', audience: 'Público', audienceTitle: 'Puntuación del público', percent: '\u00a0%' },
    it: { imdb: 'Voto IMDb', critics: 'Critica', criticsTitle: 'Punteggio della critica', audience: 'Pubblico', audienceTitle: 'Punteggio del pubblico', percent: '%' },
    pt: { imdb: 'Nota do IMDb', critics: 'Crítica', criticsTitle: 'Nota da crítica', audience: 'Público', audienceTitle: 'Nota do público', percent: '%' },
  }
  function sayScore(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (scoreWords[language] || scoreWords.en)[key]
  }
  // followScores asks for the scores of the title the route opens, and
  // gives back the page of the title it leaves.
  function followScores() {
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === rated) return
    rated = id
    scores = null
    clearTimeout(scoresTimer)
    scoresTimer = null
    if (scoresWatcher) scoresWatcher.disconnect()
    scoresWatcher = null
    scoresWatched = null
    unrate()
    if (id) askScores(id, 0)
  }
  // askScores asks for the title's scores: again 3 seconds later while
  // MDBList is asked about it (Pending), 10 times at most, and every second
  // until jellyfin-web made its client.
  function askScores(id, asked) {
    scoresTimer = null
    if (rated !== id || asked >= 10) return
    var again = function (delay) {
      scoresTimer = setTimeout(function () {
        try {
          askScores(id, asked + 1)
        } catch (error) {}
      }, delay)
    }
    var api = window.ApiClient
    if (!api) return again(1000)
    api
      .getJSON(api.getUrl('Polyfin/Items/' + id + '/Ratings'))
      .then(function (answer) {
        if (rated !== id || !answer) return
        if (answer.CommunityRating != null || answer.CriticsScore != null || answer.AudienceScore != null) {
          scores = answer
          showScores()
        }
        if (answer.Pending) again(3000)
      })
      .catch(function () {})
  }
  // unrate gives jellyfin-web's own ratings back, wherever the script hid
  // them.
  function unrate() {
    var lines = document.querySelectorAll('.polyfinRated')
    for (var i = 0; i < lines.length; i++) {
      lines[i].classList.remove('polyfinRated')
      var added = lines[i].querySelector('.polyfinScores')
      if (added) added.parentNode.removeChild(added)
    }
  }
  // showScores puts the title's scores in the first line of details of the
  // title page shown, in place of jellyfin-web's own, and watches the page
  // so that its renders find them again, or the document until the page
  // shows. It never changes the page during a video.
  function showScores() {
    if (!scores) return
    var page = shownPage()
    var target = page || document.body
    if (target && scoresWatched !== target) {
      if (scoresWatcher) scoresWatcher.disconnect()
      scoresWatched = target
      scoresWatcher = new MutationObserver(function () {
        try {
          showScores()
        } catch (error) {}
      })
      scoresWatcher.observe(target, { childList: true, subtree: true })
    }
    if (!page) return
    var line = page.querySelector('.itemMiscInfo-primary')
    if (!line || line.querySelector('.polyfinScores')) return
    scoresStyle()
    var entries = []
    if (scores.CommunityRating != null) entries.push(['IMDb', Number(scores.CommunityRating).toFixed(1), sayScore('imdb')])
    if (scores.CriticsScore != null) {
      entries.push([sayScore('critics'), Math.round(scores.CriticsScore) + sayScore('percent'), sayScore('criticsTitle')])
    }
    if (scores.AudienceScore != null) {
      entries.push([sayScore('audience'), Math.round(scores.AudienceScore) + sayScore('percent'), sayScore('audienceTitle')])
    }
    var group = document.createElement('div')
    group.className = 'mediaInfoItem polyfinScores'
    entries.forEach(function (entry) {
      var score = document.createElement('span')
      score.className = 'polyfinScore'
      score.title = entry[2]
      var label = document.createElement('span')
      label.className = 'polyfinScoreLabel'
      label.textContent = entry[0]
      score.appendChild(label)
      score.appendChild(document.createTextNode(entry[1]))
      group.appendChild(score)
    })
    var own = line.querySelector('.starRatingContainer') || line.querySelector('.mediaInfoCriticRating')
    line.insertBefore(group, own)
    if (!line.classList.contains('polyfinRated')) line.classList.add('polyfinRated')
  }
  // scoresStyle adds the look of the scores, which hides jellyfin-web's own
  // in the same line, once.
  function scoresStyle() {
    if (document.getElementById('polyfinScoresStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinScoresStyle'
    sheet.textContent =
      '.polyfinRated .starRatingContainer,.polyfinRated .mediaInfoCriticRating{display:none!important}' +
      '.polyfinScores{display:inline-flex;flex-wrap:wrap;gap:.3em 1em}' +
      '.polyfinScoreLabel{font-weight:600;opacity:.75;margin-right:.35em}'
    document.head.appendChild(sheet)
  }
  // The item the last menu button pressed, or card right-clicked, opens a
  // menu for (see the header): its identifier, its card, when, and the
  // promise of what Polyfin tells of it; and how soon its menu must open.
  var aimed = null
  var menuWithin = 10000
  // aim follows the presses of the menu buttons of cards and of title
  // pages, and right clicks on cards; any other click forgets the item.
  function aim(event) {
    aimed = null
    var target = event.target
    var api = window.ApiClient
    if (!target || !target.closest || !api) return
    var card = null
    var id = ''
    if (event.type === 'contextmenu') {
      card = target.closest('.card[data-id]')
    } else if (target.closest('.btnMoreCommands')) {
      var route = titleRoute.exec(location.hash)
      id = route ? route[1] : ''
    } else {
      var button = target.closest('button[data-action="menu"]')
      card = button && button.closest('.card[data-id], .listItem[data-id]')
    }
    if (card) id = card.getAttribute('data-id') || ''
    if (!id) return
    aimed = { id: plainId(id), card: card, at: Date.now(), facts: askFacts(api, plainId(id), card) }
  }
  function plainId(id) {
    return String(id).replace(/-/g, '').toLowerCase()
  }
  // askFacts asks for what the entries of the item's menu depend on: its
  // type and series, whether the user hid it or its series, and whether
  // its card stands in a Continue Watching row.
  function askFacts(api, id, card) {
    var user = api.getCurrentUserId()
    var row = card && card.getAttribute('data-positionticks') ? card.closest('.itemsContainer') : null
    var resume = row
      ? api.getJSON(api.getUrl('UserItems/Resume', { userId: user, Limit: 100, MediaTypes: 'Video', EnableImages: false }))
      : null
    return Promise.all([api.getItem(user, id), api.getJSON(api.getUrl('Polyfin/UserItems/Hidden')), resume]).then(
      function (answers) {
        var item = answers[0] || {}
        var series = item.SeriesId ? plainId(item.SeriesId) : ''
        function listed(answer) {
          var ids = {}
          ;((answer && answer.Items) || []).forEach(function (listed) {
            ids[plainId(listed.Id)] = true
          })
          return ids
        }
        var hidden = listed(answers[1])
        var resumable = listed(answers[2])
        var cards = row ? row.querySelectorAll('.card[data-id]') : []
        return {
          series: series,
          hideable: /^(Movie|Video|Series)$/.test(item.Type) || (/^(Season|Episode)$/.test(item.Type) && !!series),
          hidden: !!(hidden[id] || (series && hidden[series])),
          resume:
            cards.length > 0 &&
            Array.prototype.every.call(cards, function (other) {
              return resumable[plainId(other.getAttribute('data-id'))]
            }),
        }
      }
    )
  }
  // extendMenu adds the entries to the item menu a dialog container holds,
  // when it opens for the item aimed at, once Polyfin answered.
  function extendMenu(container) {
    var sheet = container.querySelector && container.querySelector('.actionSheet')
    var entry = aimed
    if (!sheet || !entry || Date.now() - entry.at > menuWithin) return
    aimed = null
    entry.facts
      .then(function (facts) {
        var scroller = sheet.querySelector('.actionSheetScroller')
        var model = scroller && scroller.querySelector('.actionSheetMenuItem')
        if (!attached(container) || !model) return
        if (facts.resume) {
          scroller.appendChild(
            menuItem(container, model, 'playlist_remove', say('removeResume'), function () {
              act(entry.id, 'DELETE', 'Resume', function (data) {
                // The answer is what the user did with the item afterwards:
                // its last play, before the resume point went.
                keepDrop({ kind: 'resume', id: entry.id, last: Date.parse((data && data.LastPlayedDate) || '') || 0 })
                dropCards(entry.card ? [entry.card] : [])
              })
            })
          )
        }
        if (facts.hideable && facts.hidden) {
          scroller.appendChild(
            menuItem(container, model, 'visibility', say('show'), function () {
              act(entry.id, 'DELETE', 'Hidden', function () {
                forgetHidden([entry.id, facts.series])
                toast(say('shown'))
              })
            })
          )
        } else if (facts.hideable) {
          scroller.appendChild(
            menuItem(container, model, 'visibility_off', say('hide'), function () {
              act(entry.id, 'POST', 'Hidden', function () {
                var titles = [entry.id, facts.series].filter(Boolean)
                titles.forEach(function (title) {
                  keepDrop({ kind: 'hidden', id: title })
                })
                var cards = Array.prototype.filter.call(document.querySelectorAll('.card[data-id]'), function (card) {
                  return titles.indexOf(plainId(card.getAttribute('data-id'))) >= 0
                })
                if (entry.card && cards.indexOf(entry.card) < 0) cards.push(entry.card)
                dropCards(cards)
                toast(say('hidden'))
              })
            })
          )
        }
        // The menu was placed for its own entries: it moves up as far as
        // the added ones need.
        var box = sheet.getBoundingClientRect()
        if (sheet.style.top && box.bottom > window.innerHeight) {
          sheet.style.top = Math.max(0, window.innerHeight - box.height) + 'px'
        }
      })
      .catch(function () {})
  }
  // menuItem makes an entry as a copy of one of the menu's own, with its
  // own icon and words, which closes the menu, as Back does, then runs.
  function menuItem(container, model, icon, text, run) {
    var item = model.cloneNode(true)
    item.setAttribute('data-id', 'polyfin')
    item.classList.add('polyfinMenuItem')
    var mark = item.querySelector('.actionsheetMenuItemIcon')
    if (mark) mark.className = mark.className.replace(/\S+$/, icon)
    var words = item.querySelector('.actionSheetItemText')
    if (words) words.textContent = text
    var aside = item.querySelector('.actionSheetItemAsideText')
    if (aside) aside.remove()
    item.addEventListener(
      'click',
      quietly(function (event) {
        // Before the menu hears the click, which would ask jellyfin-web to
        // run a command it does not know.
        event.stopPropagation()
        event.preventDefault()
        var dialog = container.querySelector('.dialog')
        if (dialog && dialog.getAttribute('data-history') === 'true') history.back()
        else container.remove()
        run()
      }),
      true
    )
    return item
  }
  // act sends a change of the user's lists, then runs done with the
  // answer: what the user did with the item, for Resume.
  function act(id, method, path, done) {
    var api = window.ApiClient
    var request = { type: method, url: api.getUrl('Polyfin/UserItems/' + id + '/' + path) }
    if (path === 'Resume') request.dataType = 'json'
    api.ajax(request).then(quietly(done), function () {})
  }
  // What the entries took off (see the header), kept in the browser's
  // session storage under dropsKey for dropsKept: jellyfin-web keeps its
  // answers for a minute. Each names the server and user it is for, and
  // the item: taken off Continue Watching (resume), with its last play
  // then, or hidden (hidden), a series standing for what is in it.
  var dropsKey = 'polyfinDrops'
  var dropsKept = 120000
  var drops = []
  try {
    var savedDrops = JSON.parse(window.sessionStorage.getItem(dropsKey))
    if (Array.isArray(savedDrops)) drops = savedDrops
  } catch (error) {}
  // Those kept too long are forgotten as soon as the page loads.
  if (drops.length) saveDrops()
  function viewerKey() {
    var api = window.ApiClient
    if (!api) return ''
    return (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + (api.getCurrentUserId() || '')
  }
  // liveDrops forgets the drops kept for longer than dropsKept.
  function liveDrops() {
    var now = Date.now()
    drops = drops.filter(function (drop) {
      return !!drop && drop.until > now
    })
    return drops
  }
  function saveDrops() {
    try {
      if (liveDrops().length) window.sessionStorage.setItem(dropsKey, JSON.stringify(drops))
      else window.sessionStorage.removeItem(dropsKey)
    } catch (error) {}
  }
  function keepDrop(drop) {
    drop.viewer = viewerKey()
    drop.until = Date.now() + dropsKept
    drops.push(drop)
    saveDrops()
  }
  function forgetHidden(ids) {
    drops = drops.filter(function (drop) {
      return !(drop && drop.kind === 'hidden' && ids.indexOf(drop.id) >= 0)
    })
    saveDrops()
  }
  // screened is a row's answer without what the drops of the user signed
  // in take off; the answer itself is left as it is.
  function screened(answer) {
    var viewer = viewerKey()
    var mine = liveDrops().filter(function (drop) {
      return drop.viewer === viewer
    })
    var items = Array.isArray(answer) ? answer : answer && Array.isArray(answer.Items) ? answer.Items : null
    if (!mine.length || !items) return answer
    var resumes =
      items.length > 0 &&
      items.every(function (item) {
        return !!item && !!item.UserData && item.UserData.PlaybackPositionTicks > 0
      })
    var kept = items.filter(function (item) {
      return (
        !item ||
        !mine.some(function (drop) {
          if (drop.kind === 'hidden') {
            return plainId(item.Id) === drop.id || (!!item.SeriesId && plainId(item.SeriesId) === drop.id)
          }
          var played = Date.parse((item.UserData && item.UserData.LastPlayedDate) || '') || 0
          return resumes && plainId(item.Id) === drop.id && played <= drop.last
        })
      )
    })
    if (kept.length === items.length) return answer
    return Array.isArray(answer) ? kept : Object.assign({}, answer, { Items: kept })
  }
  // Every row's fetchData goes through screened, as jellyfin-web sets it on
  // the row: rows are divs, and the script runs before jellyfin-web.
  if (typeof HTMLDivElement === 'function') {
    var fetchers = marks('map', 'Fetcher')
    Object.defineProperty(HTMLDivElement.prototype, 'fetchData', {
      configurable: true,
      get: function () {
        return fetchers.get(this)
      },
      set: function (fetch) {
        if (typeof fetch !== 'function') return fetchers.set(this, fetch)
        fetchers.set(this, function () {
          var answer = fetch.apply(this, arguments)
          if (!answer || typeof answer.then !== 'function') return answer
          return answer.then(function (got) {
            try {
              return screened(got)
            } catch (error) {
              return got
            }
          })
        })
      },
    })
  }
  // dropCards takes cards off the page, and hides a home row left empty.
  function dropCards(cards) {
    cards.forEach(function (card) {
      var row = card.closest('.itemsContainer')
      card.remove()
      var section = row && row.closest('.verticalSection')
      if (section && !row.querySelector('.card')) section.classList.add('hide')
    })
  }
  // toast says text as jellyfin-web's toasts do, for 3.3 seconds.
  function toast(text) {
    var holder = document.querySelector('.toastContainer')
    if (!holder) {
      holder = element('div', 'toastContainer', '')
      document.body.appendChild(holder)
    }
    var note = element('div', 'toast', text)
    holder.appendChild(note)
    setTimeout(function () {
      note.classList.add('toastVisible')
    }, 50)
    setTimeout(function () {
      note.classList.add('toastHide')
      setTimeout(function () {
        note.remove()
      }, 300)
    }, 3300)
  }
  // The title whose Share is asked about (see the header), the address its
  // button opens once Polyfin says it may be shared, the next ask while
  // jellyfin-web has no client, and the page watched for jellyfin-web's
  // renders of its buttons.
  var shareTitle = ''
  var shareAddress = ''
  var shareTimer = null
  var shareWatcher = null
  var shareWatched = null
  var shareWords = { en: 'Share', fr: 'Partager', de: 'Teilen', es: 'Compartir', it: 'Condividi', pt: 'Compartilhar' }
  // followShare asks whether the title the route opens may be shared, and
  // takes the button off the page of the title it leaves.
  function followShare() {
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === shareTitle) return
    shareTitle = id
    shareAddress = ''
    clearTimeout(shareTimer)
    shareTimer = null
    if (shareWatcher) shareWatcher.disconnect()
    shareWatcher = null
    shareWatched = null
    var added = document.querySelectorAll('.polyfinShare')
    for (var i = 0; i < added.length; i++) added[i].parentNode.removeChild(added[i])
    if (id) askShare(id, 1)
  }
  // askShare asks Polyfin whether the title may be shared: every second
  // until jellyfin-web made its client, 10 times at most.
  function askShare(id, asked) {
    shareTimer = null
    if (shareTitle !== id) return
    var api = window.ApiClient
    if (!api) {
      if (asked < 10) {
        shareTimer = setTimeout(function () {
          try {
            askShare(id, asked + 1)
          } catch (error) {}
        }, 1000)
      }
      return
    }
    api
      .getJSON(api.getUrl('Polyfin/Items/' + id + '/Share'))
      .then(function (answer) {
        if (shareTitle !== id || !answer || answer.Shareable !== true || typeof answer.Url !== 'string') return
        shareAddress = fromWebClient(resolved(answer.Url))
        showShare()
      })
      .catch(function () {})
  }
  // showShare puts the button among the buttons of the title page shown,
  // and watches the page so that its renders find it again, or the
  // document until the page shows. It never changes the page during a
  // video.
  function showShare() {
    if (!shareAddress) return
    var page = shownPage()
    var target = page || document.body
    if (target && shareWatched !== target) {
      if (shareWatcher) shareWatcher.disconnect()
      shareWatched = target
      shareWatcher = new MutationObserver(function () {
        try {
          showShare()
        } catch (error) {}
      })
      shareWatcher.observe(target, { childList: true, subtree: true })
    }
    if (!page) return
    var buttons = page.querySelector('.mainDetailButtons')
    if (!buttons || buttons.querySelector('.polyfinShare')) return
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    var label = shareWords[language] || shareWords.en
    var icon = element('span', 'material-icons detailButton-icon share', '')
    icon.setAttribute('aria-hidden', 'true')
    var button = element('button', 'button-flat detailButton emby-button polyfinShare', [
      element('div', 'detailButton-content', [icon]),
    ])
    button.setAttribute('type', 'button')
    button.setAttribute('aria-label', label)
    button.title = label
    var address = shareAddress
    button.addEventListener('click', function () {
      location.href = address
    })
    var more = buttons.querySelector('.btnMoreCommands')
    buttons.insertBefore(button, more && more.parentNode === buttons ? more : null)
  }
  // The router moves through history.pushState and replaceState, which
  // fire no event; links and typed addresses fire hashchange.
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      follow()
      return result
    }
  })
  addEventListener('hashchange', follow)
  addEventListener('popstate', follow)
  // Both run before jellyfin-web's own listeners on the version menu.
  addEventListener('change', quietly(pick), true)
  addEventListener('blur', quietly(leave), true)
  // Both run before jellyfin-web follows the link.
  addEventListener('mousedown', quietly(chosenScreen), true)
  addEventListener('click', quietly(chosenScreen), true)
  // Both run before jellyfin-web opens its item menu.
  addEventListener('click', quietly(aim), true)
  addEventListener('contextmenu', quietly(aim), true)
  if (document.body) {
    watchDialogs()
    watchMenus()
  } else {
    addEventListener('DOMContentLoaded', watchDialogs)
    addEventListener('DOMContentLoaded', watchMenus)
  }
  follow()
})()
;(function () {
  var homeRoute = /^#!?\/home(?:\.html)?(?:[?#]|$)/i
  // jellyfin-web's home sections by default, in order.
  var defaultSections = [
    'smalllibrarytiles',
    'resume',
    'resumeaudio',
    'resumebook',
    'livetv',
    'nextup',
    'latestmedia',
    'none',
    'none',
    'none',
  ]
  // The places of rows, from the top: the section each follows, Top
  // following none.
  var placeSections = { Top: '', AfterResume: 'resume', AfterNextUp: 'nextup' }
  var placeOrder = ['Top', 'AfterResume', 'AfterNextUp']
  // The home page's container of sections the rows were last asked for,
  // the answer for it, as {user, container, rows, sections}, and the
  // container being asked for.
  var shown = null
  var answer = null
  var asking = null
  // homeContainer returns the container of sections of the home page
  // shown, or null.
  function homeContainer() {
    if (!homeRoute.test(location.hash)) return null
    var containers = document.querySelectorAll('.homeSectionsContainer')
    for (var i = 0; i < containers.length; i++) {
      var page = containers[i].closest('.page')
      if (!page || !page.classList.contains('hide')) return containers[i]
    }
    return null
  }
  // check asks for the rows each time the home page shows, and draws them
  // again when jellyfin-web emptied the page.
  function check() {
    var container = homeContainer()
    if (!container) {
      shown = null
      return
    }
    if (container !== shown) {
      shown = container
      ask(container)
      return
    }
    if (answer && answer.container === container && !drawn(container)) draw(container)
  }
  // ask asks for the rows and the user's home sections, and draws them
  // once both answered, unless the home page was left meanwhile.
  function ask(container) {
    var api = window.ApiClient
    var user = api && api.getCurrentUserId()
    if (!user) {
      shown = null
      return
    }
    asking = container
    var rows = api.getJSON(api.getUrl('Polyfin/HomeRows', { Fields: 'PrimaryImageAspectRatio' }))
    var preferences = api.getDisplayPreferences('usersettings', user, 'emby')
    Promise.all([rows, preferences])
      .then(function (answers) {
        if (asking !== container || shown !== container || api.getCurrentUserId() !== user) return
        asking = null
        answer = {
          user: user,
          container: container,
          rows: (answers[0] && answers[0].Items) || [],
          sections: sectionsOf((answers[1] && answers[1].CustomPrefs) || {}),
        }
        draw(container)
      })
      .catch(function () {})
  }
  // sectionsOf lists the user's home sections, in order, as jellyfin-web
  // reads them.
  function sectionsOf(preferences) {
    var sections = []
    for (var i = 0; i < 10; i++) {
      var section = preferences['homesection' + i] || defaultSections[i]
      if (section === 'folders') section = defaultSections[0]
      sections.push(section)
    }
    var tv = document.documentElement.classList.contains('layout-tv')
    if (tv && sections.indexOf('smalllibrarytiles') < 0 && sections.indexOf('librarybuttons') < 0) {
      sections.unshift('smalllibrarytiles')
    }
    return sections
  }
  // shownRows returns the rows of the answer without the titles the item
  // menus hid meanwhile (see the tenth): those the tenth keeps in the
  // session storage under polyfinDrops for the user signed in, until they
  // expire, a hidden series taking its items with it. A row left without
  // a title is left out. Storage that fails, or holds anything else, hides
  // nothing.
  function shownRows() {
    var api = window.ApiClient
    var viewer = (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + (api.getCurrentUserId() || '')
    var hidden = {}
    try {
      var drops = JSON.parse(window.sessionStorage.getItem('polyfinDrops'))
      if (Array.isArray(drops)) {
        drops.forEach(function (drop) {
          if (drop && drop.kind === 'hidden' && drop.viewer === viewer && drop.until > Date.now()) hidden[drop.id] = true
        })
      }
    } catch (error) {}
    var plain = function (id) {
      return String(id || '')
        .replace(/-/g, '')
        .toLowerCase()
    }
    return answer.rows
      .map(function (row) {
        var items = (row.Items || []).filter(function (item) {
          return !hidden[plain(item.Id)] && !(item.SeriesId && hidden[plain(item.SeriesId)])
        })
        return Object.assign({}, row, { Items: items })
      })
      .filter(function (row) {
        return row.Items.length > 0
      })
  }
  // drawn tells whether the container holds every row shown, each with all
  // its titles. Rows are found by their identifiers: draw puts them in the
  // order of their places, which may not be the order of the answer.
  function drawn(container) {
    var rows = shownRows()
    var drawnRows = {}
    var count = 0
    for (var i = 0; i < container.children.length; i++) {
      var child = container.children[i]
      if (child.classList.contains('polyfinHomeRow')) {
        drawnRows[child.getAttribute('data-polyfin-row')] = child
        count++
      }
    }
    return (
      count === rows.length &&
      rows.every(function (row) {
        var element = drawnRows[row.Id]
        return !!element && element.querySelectorAll('.card').length === row.Items.length
      })
    )
  }
  // draw puts the rows of the answer at their places in the container,
  // in place of those drawn before.
  function draw(container) {
    if (!container.querySelector('.section0')) return
    var old = container.querySelectorAll('.polyfinHomeRow')
    for (var i = 0; i < old.length; i++) old[i].parentNode.removeChild(old[i])
    var rows = shownRows()
    // The element the next row goes after, null for the top.
    var after = null
    placeOrder.forEach(function (place) {
      var index = placeSections[place] ? answer.sections.indexOf(placeSections[place]) : -1
      var section = index >= 0 ? container.querySelector('.section' + index) : null
      if (section && section.parentNode === container) after = section
      rows.forEach(function (row) {
        if (row.Place !== place) return
        var made = rowElement(row)
        container.insertBefore(made, after ? after.nextSibling : container.firstChild)
        after = made
      })
    })
    rows.forEach(function (row) {
      if (placeOrder.indexOf(row.Place) < 0) container.appendChild(rowElement(row))
    })
  }
  // made makes an element with its class, attributes and children.
  function made(tag, className, attributes, children) {
    var element = document.createElement(tag)
    if (className) element.className = className
    Object.keys(attributes || {}).forEach(function (name) {
      element.setAttribute(name, attributes[name])
    })
    ;(children || []).forEach(function (child) {
      element.appendChild(typeof child === 'string' ? document.createTextNode(child) : child)
    })
    return element
  }
  function icon(name, className) {
    return made('span', 'material-icons ' + (className ? className + ' ' : '') + name, { 'aria-hidden': 'true' })
  }
  // rowElement draws a row as jellyfin-web draws a library's row of
  // latest items.
  function rowElement(row) {
    var api = window.ApiClient
    var server = typeof api.serverId === 'function' ? api.serverId() : ''
    var title = made('div', 'sectionTitleContainer sectionTitleContainer-cards padded-left', {}, [
      made(
        'a',
        'more button-flat button-flat-mini sectionTitleTextButton emby-button',
        { is: 'emby-linkbutton', href: '#/list?parentId=' + encodeURIComponent(row.Id) + '&serverId=' + encodeURIComponent(server) },
        [made('h2', 'sectionTitle sectionTitle-cards', {}, [row.Name]), icon('chevron_right')]
      ),
    ])
    var cards = (row.Items || []).map(function (item, index) {
      return card(api, server, item, index)
    })
    var items = made('div', 'itemsContainer scrollSlider focuscontainer-x animatedScrollX', { is: 'emby-itemscontainer' }, cards)
    var scroller = made(
      'div',
      'padded-top-focusscale padded-bottom-focusscale emby-scroller',
      { is: 'emby-scroller', 'data-centerfocus': 'true' },
      [items]
    )
    return made('div', 'verticalSection emby-scroller-container polyfinHomeRow', { 'data-polyfin-row': row.Id }, [title, scroller])
  }
  // card draws a title as jellyfin-web's cards of a row of posters.
  function card(api, server, item, index) {
    var link = '#/details?id=' + encodeURIComponent(item.Id) + '&serverId=' + encodeURIComponent(server)
    var data = item.UserData || {}
    var tag = item.ImageTags && item.ImageTags.Primary
    var image = made('a', 'cardImageContainer coveredImage cardContent itemAction', {
      href: link,
      'data-action': 'link',
      'aria-label': item.Name || '',
      role: 'img',
    })
    if (tag) {
      var url = api.getScaledImageUrl(item.Id, { type: 'Primary', fillHeight: 407, fillWidth: 272, quality: 96, tag: tag })
      image.style.backgroundImage = 'url("' + url + '")'
    } else {
      image.appendChild(icon(item.Type === 'Series' ? 'tv' : item.Type === 'BoxSet' ? 'folder' : 'movie', 'cardImageIcon'))
    }
    if (data.UnplayedItemCount) {
      image.appendChild(
        made('div', 'cardIndicators', {}, [
          made('div', 'countIndicator indicator', {}, [
            String(Math.min(data.UnplayedItemCount, 99)) + (data.UnplayedItemCount > 99 ? '+' : ''),
          ]),
        ])
      )
    } else if (data.Played) {
      image.appendChild(made('div', 'cardIndicators', {}, [made('div', 'playedIndicator indicator', {}, [icon('check', 'indicatorIcon')])]))
    }
    if (!data.Played && data.PlayedPercentage > 0 && data.PlayedPercentage < 100) {
      var bar = made('div', 'itemProgressBarForeground')
      bar.style.width = data.PlayedPercentage + '%'
      image.appendChild(
        made('div', 'innerCardFooter fullInnerCardFooter innerCardFooterClear', {}, [made('div', 'itemProgressBar', {}, [bar])])
      )
    }
    var overlay = made('div', 'cardOverlayContainer itemAction', { 'data-action': 'link' }, [
      made(
        'button',
        'cardOverlayButton cardOverlayButton-hover itemAction paper-icon-button-light cardOverlayFab-primary',
        {
          is: 'paper-icon-button-light',
          type: 'button',
          'data-action': 'resume',
        },
        [icon('play_arrow', 'cardOverlayButtonIcon cardOverlayButtonIcon-hover')]
      ),
      made('div', 'cardOverlayButton-br flex', {}, [
        made(
          'button',
          'cardOverlayButton cardOverlayButton-hover itemAction paper-icon-button-light',
          {
            is: 'paper-icon-button-light',
            type: 'button',
            'data-action': 'menu',
          },
          [icon('more_vert', 'cardOverlayButtonIcon cardOverlayButtonIcon-hover')]
        ),
      ]),
    ])
    var name = made('div', 'cardText cardTextCentered cardText-first', {}, [
      made('bdi', '', {}, [made('a', 'itemAction textActionButton', { href: link, 'data-action': 'link' }, [item.Name || ''])]),
    ])
    var year = made('div', 'cardText cardTextCentered cardText-secondary', {}, [
      made('bdi', '', {}, [item.ProductionYear ? String(item.ProductionYear) : '\u00a0']),
    ])
    return made(
      'div',
      'card overflowPortraitCard card-hoverable card-withuserdata',
      {
        'data-index': String(index),
        'data-isfolder': item.IsFolder ? 'true' : 'false',
        'data-serverid': server,
        'data-id': item.Id,
        'data-type': item.Type || '',
        'data-mediatype': item.MediaType || '',
        'data-context': 'home',
      },
      [
        made('div', 'cardBox cardBox-bottompadded', {}, [
          made('div', 'cardScalable', {}, [made('div', 'cardPadder cardPadder-overflowPortrait'), image, overlay]),
          name,
          year,
        ]),
      ]
    )
  }
  // watch checks the page as jellyfin-web changes it, the route included.
  function watch() {
    function checked() {
      try {
        check()
      } catch (error) {}
    }
    new MutationObserver(checked).observe(document.body, { childList: true, subtree: true })
    addEventListener('hashchange', checked)
    addEventListener('popstate', checked)
    checked()
  }
  if (document.body) watch()
  else addEventListener('DOMContentLoaded', watch)
})()

// Polyfin's programme reminders (see the header's thirteenth), on their own:
// they follow the routes themselves.
;(function () {
  var route = /^#!?\/details\?(?:[^#]*&)?id=([0-9a-f]{32})(?:[&#]|$)/i
  var words = {
    en: {
      remind: 'Remind me',
      set: 'Reminder set',
      every: 'Remind me of every airing',
      more: 'More reminders',
      remove: 'Remove the reminder',
      everyAiring: 'Reminds you of every airing at about ',
      noTarget: 'No target of yours receives reminders yet: choose Programme starting for one in ',
      notifications: 'My account › Notifications',
      added: 'Reminder set: you will be told before it starts.',
      removed: 'Reminder removed.',
    },
    fr: {
      remind: 'Me le rappeler',
      set: 'Rappel prévu',
      every: 'Me rappeler chaque diffusion',
      more: 'Autres rappels',
      remove: 'Supprimer le rappel',
      everyAiring: 'Vous rappelle chaque diffusion vers ',
      noTarget: 'Aucune de vos cibles ne reçoit encore les rappels : choisissez « Début d’une émission » pour l’une d’elles dans ',
      notifications: 'Mon compte › Notifications',
      added: 'Rappel prévu : vous serez prévenu avant le début.',
      removed: 'Rappel supprimé.',
    },
    de: {
      remind: 'Erinnern',
      set: 'Erinnerung gesetzt',
      every: 'An jede Ausstrahlung erinnern',
      more: 'Weitere Erinnerungen',
      remove: 'Erinnerung entfernen',
      everyAiring: 'Erinnert dich an jede Ausstrahlung gegen ',
      noTarget: 'Noch keines deiner Ziele erhält Erinnerungen: Wähle „Sendung beginnt“ für eines davon unter ',
      notifications: 'Mein Konto › Benachrichtigungen',
      added: 'Erinnerung gesetzt: Du wirst vor dem Beginn benachrichtigt.',
      removed: 'Erinnerung entfernt.',
    },
    es: {
      remind: 'Recordármelo',
      set: 'Recordatorio programado',
      every: 'Recordarme cada emisión',
      more: 'Más recordatorios',
      remove: 'Quitar el recordatorio',
      everyAiring: 'Le recuerda cada emisión hacia las ',
      noTarget: 'Ninguno de sus destinos recibe todavía recordatorios: elija «Programa a punto de empezar» para uno de ellos en ',
      notifications: 'Mi cuenta › Notificaciones',
      added: 'Recordatorio programado: se le avisará antes de que empiece.',
      removed: 'Recordatorio quitado.',
    },
    it: {
      remind: 'Ricordamelo',
      set: 'Promemoria impostato',
      every: 'Ricordami ogni messa in onda',
      more: 'Altri promemoria',
      remove: 'Rimuovi il promemoria',
      everyAiring: 'Ti ricorda ogni messa in onda verso le ',
      noTarget: 'Nessuna delle tue destinazioni riceve ancora i promemoria: scegli «Programma in partenza» per una di esse in ',
      notifications: 'Il mio account › Notifiche',
      added: 'Promemoria impostato: riceverai un avviso prima dell’inizio.',
      removed: 'Promemoria rimosso.',
    },
    pt: {
      remind: 'Lembrar-me',
      set: 'Lembrete definido',
      every: 'Lembrar-me de cada exibição',
      more: 'Mais lembretes',
      remove: 'Remover o lembrete',
      everyAiring: 'Lembra você de cada exibição por volta das ',
      noTarget: 'Nenhum dos seus destinos recebe lembretes ainda: escolha “Programa começando” para um deles em ',
      notifications: 'Minha conta › Notificações',
      added: 'Lembrete definido: você será avisado antes do início.',
      removed: 'Lembrete removido.',
    },
  }
  function language() {
    return /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
  }
  function say(key) {
    return (words[language()] || words.en)[key]
  }
  // The programme whose page is followed, how it stands as Polyfin last
  // answered, whether it was asked, whether a change is under way, the
  // buttons drawn and the answer they were drawn for, and the observer of
  // the document while the page is followed.
  var program = ''
  var state = null
  var asked = false
  var busy = false
  var drawn = null
  var drawnFor = null
  var watcher = null
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // shownPage returns the title page shown, unless a video is.
  function shownPage() {
    if (document.querySelector('.videoPlayerContainer')) return null
    return document.querySelector('.page.itemDetailPage:not(.hide)')
  }
  // follow starts following the page of the programme the route opens,
  // or stops, taking the buttons off.
  function follow() {
    var match = route.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === program) return
    program = id
    state = null
    asked = false
    busy = false
    undraw()
    if (!id) {
      if (watcher) watcher.disconnect()
      watcher = null
      return
    }
    // The script runs in the page's head: a page opened on a programme's
    // address has no body yet, which the observer waits for.
    if (document.body) watch()
    else addEventListener('DOMContentLoaded', quietly(watch))
  }
  // watch observes the document while a programme's page is followed, and
  // looks at the page.
  function watch() {
    if (!program || !document.body) return
    if (!watcher) {
      watcher = new MutationObserver(quietly(place))
      watcher.observe(document.body, { childList: true, subtree: true })
    }
    place()
  }
  // undraw takes the buttons off.
  function undraw() {
    if (drawn && drawn.parentNode) drawn.parentNode.removeChild(drawn)
    drawn = null
    drawnFor = null
  }
  // place asks Polyfin how the programme stands once jellyfin-web drew
  // the page's recording fields, then puts the buttons after them, again
  // whenever jellyfin-web renders the page anew.
  function place() {
    if (!program) return
    var page = shownPage()
    var fields = page && page.querySelector('.recordingFields')
    if (!fields) return
    if (!asked) return ask(program)
    if (!state || state.Started) return undraw()
    if (drawn && drawnFor === state && drawn.parentNode === fields.parentNode && drawn.previousSibling === fields) return
    undraw()
    style()
    drawn = draw()
    drawnFor = state
    fields.parentNode.insertBefore(drawn, fields.nextSibling)
  }
  // ask asks Polyfin how the programme id stands: an item that is not a
  // programme of the user's guide, or a user without Live TV, gets an
  // empty answer, which getJSON fails to read, and no button.
  function ask(id) {
    var api = window.ApiClient
    if (!api) return
    asked = true
    api.getJSON(api.getUrl('Polyfin/Reminders/Programs/' + id)).then(
      quietly(function (answer) {
        if (program !== id || !answer || String(answer.ProgramId || '').toLowerCase() !== id) return
        state = answer
        place()
      }),
      function () {}
    )
  }
  // button makes a button styled as jellyfin-web's record buttons.
  function button(className, icon, text) {
    var made = document.createElement('button')
    made.type = 'button'
    made.className = 'raised emby-button ' + className
    var mark = document.createElement('span')
    mark.className = 'material-icons polyfinReminderIcon ' + icon
    mark.setAttribute('aria-hidden', 'true')
    made.appendChild(mark)
    if (text) {
      var label = document.createElement('span')
      label.textContent = text
      made.appendChild(label)
    }
    return made
  }
  // draw makes the buttons for the answer: Remind me, or Reminder set,
  // and the menu button, unless a series reminder made the reminder; then
  // the lines that tell of a series reminder, and that no target receives
  // reminders.
  function draw() {
    var block = document.createElement('div')
    block.className = 'polyfinReminders'
    var row = document.createElement('div')
    row.className = 'polyfinReminderRow'
    block.appendChild(row)
    var set = !!state.ReminderId
    var main = button('polyfinRemind' + (set ? ' polyfinReminderSet' : ''), set ? 'notifications_active' : 'notifications_none', say(set ? 'set' : 'remind'))
    if (set) main.title = say('remove')
    main.disabled = busy
    main.addEventListener('click', quietly(set ? remove : remind(false)))
    row.appendChild(main)
    if (!state.SeriesReminderId) {
      var more = button('polyfinReminderMore', 'expand_more', '')
      more.title = say('more')
      more.setAttribute('aria-label', say('more'))
      more.setAttribute('aria-haspopup', 'menu')
      more.setAttribute('aria-expanded', 'false')
      more.disabled = busy
      var menu = document.createElement('div')
      menu.className = 'polyfinReminderMenu hide'
      menu.setAttribute('role', 'menu')
      var every = document.createElement('button')
      every.type = 'button'
      every.className = 'polyfinReminderEvery'
      every.setAttribute('role', 'menuitem')
      every.textContent = say('every')
      every.addEventListener('click', quietly(remind(true)))
      menu.appendChild(every)
      more.addEventListener(
        'click',
        quietly(function () {
          var open = menu.classList.contains('hide')
          menu.classList.toggle('hide', !open)
          more.setAttribute('aria-expanded', open ? 'true' : 'false')
          if (open) every.focus()
        })
      )
      row.appendChild(more)
      row.appendChild(menu)
    }
    if (state.SeriesReminderId && state.SeriesStartDate) {
      var time = new Date(state.SeriesStartDate).toLocaleTimeString(document.documentElement.lang || undefined, {
        hour: '2-digit',
        minute: '2-digit',
      })
      block.appendChild(note(say('everyAiring') + time + '.'))
    }
    if (!state.Reaches) {
      var hint = note(say('noTarget'))
      var link = document.createElement('a')
      link.className = 'polyfinReminderLink'
      // Marked as opened from the web client, as the script's other links
      // to the admin app are (see fromWebClient). The server names the page
      // from its root, /admin/…: the web client's folder is one below it,
      // or below Polyfin's base path. Written by hand, as the script writes
      // its addresses (see adminAddress).
      var page = location.href.split('#')[0].split('?')[0]
      var path = (state.NotificationsUrl || '/admin/me/account#notifications').replace(/^\/+/, '')
      var at = path.indexOf('#')
      var rest = at < 0 ? path : path.slice(0, at)
      var root = /^[a-z][a-z0-9+.-]*:\/\/[^/]*\//i.exec(page)
      var above = page.slice(0, page.lastIndexOf('/') + 1).replace(/[^/]+\/$/, '')
      link.href =
        (root && above.length < root[0].length ? root[0] : above) +
        rest +
        (rest.indexOf('?') < 0 ? '?' : '&') +
        'from=web' +
        (at < 0 ? '' : path.slice(at))
      link.textContent = say('notifications')
      hint.appendChild(link)
      hint.appendChild(document.createTextNode('.'))
      block.appendChild(hint)
    }
    return block
  }
  function note(text) {
    var line = document.createElement('div')
    line.className = 'polyfinReminderNote'
    line.textContent = text
    return line
  }
  // remind sets a reminder of the programme, or of every airing.
  function remind(series) {
    return function () {
      send('POST', 'Polyfin/Reminders', { ProgramId: program, Series: series, Language: language() })
    }
  }
  // remove removes the reminder shown: the series reminder that made it,
  // if any.
  function remove() {
    send('DELETE', 'Polyfin/Reminders/' + (state.SeriesReminderId || state.ReminderId))
  }
  // send sends a change, then shows how the programme stands, and says so.
  function send(method, path, body) {
    var api = window.ApiClient
    if (!api || busy) return
    var id = program
    busy = true
    drawnFor = null
    place()
    var request = { type: method, url: api.getUrl(path) }
    if (body) {
      request.data = JSON.stringify(body)
      request.contentType = 'application/json'
      request.dataType = 'json'
    }
    api.ajax(request).then(
      quietly(function (answer) {
        if (program !== id) return
        busy = false
        if (body && answer && answer.ProgramId) state = answer
        else state = Object.assign({}, state, { ReminderId: '', SeriesReminderId: '', SeriesStartDate: null })
        toast(say(body ? 'added' : 'removed'))
        place()
      }),
      quietly(function () {
        if (program !== id) return
        busy = false
        drawnFor = null
        place()
      })
    )
  }
  // toast says text as jellyfin-web's toasts do, for 3.3 seconds.
  function toast(text) {
    var holder = document.querySelector('.toastContainer')
    if (!holder) {
      holder = document.createElement('div')
      holder.className = 'toastContainer'
      document.body.appendChild(holder)
    }
    var shown = document.createElement('div')
    shown.className = 'toast'
    shown.textContent = text
    holder.appendChild(shown)
    setTimeout(function () {
      shown.classList.add('toastVisible')
    }, 50)
    setTimeout(function () {
      shown.classList.add('toastHide')
      setTimeout(function () {
        if (shown.parentNode) shown.parentNode.removeChild(shown)
      }, 300)
    }, 3300)
  }
  // style adds the look of the buttons, the menu and the lines, once.
  function style() {
    if (document.getElementById('polyfinRemindersStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinRemindersStyle'
    sheet.textContent =
      '.polyfinReminders{margin:-.5em 0 1.5em}' +
      '.polyfinReminderRow{position:relative;display:inline-flex;align-items:center;gap:.3em}' +
      '.polyfinRemind{min-width:10em;margin:0}' +
      '.polyfinReminderIcon{margin-right:.4em}' +
      '.polyfinReminderSet .polyfinReminderIcon{color:#00a4dc}' +
      '.polyfinReminderMore{margin:0;padding-left:.5em;padding-right:.5em;min-width:0}' +
      '.polyfinReminderMore .polyfinReminderIcon{margin:0}' +
      '.polyfinReminderMenu{position:absolute;top:100%;left:0;z-index:10;margin-top:.3em;padding:.3em 0;border-radius:.3em;' +
      'background:#202020;box-shadow:0 .2em .8em rgba(0,0,0,.6)}' +
      '.polyfinReminderMenu.hide{display:none}' +
      '.polyfinReminderEvery{display:block;width:100%;padding:.8em 1.2em;border:0;background:none;color:inherit;font:inherit;' +
      'text-align:left;white-space:nowrap;cursor:pointer}' +
      '.polyfinReminderEvery:hover,.polyfinReminderEvery:focus{background:rgba(255,255,255,.1)}' +
      '.polyfinReminderNote{margin-top:.6em;opacity:.8}' +
      '.polyfinReminderLink{color:#00a4dc}'
    document.head.appendChild(sheet)
  }
  // The router moves through history.pushState and replaceState, which
  // fire no event; links and typed addresses fire hashchange.
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      quietly(follow)()
      return result
    }
  })
  addEventListener('hashchange', quietly(follow))
  addEventListener('popstate', quietly(follow))
  // A click outside the menu closes it.
  addEventListener(
    'click',
    quietly(function (event) {
      var menu = drawn && drawn.querySelector('.polyfinReminderMenu')
      if (!menu || menu.classList.contains('hide')) return
      var target = event.target
      if (target && target.closest && target.closest('.polyfinReminderRow')) return
      menu.classList.add('hide')
    }),
    true
  )
  quietly(follow)()
})()

// Dropping jellyfin-web's kept answers when another user signs in (see the
// header's fourteenth), on its own: it follows the routes itself.
;(function () {
  var keptForKey = 'polyfinKeptFor'
  // The user, as server/user, the answers jellyfin-web keeps are for.
  var keptFor = null
  try {
    keptFor = window.localStorage.getItem(keptForKey)
  } catch (error) {}
  function quietly(listener) {
    return function (event) {
      try {
        listener(event)
      } catch (error) {}
    }
  }
  // forgetKept deletes jellyfin-web's copy of its cache in IndexedDB, then
  // empties the cache in memory as a language change does. A browser
  // without the database has nothing kept: the upgrade that would make it
  // is aborted, leaving it to jellyfin-web.
  function forgetKept() {
    try {
      var request = indexedDB.open('keyval-store')
      request.onupgradeneeded = function () {
        request.transaction.abort()
      }
      request.onerror = function (event) {
        event.preventDefault()
      }
      request.onsuccess = function () {
        var database = request.result
        try {
          if (database.objectStoreNames.contains('keyval')) {
            database.transaction('keyval', 'readwrite').objectStore('keyval').delete('jellyfin-query-cache')
          }
        } finally {
          database.close()
        }
      }
    } catch (error) {}
    var listeners = ((document._callbacks || {}).LANGUAGE_CHANGE || []).slice()
    for (var i = 0; i < listeners.length; i++) {
      try {
        listeners[i].apply(document, [{ type: 'LANGUAGE_CHANGE' }, document.documentElement.lang])
      } catch (error) {}
    }
  }
  // signedIn forgets the kept answers when user, signed in, is not the one
  // they were kept for.
  function signedIn(user) {
    if (!user || user === keptFor) return
    keptFor = user
    try {
      window.localStorage.setItem(keptForKey, user)
    } catch (error) {}
    forgetKept()
  }
  // savedUser is the user jellyfin-web's saved credentials sign in on the
  // server it used last, as server/user; "" when none.
  function savedUser() {
    var servers = JSON.parse(window.localStorage.getItem('jellyfin_credentials') || '{}').Servers || []
    var last = null
    for (var i = 0; i < servers.length; i++) {
      var server = servers[i]
      if (!server || !server.AccessToken || !server.UserId) continue
      if (!last || (server.DateLastAccessed || 0) > (last.DateLastAccessed || 0)) last = server
    }
    return last ? last.Id + '/' + last.UserId : ''
  }
  try {
    signedIn(savedUser())
  } catch (error) {}
  // A user signing in in the page goes with a route change: jellyfin-web
  // then leaves its sign-in page.
  function follow() {
    var api = window.ApiClient
    var user = api && api.getCurrentUserId()
    if (user) signedIn((typeof api.serverId === 'function' ? api.serverId() : '') + '/' + user)
  }
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      quietly(follow)()
      return result
    }
  })
  addEventListener('hashchange', quietly(follow))
  addEventListener('popstate', quietly(follow))
  quietly(follow)()
})()

// The user's recent searches on jellyfin-web's search page (see the
// header's fifteenth), on their own: they follow the routes themselves.
;(function () {
  var route = /^#!?\/search(?:[?#]|$)/i
  var recentKey = 'polyfinRecentSearches/'
  var most = 10
  // removeOne names the button that removes the search in {0}.
  var searchWords = {
    en: { title: 'Recent searches', remove: 'Remove', removeOne: 'Remove “{0}”', clear: 'Clear all' },
    fr: { title: 'Recherches récentes', remove: 'Supprimer', removeOne: 'Supprimer « {0} »', clear: 'Tout effacer' },
    de: { title: 'Letzte Suchen', remove: 'Entfernen', removeOne: '„{0}“ entfernen', clear: 'Alle löschen' },
    es: { title: 'Búsquedas recientes', remove: 'Eliminar', removeOne: 'Eliminar «{0}»', clear: 'Borrar todo' },
    it: { title: 'Ricerche recenti', remove: 'Rimuovi', removeOne: 'Rimuovi «{0}»', clear: 'Cancella tutto' },
    pt: { title: 'Buscas recentes', remove: 'Remover', removeOne: 'Remover “{0}”', clear: 'Limpar tudo' },
  }
  function language() {
    return /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
  }
  function say(key) {
    return (searchWords[language()] || searchWords.en)[key]
  }
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // The route's search, trimmed, when last followed, null off the search
  // page; the search shown with results, with the user it is for, as
  // { user, query }; the first card of the results and the line that
  // nothing was found when last looked at; the list drawn and what it was
  // drawn for; and the observer of the document while the page is followed.
  var typed = null
  var shown = null
  var shownCard = null
  var shownNothing = null
  var drawn = null
  var drawnFor = ''
  var watcher = null
  // user is the user signed in, as server/user; "" when none.
  function user() {
    var api = window.ApiClient
    var id = api && api.getCurrentUserId()
    return id ? (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + id : ''
  }
  // params are the parameters of a search route, null for another route.
  function params(hash) {
    if (!route.test(hash)) return null
    var at = hash.indexOf('?')
    return new RouteParams(at < 0 ? '' : hash.slice(at + 1).split('#')[0])
  }
  // RouteParams are a route's parameters, read and written as
  // URLSearchParams does, which browsers before Chrome 49 lack: the script
  // runs before jellyfin-web's polyfills. get gives a parameter's first
  // value, set replaces its first and drops the others, and toString
  // writes them back.
  function RouteParams(text) {
    this.pairs = []
    var parts = text.split('&')
    for (var i = 0; i < parts.length; i++) {
      if (!parts[i]) continue
      var at = parts[i].indexOf('=')
      this.pairs.push([decoded(at < 0 ? parts[i] : parts[i].slice(0, at)), at < 0 ? '' : decoded(parts[i].slice(at + 1))])
    }
  }
  RouteParams.prototype.get = function (name) {
    for (var i = 0; i < this.pairs.length; i++) if (this.pairs[i][0] === name) return this.pairs[i][1]
    return null
  }
  RouteParams.prototype.set = function (name, value) {
    var kept = []
    var placed = false
    for (var i = 0; i < this.pairs.length; i++) {
      if (this.pairs[i][0] !== name) {
        kept.push(this.pairs[i])
      } else if (!placed) {
        kept.push([name, String(value)])
        placed = true
      }
    }
    if (!placed) kept.push([name, String(value)])
    this.pairs = kept
  }
  RouteParams.prototype.toString = function () {
    return this.pairs
      .map(function (pair) {
        return encoded(pair[0]) + '=' + encoded(pair[1])
      })
      .join('&')
  }
  // decoded reads a parameter's name or value, + standing for a space; a
  // malformed escape leaves it as it is.
  function decoded(text) {
    var plain = text.replace(/\+/g, ' ')
    try {
      return decodeURIComponent(plain)
    } catch (error) {
      return plain
    }
  }
  // encoded writes a parameter's name or value as URLSearchParams does
  // (application/x-www-form-urlencoded): a space as +, and every character
  // but letters, digits and *-._ escaped.
  function encoded(text) {
    return encodeURIComponent(text)
      .replace(/[!'()~]/g, function (character) {
        return '%' + character.charCodeAt(0).toString(16).toUpperCase()
      })
      .replace(/%20/g, '+')
  }
  // recent are the searches kept for who, newest first.
  function recent(who) {
    try {
      var list = JSON.parse(window.localStorage.getItem(recentKey + who))
      if (!Array.isArray(list)) return []
      return list
        .filter(function (query) {
          return typeof query === 'string' && query.trim() !== ''
        })
        .slice(0, most)
    } catch (error) {
      return []
    }
  }
  function save(who, list) {
    try {
      if (list.length) window.localStorage.setItem(recentKey + who, JSON.stringify(list))
      else window.localStorage.removeItem(recentKey + who)
    } catch (error) {}
  }
  // same tells whether two searches are the same words, in any capitals.
  function same(a, b) {
    return a.toLowerCase() === b.toLowerCase()
  }
  // without is the list without query.
  function without(list, query) {
    return list.filter(function (kept) {
      return !same(kept, query)
    })
  }
  // leave keeps the search shown, at the top, as the user leaves it.
  function leave() {
    var search = shown
    shown = null
    if (!search) return
    var list = without(recent(search.user), search.query)
    list.unshift(search.query)
    save(search.user, list.slice(0, most))
  }
  // follow follows the search page's field, as the route holds it: it
  // keeps the search shown when the route leaves the page, clears the
  // field, or holds a search that neither starts with it nor is its start.
  function follow() {
    var found = params(location.hash)
    var query = found ? (found.get('query') || '').trim() : null
    if (shown) {
      var a = shown.query.toLowerCase()
      var b = (query || '').toLowerCase()
      if (!b || (a.indexOf(b) !== 0 && b.indexOf(a) !== 0)) leave()
    }
    typed = query
    if (query === null) {
      if (watcher) watcher.disconnect()
      watcher = null
      shownCard = null
      shownNothing = null
      undraw()
      return
    }
    // The script runs in the page's head: a page opened on the search
    // page's address has no body yet, which the observer waits for.
    if (document.body) watch()
    else addEventListener('DOMContentLoaded', quietly(watch))
  }
  // watch observes the document while the search page is followed, and
  // looks at the page.
  function watch() {
    if (typed === null || !document.body) return
    if (!watcher) {
      watcher = new MutationObserver(quietly(look))
      watcher.observe(document.body, { childList: true, subtree: true })
    }
    look()
  }
  function searchPage() {
    var page = document.getElementById('searchPage')
    return page && !page.classList.contains('hide') ? page : null
  }
  // look notes the field's search as shown when jellyfin-web draws its
  // results with a card, or drops it when jellyfin-web says that nothing
  // was found, then lists the recent searches while the field is empty
  // and jellyfin-web shows no result.
  function look() {
    if (typed === null) return
    var page = searchPage()
    if (!page) return undraw()
    var results = page.querySelector('.searchResults')
    var card = results && results.querySelector('.card')
    if (card !== shownCard) {
      shownCard = card
      var who = user()
      if (card && typed && who) shown = { user: who, query: typed }
    }
    var nothing = page.querySelector('.noItemsMessage')
    if (nothing !== shownNothing) {
      shownNothing = nothing
      if (nothing) shown = null
    }
    place(page, !typed && !results && !nothing)
  }
  // undraw takes the list off.
  function undraw() {
    if (drawn && drawn.parentNode) drawn.parentNode.removeChild(drawn)
    drawn = null
    drawnFor = ''
  }
  // place puts the list after the field when it is to show, again when
  // it changes or jellyfin-web draws the page anew.
  function place(page, empty) {
    var who = user()
    var list = empty && who ? recent(who) : []
    var fields = page.querySelector('.searchFields')
    if (!list.length || !fields) return undraw()
    var key = [who, language(), location.hash].concat(list).join('\n')
    if (drawn && drawnFor === key && drawn.parentNode === fields.parentNode && drawn.previousSibling === fields) return
    undraw()
    style()
    drawn = draw(who, list)
    drawnFor = key
    fields.parentNode.insertBefore(drawn, fields.nextSibling)
  }
  function made(tag, className, children) {
    var element = document.createElement(tag)
    element.className = className
    ;(children || []).forEach(function (child) {
      if (typeof child === 'string') element.textContent = child
      else element.appendChild(child)
    })
    return element
  }
  // address is the route that searches query again on the search page,
  // in the page's library, if any.
  function address(query) {
    var found = params(location.hash) || new RouteParams('')
    found.set('query', query)
    return '#/search?' + found.toString()
  }
  // draw makes the list, drawn as jellyfin-web draws the Suggestions:
  // centred, a heading, then a link for each search, with its button that
  // removes it, then the button that removes them all. The TV layout
  // scales the button with the focus, as jellyfin-web's own. A link
  // chosen gives the focus to the field, as the list leaves with it.
  function draw(who, list) {
    var root = document.documentElement
    var focus = root.classList && root.classList.contains('layout-tv') ? ' show-focus' : ''
    var entries = made('div', 'polyfinRecentSearchesList padded-left padded-right')
    list.forEach(function (query) {
      var link = made('a', 'button-link emby-button polyfinRecentSearchLink' + focus, [query])
      link.setAttribute('href', address(query))
      link.addEventListener('click', quietly(focusField))
      var remove = made('button', 'paper-icon-button-light polyfinRecentSearchRemove' + focus, [
        made('span', 'material-icons close'),
      ])
      remove.firstChild.setAttribute('aria-hidden', 'true')
      remove.setAttribute('type', 'button')
      remove.setAttribute(
        'aria-label',
        say('removeOne').replace('{0}', function () {
          return query
        })
      )
      remove.title = say('remove')
      remove.addEventListener(
        'click',
        quietly(function () {
          forget(who, query)
        })
      )
      entries.appendChild(made('div', 'polyfinRecentSearch', [made('span', 'polyfinRecentSearchItem', [link, remove])]))
    })
    var clear = made('button', 'button-link emby-button polyfinRecentSearchesClear' + focus, [say('clear')])
    clear.setAttribute('type', 'button')
    clear.addEventListener(
      'click',
      quietly(function () {
        forget(who, null)
      })
    )
    var heading = made('h2', 'sectionTitle padded-left padded-right', [say('title')])
    return made('div', 'verticalSection polyfinRecentSearches', [made('div', '', [heading]), entries, made('div', '', [clear])])
  }
  // forget removes query from who's searches, or all of them for null,
  // then gives the focus to the next button that removes one, the last
  // one, or the field when none is left.
  function forget(who, query) {
    var list = recent(who)
    var at = query === null ? 0 : list.indexOf(query)
    save(who, query === null ? [] : without(list, query))
    look()
    var removes = drawn ? drawn.querySelectorAll('.polyfinRecentSearchRemove') : []
    if (removes.length) removes[Math.min(Math.max(at, 0), removes.length - 1)].focus()
    else focusField()
  }
  function focusField() {
    var field = document.getElementById('searchTextInput')
    if (field) field.focus()
  }
  // style adds the look of the list, once: the searches centred as the
  // Suggestions, each button that removes one after its search.
  function style() {
    if (document.getElementById('polyfinRecentSearchesStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinRecentSearchesStyle'
    sheet.textContent =
      '.polyfinRecentSearches{text-align:center}' +
      '.polyfinRecentSearchItem{position:relative;display:inline-block}' +
      '.polyfinRecentSearchLink,.polyfinRecentSearchesClear{display:inline-block;padding:.5em 1em}' +
      '.polyfinRecentSearchRemove{position:absolute;top:0;bottom:0;left:100%;margin:0;padding:0 .2em;opacity:.6}' +
      '.polyfinRecentSearchRemove>.material-icons{font-size:1.2em}' +
      '.polyfinRecentSearchRemove:hover,.polyfinRecentSearchRemove:focus{opacity:1}' +
      '.polyfinRecentSearchesClear{margin-top:.3em;opacity:.7}' +
      '.polyfinRecentSearchesClear:hover,.polyfinRecentSearchesClear:focus{opacity:1}'
    document.head.appendChild(sheet)
  }
  // The router moves through history.pushState and replaceState, which
  // fire no event; links and typed addresses fire hashchange.
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      quietly(follow)()
      return result
    }
  })
  addEventListener('hashchange', quietly(follow))
  addEventListener('popstate', quietly(follow))
  // Closing the tab, or loading another page in it, leaves the search too.
  addEventListener('pagehide', quietly(leave))
  quietly(follow)()
})()

// Titles' ratings on their posters (see the header's sixteenth), on their
// own: they follow the pointer and the focus.
;(function () {
  var titleCard = '.card[data-id]'
  var titleTypes = { Movie: true, Series: true }
  // How long an answer is kept, so that turning the ratings on posters on
  // or off applies within a minute, how long a server that shows none, or
  // did not answer, is left unasked, and how many titles one request names.
  var keptFor = 60 * 1000
  var offFor = 60 * 1000
  var most = 100
  // The ratings answered, by identifier, as { at, rating }, rating null for
  // a title without any, for keptUser; until when not to ask; whether a
  // request is under way, and the card last met meanwhile.
  var kept = {}
  var keptUser = ''
  var offUntil = 0
  var asking = false
  var waiting = null
  // The icons, MUI's Material Icons (Apache License 2.0) as jellyfin-web
  // draws its own: a star, a review and a group of people.
  var icons = {
    community: 'M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z',
    critics:
      'M20 2H4c-1.1 0-1.99.9-1.99 2L2 22l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zM6 14v-2.47l6.88-6.88c.2-.2.51-.2.71 0l1.77 1.77c.2.2.2.51 0 .71L8.47 14H6zm12 0h-7.5l2-2H18v2z',
    audience:
      'M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z',
  }
  // The percent sign as each of jellyfin-web's languages writes it after a
  // number, "%" for the others.
  var percents = { fr: '\u00a0%', de: '\u00a0%', es: '\u00a0%' }
  function percent() {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return percents[language] || '%'
  }
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  function plainId(id) {
    return String(id).replace(/-/g, '').toLowerCase()
  }
  // titleOf returns the title card an element is in, null outside one.
  function titleOf(element) {
    var card = element && element.closest ? element.closest(titleCard) : null
    return card && titleTypes[card.getAttribute('data-type')] ? card : null
  }
  function known(id, now) {
    var entry = kept[id]
    return entry && now - entry.at < keptFor ? entry : null
  }
  // arrive follows the pointer and the focus to a title card: it shows the
  // card's ratings when known, and asks for them otherwise.
  function arrive(event) {
    var card = titleOf(event.target)
    var api = window.ApiClient
    if (!card || !api || /^#!?\/video(?:[/?#]|$)/i.test(location.hash)) return
    var user = api.getCurrentUserId() || ''
    if (!user) return
    if (user !== keptUser) {
      kept = {}
      keptUser = user
      offUntil = 0
    }
    var now = Date.now()
    var entry = known(plainId(card.getAttribute('data-id')), now)
    if (entry) return show(card, entry.rating)
    if (now < offUntil) return
    if (asking) {
      waiting = card
      return
    }
    ask(api, user, card, now)
  }
  // ask asks for the ratings of card and of the other title cards of the
  // page not known, those of its row first, 100 at most, then shows them.
  function ask(api, user, card, now) {
    var ids = []
    var row = card.closest('.itemsContainer')
    var cards = [card]
      .concat(row ? Array.prototype.slice.call(row.querySelectorAll(titleCard)) : [])
      .concat(Array.prototype.slice.call(document.querySelectorAll(titleCard)))
    for (var i = 0; i < cards.length && ids.length < most; i++) {
      if (!titleTypes[cards[i].getAttribute('data-type')]) continue
      var id = plainId(cards[i].getAttribute('data-id'))
      if (ids.indexOf(id) < 0 && !known(id, now)) ids.push(id)
    }
    asking = true
    var done = function () {
      asking = false
      var next = waiting
      waiting = null
      if (next) quietly(arrive)({ target: next })
    }
    api
      .getJSON(api.getUrl('Polyfin/Items/Ratings', { Ids: ids.join(',') }))
      .then(
        quietly(function (answer) {
          if (keptUser !== user) return
          var at = Date.now()
          if (!answer || !answer.Enabled) {
            offUntil = at + offFor
            strip()
            return
          }
          ids.forEach(function (id) {
            kept[id] = { at: at, rating: null }
          })
          ;(answer.Items || []).forEach(function (item) {
            if (item && item.Id) kept[plainId(item.Id)] = { at: at, rating: item }
          })
          decorate(at)
        }),
        function () {
          offUntil = Date.now() + offFor
        }
      )
      .then(done, done)
  }
  // decorate gives every title card of the page whose ratings are known
  // its ratings.
  function decorate(now) {
    var cards = document.querySelectorAll(titleCard)
    for (var i = 0; i < cards.length; i++) {
      if (!titleTypes[cards[i].getAttribute('data-type')]) continue
      var entry = known(plainId(cards[i].getAttribute('data-id')), now)
      if (entry) show(cards[i], entry.rating)
    }
  }
  // strip takes every card's ratings away.
  function strip() {
    var shown = document.querySelectorAll('.polyfinPosterRatings')
    for (var i = 0; i < shown.length; i++) if (shown[i].parentNode) shown[i].parentNode.removeChild(shown[i])
  }
  // show puts a title's ratings over its card's poster, once.
  function show(card, rating) {
    var poster = card.querySelector('.cardScalable')
    if (!rating || !poster || poster.querySelector('.polyfinPosterRatings')) return
    var entries = []
    if (rating.CommunityRating != null) entries.push(['community', Number(rating.CommunityRating).toFixed(1)])
    if (rating.CriticsScore != null) entries.push(['critics', Math.round(rating.CriticsScore) + percent()])
    if (rating.AudienceScore != null) entries.push(['audience', Math.round(rating.AudienceScore) + percent()])
    if (!entries.length) return
    style()
    var badge = document.createElement('div')
    badge.className = 'polyfinPosterRatings'
    // What the pointer or the focus already reaches: the card names the
    // title, and its page tells the ratings in words.
    badge.setAttribute('aria-hidden', 'true')
    entries.forEach(function (entry) {
      var score = document.createElement('span')
      score.className = 'polyfinPosterRating polyfinPosterRating-' + entry[0]
      var icon = document.createElement('span')
      icon.className = 'polyfinPosterRatingIcon'
      icon.innerHTML = '<svg viewBox="0 0 24 24" focusable="false"><path d="' + icons[entry[0]] + '"></path></svg>'
      score.appendChild(icon)
      score.appendChild(document.createTextNode(entry[1]))
      badge.appendChild(score)
    })
    poster.appendChild(badge)
  }
  // style adds the look of the ratings, once: hidden until the pointer or
  // the focus comes to the card, above jellyfin-web's hover menu, under a
  // version count, never in the pointer's way.
  function style() {
    if (document.getElementById('polyfinPosterRatingsStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinPosterRatingsStyle'
    sheet.textContent =
      '.polyfinPosterRatings{position:absolute;top:.45em;inset-inline-start:.45em;z-index:3;display:flex;flex-direction:column;' +
      'align-items:flex-start;gap:.3em;pointer-events:none;opacity:0;transition:opacity .15s ease}' +
      '.cardScalable:has(.mediaSourceIndicator) .polyfinPosterRatings{top:2.4em}' +
      '.card:hover .polyfinPosterRatings,.card:focus .polyfinPosterRatings,.card:focus-within .polyfinPosterRatings{opacity:1}' +
      '.polyfinPosterRating{display:inline-flex;align-items:center;gap:.3em;padding:.2em .5em;border-radius:1em;' +
      'background:rgba(0,0,0,.68);color:#fff;font-size:.8rem;font-weight:600;line-height:1.25;white-space:nowrap;' +
      'font-variant-numeric:tabular-nums;-webkit-backdrop-filter:blur(6px);backdrop-filter:blur(6px)}' +
      '.polyfinPosterRatingIcon{display:inline-flex}' +
      '.polyfinPosterRatingIcon svg{width:1.1em;height:1.1em;fill:currentColor}' +
      '.polyfinPosterRating-community .polyfinPosterRatingIcon{color:#ffc83d}' +
      '@media (prefers-reduced-motion:reduce){.polyfinPosterRatings{transition:none}}'
    document.head.appendChild(sheet)
  }
  addEventListener('mouseover', quietly(arrive), true)
  addEventListener('focusin', quietly(arrive), true)
})()

// Polyfin's line under jellyfin-web's spinner while a torrent starts (see
// the header's seventeenth), on its own.
;(function () {
  var startWords = {
    en: {
      looking: 'Finding peers…',
      finding: 'Finding peers: {found} found, {connected} connected',
      receiving: 'Receiving the start: {mb} MB from {peers} peers',
      receivingOne: 'Receiving the start: {mb} MB from 1 peer',
      failed: 'The torrent did not start: {why}',
      silent: 'its peers sent nothing',
      noUdp: 'no peer found, and the route carries no UDP',
      refused: 'its trackers did not answer',
      noTrackers: 'the addon named no tracker, and none was found otherwise',
      noPeers: 'no peer found',
      route: 'the route for torrents is down',
      atCap: 'as many torrents play as allowed',
    },
    fr: {
      looking: 'Recherche de pairs…',
      finding: 'Recherche de pairs : {found} trouvés, {connected} connectés',
      receiving: 'Réception du début : {mb} Mo depuis {peers} pairs',
      receivingOne: 'Réception du début : {mb} Mo depuis 1 pair',
      failed: 'Le torrent n’a pas démarré : {why}',
      silent: 'ses pairs n’ont rien envoyé',
      noUdp: 'aucun pair trouvé, et la route ne transporte pas l’UDP',
      refused: 'ses trackers n’ont pas répondu',
      noTrackers: 'l’addon n’a indiqué aucun tracker, et aucun pair n’a été trouvé autrement',
      noPeers: 'aucun pair trouvé',
      route: 'la route des torrents est coupée',
      atCap: 'autant de torrents sont lus que permis',
    },
    de: {
      looking: 'Suche nach Peers…',
      finding: 'Suche nach Peers: {found} gefunden, {connected} verbunden',
      receiving: 'Empfange den Anfang: {mb} MB von {peers} Peers',
      receivingOne: 'Empfange den Anfang: {mb} MB von 1 Peer',
      failed: 'Der Torrent ist nicht gestartet: {why}',
      silent: 'seine Peers haben nichts gesendet',
      noUdp: 'kein Peer gefunden, und die Route überträgt kein UDP',
      refused: 'seine Tracker haben nicht geantwortet',
      noTrackers: 'das Add-on hat keinen Tracker genannt, und anders wurde kein Peer gefunden',
      noPeers: 'kein Peer gefunden',
      route: 'die Route für Torrents ist unterbrochen',
      atCap: 'es laufen so viele Torrents wie erlaubt',
    },
    es: {
      looking: 'Buscando pares…',
      finding: 'Buscando pares: {found} encontrados, {connected} conectados',
      receiving: 'Recibiendo el inicio: {mb} MB de {peers} pares',
      receivingOne: 'Recibiendo el inicio: {mb} MB de 1 par',
      failed: 'El torrent no se inició: {why}',
      silent: 'sus pares no enviaron nada',
      noUdp: 'no se encontró ningún par, y la ruta no transporta UDP',
      refused: 'sus trackers no respondieron',
      noTrackers: 'la extensión no indicó ningún tracker, y no se encontró ningún par de otro modo',
      noPeers: 'no se encontró ningún par',
      route: 'la ruta de los torrents está caída',
      atCap: 'se reproducen tantos torrents como se permite',
    },
    it: {
      looking: 'Ricerca dei peer…',
      finding: 'Ricerca dei peer: {found} trovati, {connected} connessi',
      receiving: 'Ricezione dell’inizio: {mb} MB da {peers} peer',
      receivingOne: 'Ricezione dell’inizio: {mb} MB da 1 peer',
      failed: 'Il torrent non è partito: {why}',
      silent: 'i suoi peer non hanno inviato nulla',
      noUdp: 'nessun peer trovato, e il percorso non trasporta UDP',
      refused: 'i suoi tracker non hanno risposto',
      noTrackers: 'l’add-on non ha indicato nessun tracker, e non è stato trovato nessun peer in altro modo',
      noPeers: 'nessun peer trovato',
      route: 'il percorso dei torrent è interrotto',
      atCap: 'sono in riproduzione tanti torrent quanti consentiti',
    },
    pt: {
      looking: 'Procurando peers…',
      finding: 'Procurando peers: {found} encontrados, {connected} conectados',
      receiving: 'Recebendo o início: {mb} MB de {peers} peers',
      receivingOne: 'Recebendo o início: {mb} MB de 1 peer',
      failed: 'O torrent não começou: {why}',
      silent: 'seus peers não enviaram nada',
      noUdp: 'nenhum peer encontrado, e a rota não transporta UDP',
      refused: 'seus trackers não responderam',
      noTrackers: 'o complemento não indicou nenhum tracker, e nenhum peer foi encontrado de outro modo',
      noPeers: 'nenhum peer encontrado',
      route: 'a rota dos torrents está fora do ar',
      atCap: 'há tantos torrents em reprodução quanto o permitido',
    },
  }
  // The words of each failure Polyfin tells.
  var failures = { silent: 'silent', no_udp: 'noUdp', refused: 'refused', no_trackers: 'noTrackers', no_peers: 'noPeers', route: 'route', at_cap: 'atCap' }
  function language() {
    return /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
  }
  function say(key) {
    return (startWords[language()] || startWords.en)[key]
  }
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // The spinner watched, the round of asks while it shows (0 while it does
  // not), the ask to come, and the line drawn.
  var watched = null
  var rounds = 0
  var polling = 0
  var timer = null
  var line = null
  function spinning() {
    return !!document.querySelector('.docspinner.mdlSpinnerActive')
  }
  // watch follows jellyfin-web's spinner, which it makes at its first
  // show, then turns on and off by its class.
  function watch() {
    var loader = document.querySelector('.docspinner')
    if (loader && loader !== watched) {
      watched = loader
      new MutationObserver(quietly(check)).observe(loader, { attributes: true, attributeFilter: ['class'] })
    }
    check()
  }
  function check() {
    if (spinning()) {
      if (!polling) {
        polling = ++rounds
        ask(polling)
      }
      return
    }
    if (!polling) return
    polling = 0
    clearTimeout(timer)
    timer = null
    undraw()
  }
  function ask(round) {
    timer = null
    if (round !== polling) return
    var api = window.ApiClient
    if (!api || !api.getCurrentUserId()) return again(round)
    api
      .getJSON(api.getUrl('Polyfin/Playback/Starting'))
      .then(
        quietly(function (progress) {
          if (round !== polling) return
          draw(describe(progress))
          again(round)
        }),
        quietly(function () {
          if (round === polling) again(round)
        })
      )
      .catch(function () {})
  }
  function again(round) {
    timer = setTimeout(
      quietly(function () {
        ask(round)
      }),
      1000
    )
  }
  // describe words how the start goes, empty when nothing starts.
  function describe(progress) {
    if (!progress || !progress.Stage) return ''
    if (progress.Stage === 'failed') return say('failed').replace('{why}', say(failures[progress.Failure] || 'noPeers'))
    if (progress.Stage === 'pieces') {
      return say(progress.PeersConnected === 1 ? 'receivingOne' : 'receiving')
        .replace('{mb}', megabytes(progress.Received))
        .replace('{peers}', String(progress.PeersConnected))
    }
    if (!progress.PeersFound) return say('looking')
    return say('finding').replace('{found}', String(progress.PeersFound)).replace('{connected}', String(progress.PeersConnected))
  }
  // megabytes writes bytes in megabytes, with one decimal, as the
  // language writes numbers.
  function megabytes(bytes) {
    var lang = String(document.documentElement.lang || 'en')
    try {
      return (bytes / 1e6).toLocaleString(lang, { minimumFractionDigits: 1, maximumFractionDigits: 1 })
    } catch (error) {
      return (bytes / 1e6).toFixed(1)
    }
  }
  function draw(text) {
    if (!text) return undraw()
    style()
    if (!line) {
      line = document.createElement('div')
      line.className = 'polyfinTorrentStart'
      line.setAttribute('role', 'status')
      line.setAttribute('aria-live', 'polite')
    }
    if (line.textContent !== text) line.textContent = text
    if (!line.parentNode) document.body.appendChild(line)
    place()
  }
  // place puts the line under the spinner where it shows: jellyfin-web
  // centres it in the window, a web theme may move it, as the default
  // theme does under its header.
  function place() {
    var loader = document.querySelector('.docspinner.mdlSpinnerActive')
    var box = loader && loader.getBoundingClientRect && loader.getBoundingClientRect()
    if (!box || !box.height || !line.style) return
    line.style.top = Math.round(box.bottom + 12) + 'px'
    line.style.left = Math.round(box.left + box.width / 2) + 'px'
  }
  function undraw() {
    if (line && line.parentNode) line.parentNode.removeChild(line)
  }
  // style places the line under the spinner as jellyfin-web places it,
  // centred in the window, 10vh wide and tall, above everything, until
  // place finds where it shows.
  function style() {
    if (document.getElementById('polyfinTorrentStartStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinTorrentStartStyle'
    sheet.textContent =
      '.polyfinTorrentStart{position:fixed;left:50%;top:calc(50% + 7vh);transform:translateX(-50%);z-index:9999999;' +
      'padding:.35em .9em;border-radius:1em;background:rgba(0,0,0,.65);color:#fff;font-size:1em;line-height:1.4;' +
      'text-align:center;white-space:nowrap;pointer-events:none}'
    document.head.appendChild(sheet)
  }
  // The script runs from the page's head: the body, where jellyfin-web
  // puts its spinner, comes later.
  function start() {
    new MutationObserver(quietly(watch)).observe(document.body, { childList: true })
    watch()
  }
  if (document.body) quietly(start)()
  else addEventListener('DOMContentLoaded', quietly(start))
})()

;(function () {
  var providerWords = {
    en: {
      signIn: 'Sign in with {0}',
      signingIn: 'Signing in…',
      refused: 'The sign-in was cancelled.',
      noAccount: 'No account on this server matches your account at the identity provider.',
      disabled: 'This account is disabled.',
      hours: 'This account may not sign in at this time.',
      remote: 'This account may only sign in from the local network.',
      tooMany: 'Too many attempts. Try again later.',
      failed: 'The sign-in with the identity provider failed. Try again.',
    },
    fr: {
      signIn: 'Se connecter avec {0}',
      signingIn: 'Connexion…',
      refused: 'La connexion a été annulée.',
      noAccount: 'Aucun compte de ce serveur ne correspond à votre compte chez le fournisseur d’identité.',
      disabled: 'Ce compte est désactivé.',
      hours: 'Ce compte ne peut pas se connecter à cette heure.',
      remote: 'Ce compte ne peut se connecter que depuis le réseau local.',
      tooMany: 'Trop de tentatives. Réessayez plus tard.',
      failed: 'La connexion avec le fournisseur d’identité a échoué. Réessayez.',
    },
    de: {
      signIn: 'Mit {0} anmelden',
      signingIn: 'Anmeldung…',
      refused: 'Die Anmeldung wurde abgebrochen.',
      noAccount: 'Kein Konto dieses Servers passt zu deinem Konto beim Identitätsanbieter.',
      disabled: 'Dieses Konto ist deaktiviert.',
      hours: 'Dieses Konto darf sich zu dieser Zeit nicht anmelden.',
      remote: 'Dieses Konto darf sich nur im lokalen Netzwerk anmelden.',
      tooMany: 'Zu viele Versuche. Versuche es später erneut.',
      failed: 'Die Anmeldung beim Identitätsanbieter ist fehlgeschlagen. Versuche es erneut.',
    },
    es: {
      signIn: 'Iniciar sesión con {0}',
      signingIn: 'Iniciando sesión…',
      refused: 'Se canceló el inicio de sesión.',
      noAccount: 'Ninguna cuenta de este servidor corresponde a su cuenta del proveedor de identidad.',
      disabled: 'Esta cuenta está desactivada.',
      hours: 'Esta cuenta no puede iniciar sesión a esta hora.',
      remote: 'Esta cuenta solo puede iniciar sesión desde la red local.',
      tooMany: 'Demasiados intentos. Vuelva a intentarlo más tarde.',
      failed: 'El inicio de sesión con el proveedor de identidad falló. Vuelva a intentarlo.',
    },
    it: {
      signIn: 'Accedi con {0}',
      signingIn: 'Accesso in corso…',
      refused: 'L’accesso è stato annullato.',
      noAccount: 'Nessun account di questo server corrisponde al tuo account presso il provider di identità.',
      disabled: 'Questo account è disattivato.',
      hours: 'Questo account non può accedere a quest’ora.',
      remote: 'Questo account può accedere solo dalla rete locale.',
      tooMany: 'Troppi tentativi. Riprova più tardi.',
      failed: 'L’accesso con il provider di identità non è riuscito. Riprova.',
    },
    pt: {
      signIn: 'Entrar com {0}',
      signingIn: 'Entrando…',
      refused: 'O login foi cancelado.',
      noAccount: 'Nenhuma conta deste servidor corresponde à sua conta no provedor de identidade.',
      disabled: 'Esta conta está desativada.',
      hours: 'Esta conta não pode entrar neste horário.',
      remote: 'Esta conta só pode entrar pela rede local.',
      tooMany: 'Tentativas demais. Tente novamente mais tarde.',
      failed: 'O login com o provedor de identidade falhou. Tente novamente.',
    },
  }
  // The words for what the server says went wrong; any other, failed.
  var providerOutcomes = {
    provider_refused: 'refused',
    provider_no_account: 'noAccount',
    account_disabled: 'disabled',
    outside_allowed_hours: 'hours',
    outside_local_network: 'remote',
    too_many_attempts: 'tooMany',
    signing_in: 'signingIn',
  }
  function sayProvider(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (providerWords[language] || providerWords.en)[key]
  }
  if (typeof fetch !== 'function' || typeof localStorage === 'undefined' || !document.querySelectorAll) return
  var credentialsKey = 'jellyfin_credentials'
  // provider is the provider /admin/api/status names, null for none;
  // message what went wrong, or signing_in, "" for nothing.
  var provider = null
  var message = ''
  var outcome = ''
  try {
    var address = new URL(location.href)
    outcome = address.searchParams.get('polyfinProvider') || ''
    if (outcome) {
      address.searchParams.delete('polyfinProvider')
      history.replaceState(history.state, '', address.pathname + address.search + address.hash)
    }
  } catch (error) {
    return
  }
  message = outcome === 'signed-in' ? 'signing_in' : outcome
  function quietly(run) {
    return function () {
      try {
        return run.apply(this, arguments)
      } catch (error) {
        return undefined
      }
    }
  }
  fetch('/admin/api/status', { credentials: 'same-origin' })
    .then(function (response) {
      return response.ok ? response.json() : null
    })
    .then(
      quietly(function (status) {
        var named = status && status.provider
        provider = named && typeof named.start === 'string' && typeof named.name === 'string' ? named : null
        follow()
      })
    )
    .catch(function () {})
  if (outcome === 'signed-in') settle(0)
  // settle trades the ticket once jellyfin-web drew its first page, or
  // after 20 seconds.
  function settle(waited) {
    if ((/^#!?\//.test(location.hash) && document.querySelector('.page')) || waited >= 20000) return trade()
    setTimeout(
      quietly(function () {
        settle(waited + 250)
      }),
      250
    )
  }
  // device is how jellyfin-web names itself: its client's, else what it
  // keeps in the browser and the browser it runs in.
  function device() {
    var api = typeof window !== 'undefined' && window.ApiClient
    var ask = function (name) {
      try {
        return api && typeof api[name] === 'function' ? String(api[name]() || '') : ''
      } catch (error) {
        return ''
      }
    }
    var agent = typeof navigator !== 'undefined' ? String(navigator.userAgent || '') : ''
    var id = ask('deviceId') || localStorage.getItem('_deviceId2')
    if (!id) {
      id = btoa(agent + '|' + Date.now()).replace(/=/g, '1')
      localStorage.setItem('_deviceId2', id)
    }
    var browser = /Firefox\//.test(agent)
      ? 'Firefox'
      : /Edg\//.test(agent)
        ? 'Edge Chromium'
        : /OPR\//.test(agent)
          ? 'Opera'
          : /Chrome\//.test(agent)
            ? 'Chrome'
            : /Safari\//.test(agent)
              ? 'Safari'
              : 'Web Browser'
    return {
      client: ask('appName') || 'Jellyfin Web',
      name: ask('deviceName') || browser,
      id: id,
      version: ask('appVersion') || '12.4.0',
    }
  }
  // encode escapes a value of the header once, as jellyfin-web sends it:
  // its client gives some values escaped already, such as its app's name
  // (Jellyfin%20Web), which the server takes back once.
  function encode(value) {
    try {
      return encodeURIComponent(decodeURIComponent(value))
    } catch (error) {
      return encodeURIComponent(value)
    }
  }
  function trade() {
    var named = device()
    var header =
      'MediaBrowser Client="' +
      encode(named.client) +
      '", Device="' +
      encode(named.name) +
      '", DeviceId="' +
      encode(named.id) +
      '", Version="' +
      encode(named.version) +
      '"'
    fetch('/admin/api/provider/web', { method: 'POST', credentials: 'same-origin', headers: { Authorization: header } })
      .then(function (response) {
        return response.json().then(function (body) {
          return { ok: response.ok, body: body }
        })
      })
      .then(
        quietly(function (answer) {
          if (!answer.ok || !answer.body || typeof answer.body.accessToken !== 'string') {
            message = (answer.body && answer.body.error) || 'failed'
            return follow()
          }
          keep(answer.body)
          location.replace(location.pathname)
        })
      )
      .catch(function () {
        message = 'failed'
        follow()
      })
  }
  // keep puts the sign-in in this server's entry of jellyfin-web's
  // credentials, first, keeping what jellyfin-web saved there.
  function keep(signedIn) {
    var stored
    try {
      stored = JSON.parse(localStorage.getItem(credentialsKey) || '{}')
    } catch (error) {
      stored = {}
    }
    if (!stored || typeof stored !== 'object' || Array.isArray(stored)) stored = {}
    var servers = Array.isArray(stored.Servers) ? stored.Servers : []
    var entry = { ManualAddress: location.origin, LastConnectionMode: 2 }
    var others = []
    servers.forEach(function (server) {
      if (server && typeof server === 'object' && server.Id === signedIn.serverId) entry = server
      else others.push(server)
    })
    entry.Id = signedIn.serverId
    entry.Name = entry.Name || signedIn.serverName
    entry.UserId = signedIn.userId
    entry.AccessToken = signedIn.accessToken
    entry.DateLastAccessed = Date.now()
    stored.Servers = [entry].concat(others)
    localStorage.setItem(credentialsKey, JSON.stringify(stored))
  }
  // place draws the button, and the line under it, on the sign-in pages
  // shown, and reports whether there was one.
  function place() {
    var found = false
    var pages = document.querySelectorAll('#loginPage .readOnlyContent')
    for (var i = 0; i < pages.length; i++) {
      var container = pages[i]
      found = true
      var button = container.querySelector('.polyfinProviderButton')
      var line = container.querySelector('.polyfinProviderMessage')
      if (!provider) {
        if (button) button.remove()
        if (line) line.remove()
        continue
      }
      if (!button) {
        button = document.createElement('button')
        button.setAttribute('type', 'button')
        button.className = 'raised block emby-button polyfinProviderButton'
        button.appendChild(document.createElement('span'))
        button.addEventListener(
          'click',
          quietly(function () {
            location.href = provider.start + '?app=web'
          })
        )
        container.insertBefore(button, container.firstChild)
      }
      button.firstChild.textContent = sayProvider('signIn').replace('{0}', function () {
        return provider.name
      })
      if (!message) {
        if (line) line.remove()
        continue
      }
      if (!line) {
        line = document.createElement('p')
        line.className = 'fieldDescription polyfinProviderMessage'
        line.setAttribute('role', 'status')
        container.insertBefore(line, button.nextSibling)
      }
      line.textContent = sayProvider(providerOutcomes[message] || 'failed')
    }
    return found
  }
  // follow looks for the sign-in page as the route changes, every 200
  // milliseconds for 5 seconds, until it finds it.
  var looking = null
  function follow() {
    clearTimeout(looking)
    var tries = 0
    var look = quietly(function () {
      looking = null
      if (place() || ++tries >= 25) return
      looking = setTimeout(look, 200)
    })
    look()
  }
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      quietly(follow)()
      return result
    }
  })
  addEventListener('hashchange', quietly(follow))
  addEventListener('popstate', quietly(follow))
})()
