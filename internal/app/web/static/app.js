// ReelAnvil page behaviour. Served from /assets/app.js so the Content Security
// Policy can forbid inline scripts.
(function () {
  'use strict';

  // Sidebar: compact rail (remembered per browser) and grouped flyouts.
  function initSidebar() {
    const shell = document.querySelector('.shell');
    const sidebar = document.getElementById('app-sidebar');
    const toggle = document.getElementById('rail-toggle');
    const flyout = document.getElementById('rail-flyout');
    if (!shell || !sidebar || !toggle || !flyout) return;
    let compact = false;
    try { compact = localStorage.getItem('reelanvil-rail') === 'compact'; } catch (e) {}

    function hideFlyout() {
      flyout.classList.remove('visible');
      flyout.replaceChildren();
    }
    function setCompact(v) {
      compact = v;
      shell.classList.toggle('rail-compact', v);
      toggle.setAttribute('aria-label', v ? 'Expand sidebar' : 'Collapse sidebar');
      toggle.setAttribute('aria-expanded', String(!v));
      hideFlyout();
      try { localStorage.setItem('reelanvil-rail', v ? 'compact' : 'expanded'); } catch (e) {}
    }

    sidebar.querySelectorAll('.nav-group').forEach(function (g) {
      const button = g.querySelector('.nav-group-toggle');
      const active = !!g.querySelector('a.active');
      g.classList.toggle('open', active);
      button.setAttribute('aria-expanded', String(active));
      button.classList.toggle('group-active', active);
      button.addEventListener('click', function () {
        if (!compact) {
          g.classList.toggle('open');
          button.setAttribute('aria-expanded', String(g.classList.contains('open')));
          return;
        }
        if (flyout.classList.contains('visible') && flyout.dataset.group === g.dataset.group) {
          hideFlyout();
          return;
        }
        flyout.dataset.group = g.dataset.group;
        flyout.replaceChildren();
        const title = document.createElement('div');
        title.className = 'flyout-title';
        title.textContent = button.querySelector('span').textContent;
        flyout.append(title);
        g.querySelectorAll('.nav-children a').forEach(function (a) { flyout.append(a.cloneNode(true)); });
        flyout.classList.add('visible');
      });
    });

    setCompact(compact);
    toggle.addEventListener('click', function () { setCompact(!compact); });
    sidebar.addEventListener('keydown', function (e) { if (e.key === 'Escape') hideFlyout(); });
    document.addEventListener('click', function (e) { if (!sidebar.contains(e.target)) hideFlyout(); });
  }

  // Settings → Themes: preview the selected theme before saving. Each option
  // carries the body class it needs, so built-in IDs are not repeated here.
  function initThemePicker() {
    const picker = document.getElementById('theme');
    if (!picker) return;
    const initial = document.body.className;
    function apply() {
      const option = picker.selectedOptions[0];
      document.body.className = option ? option.dataset.bodyClass : initial;
    }
    picker.addEventListener('change', apply);
    // Restore the saved theme when the page comes back from the bfcache with a
    // reset form.
    window.addEventListener('pageshow', function () { if (!picker.value) document.body.className = initial; });
  }

  // Settings → Themes: custom theme editor with an unsaved preview.
  function initThemeEditor() {
    const editor = document.getElementById('theme-editor');
    if (!editor) return;
    const select = document.getElementById('custom-id');
    select?.addEventListener('change', function () {
      const u = new URL(location.href);
      u.searchParams.set('edit', select.value);
      location.assign(u.toString());
    });
    const fields = [...editor.querySelectorAll('.color-field input')];
    const name = editor.querySelector('#custom-name');
    const swatches = document.getElementById('custom-swatches');
    if (!swatches) return;
    const original = new Map();
    original.set('name', name ? name.value : '');
    fields.forEach(function (i) { original.set(i.name, i.value); });

    function preview() {
      swatches.replaceChildren();
      fields.forEach(function (i) {
        const v = i.value;
        if (!/^#[0-9a-fA-F]{6}$/.test(v)) return;
        const chip = document.createElement('span');
        chip.className = 'swatch';
        chip.style.backgroundColor = v;
        swatches.append(chip);
        i.parentElement.querySelector('.color-chip').style.backgroundColor = v;
      });
    }
    fields.forEach(function (i) {
      i.addEventListener('change', preview);
      i.addEventListener('keydown', function (e) {
        if (e.key === 'Enter') { e.preventDefault(); i.blur(); preview(); }
      });
    });
    document.getElementById('theme-cancel')?.addEventListener('click', function () {
      if (name) name.value = original.get('name');
      fields.forEach(function (i) { i.value = original.get(i.name); });
      preview();
    });
    preview();
  }

  // Campaigns: copy a campaign link to the clipboard.
  function initCopyLinks() {
    document.querySelectorAll('.copy-link').forEach(function (b) {
      b.addEventListener('click', async function () {
        try {
          await navigator.clipboard.writeText(b.dataset.copy);
          const before = b.textContent;
          b.textContent = 'Copied';
          setTimeout(function () { b.textContent = before; }, 1200);
        } catch (e) {
          b.textContent = 'Copy unavailable';
        }
      });
    });
  }

  initSidebar();
  initThemePicker();
  initThemeEditor();
  initCopyLinks();
})();
