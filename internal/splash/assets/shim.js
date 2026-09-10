// Splash readiness shim. Injected into every splash document (standalone and
// overlay). Page templates only ever call window.splash.animated().
//
//   page → host : splash.animated()   intro finished (auto after all
//                                     [data-splash-intro] animations end, or 5s)
//   host → page : splash.dismiss()    host decided: animated ∧ connected ∧ minDuration
//   page → host : splash.dismissed()  fade-out done (auto 400ms after dismiss)
//
// Transport: window.__splashPost(name) when the native host bound it, and a
// document CustomEvent('splash:' + name) for the browser host.
//
// Ordering: this shim runs inline in <head>, so its own DOMContentLoaded
// listener (registered below when the document is still loading) fires
// before a page script's own DOMContentLoaded listener (registered later, in
// <body>) does. For the phase==='final' / reduced-motion path, watchIntro
// would otherwise call splash.animated() synchronously inside its own
// listener, dispatching 'splash:animated' before the page's listener exists
// to hear it — deferring with setTimeout(splash.animated, 0) lets
// later-registered DOMContentLoaded listeners run first. window.splash.sent
// is exposed as a fallback for any listener that still starts late (e.g. one
// created inside a nested DOMContentLoaded/async callback): it can check
// sent.animated to catch up on an event it missed.
(function () {
  if (window.splash) return;
  var cfg = window.SPLASH || {};
  var sent = {};

  function post(name) {
    if (sent[name]) return;
    sent[name] = true;
    if (typeof window.__splashPost === 'function') {
      try { window.__splashPost(name); } catch (e) { /* host gone */ }
    }
    try { document.dispatchEvent(new CustomEvent('splash:' + name)); } catch (e) { /* old engine */ }
  }

  function root() { return document.getElementById('splash'); }

  var splash = window.splash = {
    animated: function () { post('animated'); },
    dismiss: function () {
      var r = root();
      if (r) r.classList.add('splash-dismissing');
      setTimeout(splash.dismissed, 400);
    },
    dismissed: function () { post('dismissed'); },
    config: cfg,
    sent: sent,
  };

  function watchIntro() {
    var r = root();
    if (!r) return;
    var reduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (cfg.phase === 'final' || reduced) { setTimeout(splash.animated, 0); return; }
    var els = r.querySelectorAll('[data-splash-intro]');
    var pending = els.length;
    var cap = setTimeout(splash.animated, 5000);
    if (!pending) return; // custom page: it calls splash.animated() itself (or the cap fires)
    for (var i = 0; i < els.length; i++) {
      els[i].addEventListener('animationend', function () {
        if (--pending <= 0) { clearTimeout(cap); splash.animated(); }
      }, { once: true });
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', watchIntro);
  } else {
    watchIntro();
  }
})();
