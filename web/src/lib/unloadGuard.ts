// Shared flag so the global ErrorBoundary can force a reload without SiteForm's
// "unsaved changes" beforeunload guard blocking it. Needed because React may not
// have run the crashed subtree's effect cleanup (which removes the listener)
// before the boundary reloads, and `window.onbeforeunload = null` does NOT remove
// an addEventListener("beforeunload", ...) handler.
let bypass = false;

/** Skip the next beforeunload prompt (e.g. when the ErrorBoundary auto-reloads). */
export function bypassUnloadGuardOnce() {
  bypass = true;
}

export function isUnloadGuardBypassed() {
  return bypass;
}
