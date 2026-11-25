// Part 6: the duplicate error of a refused playback.
//
// It closes the generic error jellyfin-web shows on top of a refused
// playback's own error. When a title's PlaybackInfo answers with an
// ErrorCode, such as the RateLimitExceeded of a user playing on as many
// devices as allowed, jellyfin-web 12.4 explains the code, then rejects the
// playback without a reason, and its rejection handler shows "There was an
// error processing the request" over it: the user meets the wrong error
// first. Of two alerts with the same title shown within a second of each
// other, the first still open, the script hides the second at once, with
// its backdrop, and closes it once it is open, as its button would.
//
// Anchors in jellyfin-web:
//   selector: .dialog
//   selector: .formDialogHeaderTitle
//   selector: .btnOption
//   class: dialogBackdrop
//   class: opened

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
