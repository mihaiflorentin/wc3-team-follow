'use strict';

const $ = id => document.getElementById(id);
const key = tag => tag.trim().toLowerCase();
const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;
const nameOf = tag => tag.split('#')[0];

const bridge = window.go?.gui?.Bridge ?? demoBridge();
const MAX_TAGS = 12;

let state = null;
let lastJoinSeen;
let exePath = '';

/* Calls ---------------------------------------------------------------- */

async function act(call) {
  $('error').textContent = '';
  try {
    render(await call());
    return true;
  } catch (err) {
    $('error').textContent = String(err?.message ?? err);
    return false;
  }
}

const configure = cfg => act(() => bridge.Configure(cfg));

const autoJoin = fromMenus => configure({enabled: true, diagnostic: false, fromMenus});

// changeGroup applies a new selection. The engine stops auto-join on any
// change; when it was on and nothing is in the way, it is turned back on
// for the new group.
async function changeGroup(cfg) {
  const before = state;
  const ok = await configure(cfg);
  if (ok && before?.enabled && !before.busy && !before.queued && !before.inMatch && hasTargets(state)) await autoJoin(true);
  return ok;
}

/* Building blocks ------------------------------------------------------- */

function icon(name) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('class', 'icon');
  svg.setAttribute('aria-hidden', 'true');
  const use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
  use.setAttribute('href', `#i-${name}`);
  svg.append(use);
  return svg;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

// A stable colour per friend, so the same person looks the same everywhere.
function hue(tag) {
  let h = 0;
  for (const ch of key(tag)) h = (h * 31 + ch.codePointAt(0)) % 360;
  return h;
}

function avatar(tag, {badge = false, banned = false} = {}) {
  const node = el('span', banned ? 'avatar banned' : 'avatar');
  node.style.setProperty('--hue', hue(tag));
  node.setAttribute('aria-hidden', 'true');
  const letters = [...nameOf(tag)].filter(c => /\p{L}|\p{N}/u.test(c));
  node.textContent = (letters[0] ?? '?').toUpperCase() + (letters[1] ?? '').toLowerCase();
  if (badge) {
    const b = el('span', 'badge');
    b.append(icon('enter'));
    node.append(b);
  }
  return node;
}

// Status keys for colour: open, game, online, away, offline.
function statusOf(f) {
  if (f.open) return 'open';
  if (f.inGame) return 'game';
  if (f.online && f.away) return 'away';
  if (f.online) return 'online';
  return 'offline';
}

function statusOfLabel(label, open) {
  if (open || label === 'Lobby open') return 'open';
  if (label === 'In Warcraft III') return 'game';
  if (label === 'Online in Battle.net') return 'online';
  if (label === 'Away') return 'away';
  return 'offline';
}

function presence(text, lobby) {
  const node = el('span', 'presence');
  node.append(document.createTextNode(text));
  if (lobby) {
    node.firstChild.appendData(':');
    node.append(el('span', 'lobby', lobby));
    node.title = lobby;
  }
  return node;
}

/* Derived state -------------------------------------------------------- */

// isBlacklisted reports whether tag is on the blacklist.
const isBlacklisted = (s, tag) => (s.blacklist ?? []).some(b => key(b.tag) === key(tag));

// noteOf returns the player's note about tag, or undefined.
const noteOf = (s, tag) => s.notes?.[key(tag)];

// matchesPlayer reports whether a search finds tag, by its BattleTag or by
// the player's note about it. query is lower-cased.
const matchesPlayer = (s, tag, query) => key(tag).includes(query) || Boolean(noteOf(s, tag)?.text.toLowerCase().includes(query));

// noteMark is the small icon on a row whose player has a note.
function noteMark() {
  const mark = el('span', 'note-mark');
  mark.setAttribute('role', 'img');
  mark.setAttribute('aria-label', 'Has a note');
  mark.title = 'You have a note about this player';
  mark.append(icon('note'));
  return mark;
}

// nameCell is a row's BattleTag, with the note icon when the player has a
// note and the Blacklisted badge when it is on the blacklist.
function nameCell(tag, banned, noted = false) {
  const wrap = el('span', 'name-wrap');
  const name = el('span', 'name', tag);
  name.title = tag;
  wrap.append(name);
  if (noted) wrap.append(noteMark());
  if (banned) {
    const badge = el('span', 'ban-badge');
    badge.title = 'Blacklisted';
    badge.append(icon('ban'), el('span', 'ban-badge-text', 'Blacklisted'));
    wrap.append(badge);
  }
  return wrap;
}

// lockBox makes a checkbox unavailable without disabling it, so it stays
// focusable and the player menu still opens on its row from the keyboard.
function lockBox(box, locked) {
  box.setAttribute('aria-disabled', String(locked));
  if (locked) box.addEventListener('click', event => event.preventDefault());
}

// markPlayer makes row a player row: right-click, Shift+F10 or the Menu key
// on it opens the player menu for tag, and focus returns to target.
function markPlayer(row, target, tag) {
  row.dataset.player = tag;
  target.dataset.tag = tag;
  target.setAttribute('aria-keyshortcuts', 'Shift+F10');
}

function hasTargets(s) {
  return (s.includeTeam && s.team.some(m => !m.isSelf)) || s.manualTags.length > 0;
}

function tone(s) {
  if (!s.connected) return 'off';
  if (s.passwordRequest) return 'alert';
  if (s.host?.enabled) return s.host.phase === 'match' ? 'paused' : 'live';
  if (s.enabled && s.paused) return 'paused';
  if (s.enabled) return 'live';
  return hasTargets(s) ? 'watching' : 'off';
}

const toneIcon = {off: 'plug', watching: 'eye', live: 'enter', paused: 'pause', alert: 'lock'};

// The hint's first sentence becomes the title, so a problem such as two
// copies of the game running reads as the headline, not small print.
function waitingHeadline(hint) {
  if (!hint) return ['Waiting for Warcraft III', 'Start the game through Battle.net. Team Follow finds it by itself.'];
  const cut = hint.indexOf('. ');
  if (cut < 0) return [hint.replace(/\.$/, ''), 'Team Follow connects by itself.'];
  return [hint.slice(0, cut), hint.slice(cut + 2)];
}

function headline(s) {
  const n = s.watched.length;
  if (!s.connected) return waitingHeadline(s.connectionHint);
  if (s.passwordRequest) {
    const r = s.passwordRequest;
    return ['This lobby needs a password', `${r.friend}'s lobby “${r.lobby}” asked for one.`];
  }
  if (s.host?.enabled) return hostHeadline(s);
  if (s.enabled && s.paused) return ['Following, paused for now', s.status];
  if (s.enabled && s.joinPending && s.lastJoin) return [`Joining ${nameOf(s.lastJoin.friend)}…`, s.status];
  if (s.enabled) return [`Following ${plural(n, 'friend', 'friends')}`, 'The first of them to open a lobby gets joined. Joining pauses while you play.'];
  if (!hasTargets(s)) return ['Choose who to follow', 'Tick friends in the list below, or form a team in Warcraft III.'];
  const members = group(s);
  const open = members.filter(m => m.status === 'open');
  const watching = `Watching ${plural(members.length, 'friend', 'friends')}`;
  if (open.length) return [watching, `${nameOf(open[0].tag)} has a lobby open. Turn on Auto-join to join lobbies like this automatically.`];
  return [watching, 'You’ll see here when one of them opens a lobby. Turn on Auto-join to follow them in.'];
}

function blockedReason(s) {
  if (!s.connected) return 'Waiting for Warcraft III.';
  if (!s.verifiedBuild) return `Auto-join isn’t available on game build ${s.version} until it’s been tested. Watching still works.`;
  if (!hasTargets(s)) return 'Choose friends or form a team first.';
  if (s.joinPending) return 'A join is still in progress.';
  if (s.busy || s.queued || s.inMatch) return 'Go back to the game’s menus first. Auto-join can’t start in a lobby, match or queue.';
  return '';
}

// The group shown in the party bar: who is followed now, or who would be.
function group(s) {
  if (s.enabled) return s.watched.map(w => ({tag: w.battleTag, status: statusOfLabel(w.label, w.open), lobby: w.lobby}));
  const out = [];
  if (s.includeTeam) {
    for (const m of s.team) if (!m.isSelf) out.push({tag: m.battleTag, status: statusOfLabel(m.status)});
  }
  for (const tag of s.manualTags) {
    if (out.some(o => key(o.tag) === key(tag))) continue;
    const r = s.roster.find(f => key(f.battleTag) === key(tag));
    out.push({tag, status: r ? statusOf(r) : 'offline', lobby: r?.lobby});
  }
  return out;
}

/* Rendering ------------------------------------------------------------ */

function render(s) {
  if (!s) return;
  state = s;

  $('connection').dataset.state = s.connected ? 'on' : 'off';
  $('connection-text').textContent = s.connected ? `Warcraft III ${s.version || ''}`.trim() : 'Not connected';

  const beacon = $('beacon');
  const t = tone(s);
  beacon.dataset.tone = t;
  const glyph = s.host?.enabled ? 'crown' : s.connected && t === 'off' ? 'users' : toneIcon[t];
  $('state-icon').setAttribute('href', `#i-${glyph}`);
  const joinStamp = s.lastJoin?.time;
  if (lastJoinSeen !== undefined && joinStamp && joinStamp !== lastJoinSeen) {
    beacon.classList.remove('flare');
    void beacon.offsetWidth;
    beacon.classList.add('flare');
  }
  lastJoinSeen = joinStamp ?? null;

  const [title, next] = headline(s);
  $('status').textContent = title;
  $('next').textContent = next === title ? '' : next;

  renderCommand(s);
  renderCloseGames(s);
  renderSteps(s);
  renderParty(s);
  $('password-form').hidden = !s.passwordRequest;
  if (!s.passwordRequest) $('lobby-password').value = '';
  renderActions(s);
  renderHost(s);
  $('ask-password').checked = Boolean(s.askPassword);
  $('join-host-team').checked = Boolean(s.joinHostTeam);
  renderFriends(s);
  renderRecent(s);
  renderBlacklist(s);
  renderLog(s);
  renderUpdate(s);
}

function renderCommand(s) {
  const [reason, command] = (s.connectionHint || '').split(' To allow it, run: ');
  const show = !s.connected && Boolean(command);
  $('hint-command').hidden = !show;
  if (!show) return;
  $('next').textContent = `${reason} To allow it, run this in a terminal, then restart Team Follow:`;
  $('hint-command-text').textContent = command.replace('<path to wc3-team-follow>', exePath || '<path to wc3-team-follow>');
}

// manyGames: discovery found more than one copy of the game, so the hero
// offers to close them all.
const manyGames = s => !s.connected && s.gameCount > 1;

function renderCloseGames(s) {
  const show = manyGames(s);
  if (!show) $('close-confirm').hidden = true;
  $('close-games-row').hidden = !show || !$('close-confirm').hidden;
  $('close-games').disabled = closingGames;
}

function renderSteps(s) {
  const steps = $('steps');
  steps.hidden = s.enabled || s.host?.enabled || Boolean(s.passwordRequest) || !$('hint-command').hidden || manyGames(s);
  const done = {game: s.connected, people: hasTargets(s), start: false};
  const current = !done.game ? 'game' : !done.people ? 'people' : 'start';
  for (const li of steps.children) {
    const step = li.dataset.step;
    li.dataset.done = String(done[step]);
    li.dataset.current = String(step === current);
  }
  const n = group(s).length;
  steps.querySelector('[data-step="people"] .step-text').textContent = n ? `${plural(n, 'friend', 'friends')} chosen` : 'Choose friends to follow';
}

let partySignature = '';

// renderParty draws the group bar. Each chip is a button that opens the
// player menu; it is redrawn only when the group changes, so focus stays.
function renderParty(s) {
  const list = $('party');
  const members = group(s);
  const signature = JSON.stringify(members);
  if (signature !== partySignature) {
    partySignature = signature;
    const focused = list.contains(document.activeElement) ? document.activeElement.dataset.tag : '';
    list.replaceChildren();
    for (const m of members) {
      const li = el('li');
      li.dataset.status = m.status;
      const chip = el('button', 'chip');
      chip.type = 'button';
      chip.title = m.lobby ? `${m.tag}: lobby “${m.lobby}”` : m.tag;
      chip.setAttribute('aria-label', m.lobby ? `${m.tag}, lobby ${m.lobby}` : m.tag);
      chip.setAttribute('aria-haspopup', 'menu');
      chip.setAttribute('aria-expanded', 'false');
      markPlayer(chip, chip, m.tag);
      chip.onclick = () => openPlayerMenu(m.tag, chip);
      chip.append(avatar(m.tag, {badge: m.status === 'open'}), el('span', 'tag', nameOf(m.tag)));
      li.append(chip);
      list.append(li);
    }
    if (focused) list.querySelector(`[data-tag="${CSS.escape(focused)}"]`)?.focus();
  }
  const open = members.filter(m => m.status === 'open').length;
  $('party-note').textContent = !members.length ? ''
    : s.enabled ? (open ? `${plural(open, 'lobby is', 'lobbies are')} open right now.` : 'Nobody in your group has a lobby open yet.')
    : '';
}

function renderActions(s) {
  const toggle = $('autojoin');
  const reason = s.enabled ? '' : blockedReason(s);
  toggle.checked = s.enabled;
  toggle.disabled = !s.enabled && Boolean(reason);
  $('autojoin-help').textContent = s.enabled
    ? (s.paused ? 'On, and paused while you’re in a lobby, match or queue.' : 'On. Joins the first friend in your group who opens a lobby.')
    : 'Join the first friend in your group who opens a lobby. Pauses while you play.';
  $('blocked').textContent = s.connected ? reason : '';
  if (confirmFor === 'Auto-join' && (s.enabled || reason)) closeConfirm();
}

function watchedFor(s, tag) {
  return s.watched.find(w => key(w.battleTag) === key(tag));
}

// Status order for the picker: joinable first, offline last.
function rank(f) {
  return {open: 0, game: 1, online: 2, away: 3, offline: 4}[statusOf(f)];
}

// pickerRows merges the Battle.net roster with chosen friends that aren't on
// it (typed by hand), chosen first, then by status and name.
function pickerRows(s) {
  const chosen = new Set(s.manualTags.map(key));
  const rows = s.roster.map(f => ({...f, selected: chosen.has(key(f.battleTag))}));
  for (const tag of s.manualTags) {
    if (rows.some(r => key(r.battleTag) === key(tag))) continue;
    const live = s.friends.find(f => key(f.battleTag) === key(tag));
    rows.push({battleTag: tag, label: s.connected ? (live?.label ?? 'Waiting for activity') : 'Saved', open: live?.open, online: false, selected: true});
  }
  const byStatus = (a, b) => rank(a) - rank(b) || key(a.battleTag).localeCompare(key(b.battleTag));
  return {
    chosen: rows.filter(r => r.selected).sort(byStatus),
    others: rows.filter(r => !r.selected).sort(byStatus),
  };
}

function pickRow(f, full) {
  const li = el('li');
  const label = el('label', 'pick');
  label.dataset.status = statusOf(f);
  label.dataset.selected = String(f.selected);
  const box = document.createElement('input');
  box.type = 'checkbox';
  box.checked = f.selected;
  box.dataset.tag = f.battleTag;
  const banned = isBlacklisted(state, f.battleTag);
  const locked = (!f.selected && full) || banned;
  lockBox(box, locked);
  box.setAttribute('aria-label', banned ? `${f.battleTag} is blacklisted` : `Follow ${f.battleTag}`);
  label.dataset.disabled = String(locked);
  markPlayer(label, box, f.battleTag);
  box.onchange = () => {
    const tags = box.checked
      ? [...state.manualTags, f.battleTag]
      : state.manualTags.filter(t => key(t) !== key(f.battleTag));
    changeGroup({tags});
  };
  label.append(box, avatar(f.battleTag, {badge: Boolean(f.open), banned}), nameCell(f.battleTag, banned, Boolean(noteOf(state, f.battleTag))), presence(f.open ? 'Lobby open' : f.label, f.open ? f.lobby : ''));
  li.append(label);
  return li;
}

let pickerSignature = '';

function renderFriends(s) {
  const list = $('friends');
  const query = $('friend-search').value.trim().toLowerCase();
  const showOffline = $('show-offline').checked;
  const full = s.manualTags.length >= MAX_TAGS;
  const {chosen, others} = pickerRows(s);
  const visible = others.filter(f => (query ? matchesPlayer(s, f.battleTag, query) : showOffline || f.online));
  const hiddenOffline = others.filter(f => !f.online).length;

  // Re-render only when something visible changed, so scrolling and focus
  // survive the once-a-second state updates.
  const signature = JSON.stringify([s.connected, full, chosen, visible, query, showOffline, s.roster.length, s.blacklist, s.notes]);
  if (signature !== pickerSignature) {
    pickerSignature = signature;
    const scroll = list.scrollTop;
    const focused = document.activeElement?.dataset?.tag;
    list.replaceChildren();
    if (!s.connected && !chosen.length) {
      list.append(el('li', 'empty', 'Start Warcraft III to load your Battle.net friends.'));
    } else {
      for (const f of chosen) list.append(pickRow(f, full));
      if (chosen.length && visible.length) list.append(el('li', 'divider', query ? 'Matches' : 'Everyone else'));
      for (const f of visible) list.append(pickRow(f, full));
      if (!visible.length && s.connected) {
        if (query) list.append(el('li', 'empty', `No friend matches “${query}”.`));
        else if (!s.roster.length) list.append(el('li', 'empty', 'Loading your Battle.net friends…'));
        else if (!showOffline && hiddenOffline) list.append(el('li', 'empty', `None of your other friends are online. Turn on Show offline to see all ${s.roster.length}.`));
      }
    }
    list.scrollTop = scroll;
    if (focused) list.querySelector(`input[data-tag="${CSS.escape(focused)}"]`)?.focus();
  }

  // How many friends are online sits on the Friends tab; the header counts
  // the follow group, which both tabs share.
  const online = s.roster.filter(f => f.online).length;
  const badge = $('online-count');
  badge.textContent = s.roster.length ? String(online) : '';
  badge.title = s.roster.length ? `${online} of ${s.roster.length} online` : '';
  if (s.roster.length) $('tab-friends').setAttribute('aria-label', `Friends, ${online} online`);
  else $('tab-friends').removeAttribute('aria-label');
  const count = $('friend-count');
  count.textContent = s.manualTags.length ? `${s.manualTags.length} of ${MAX_TAGS} chosen` : '';
  count.dataset.full = String(full);
  $('friend-tag').disabled = full;
  $('friend-form').querySelector('button').disabled = full;
  const note = $('friend-note');
  note.textContent = s.preferencesError
    || (full ? `You can follow up to ${MAX_TAGS} friends. Untick someone to choose another.` : 'Tick everyone you play with. Team Follow joins whoever opens a lobby first.');
  note.classList.toggle('error-note', Boolean(s.preferencesError));
}

/* People tabs ---------------------------------------------------------- */

const TAB_KEY = 'wtf.peopleTab';
let tab = 'friends';
try {
  const saved = localStorage.getItem(TAB_KEY);
  if (saved === 'recent' || saved === 'blacklist') tab = saved;
} catch { /* storage blocked */ }

const placeholders = {friends: 'Search your friends and notes', recent: 'Search recent players and notes', blacklist: 'Search the blacklist and notes'};

const tabs = [...document.querySelectorAll('.tabs [role="tab"]')];

function selectTab(name, {focus = false} = {}) {
  tab = name;
  try { localStorage.setItem(TAB_KEY, name); } catch { /* storage blocked */ }
  for (const t of tabs) {
    const on = t.dataset.tab === name;
    t.setAttribute('aria-selected', String(on));
    t.tabIndex = on ? 0 : -1;
    if (on && focus) t.focus();
  }
  $('panel-friends').hidden = name !== 'friends';
  $('panel-recent').hidden = name !== 'recent';
  $('panel-blacklist').hidden = name !== 'blacklist';
  $('show-offline-switch').hidden = name !== 'friends';
  $('friend-search').placeholder = placeholders[name];
  $('search-label').textContent = placeholders[name];
  closeClearConfirm();
  if (name !== 'recent') leaveBanMode();
  if (state) render(state);
}

for (const t of tabs) {
  t.onclick = () => selectTab(t.dataset.tab);
  // Arrow keys, Home and End move between tabs, as a tab list does.
  t.onkeydown = event => {
    const i = tabs.indexOf(t);
    const next = {ArrowRight: i + 1, ArrowLeft: i - 1, Home: 0, End: tabs.length - 1}[event.key];
    if (next === undefined) return;
    event.preventDefault();
    selectTab(tabs[(next + tabs.length) % tabs.length].dataset.tab, {focus: true});
  };
}

/* Recent players ------------------------------------------------------- */

// lastSeen says when a recent player was last in a lobby or match with
// you: "Now", "2 min ago", "3 hours ago", "Yesterday" or a date.
function lastSeen(iso, here, now = new Date()) {
  if (here) return 'Now';
  const then = new Date(iso);
  const mins = Math.floor((now - then) / 60000);
  if (mins < 1) return 'Just now';
  if (mins < 60) return `${mins} min ago`;
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  if (then >= midnight) return plural(Math.floor(mins / 60), 'hour ago', 'hours ago');
  if (then >= new Date(midnight - 86400000)) return 'Yesterday';
  const opts = {day: 'numeric', month: 'short'};
  if (then.getFullYear() !== now.getFullYear()) opts.year = 'numeric';
  return then.toLocaleDateString([], opts);
}

// recentStatus works out a recent player's status: in your lobby, their
// Battle.net status when they're a friend, or that the game doesn't say.
function recentStatus(s, tag) {
  if (s.lobbyTags.some(t => key(t) === key(tag))) return {status: 'lobby', label: 'In your lobby', lobby: ''};
  const f = s.roster.find(r => key(r.battleTag) === key(tag));
  if (f) return {status: statusOf(f), label: f.open ? 'Lobby open' : f.label, lobby: f.open ? f.lobby : ''};
  if (s.connected && s.roster.length) return {status: 'unknown', label: 'Not a friend', lobby: ''};
  return {status: 'unknown', label: '', lobby: ''};
}

function recentRow(s, p, full) {
  const chosen = s.manualTags.some(t => key(t) === key(p.tag));
  const banned = isBlacklisted(s, p.tag);
  const st = recentStatus(s, p.tag);
  const li = el('li');
  const label = el('label', 'pick');
  label.dataset.status = st.status;
  const box = document.createElement('input');
  box.type = 'checkbox';
  box.dataset.tag = p.tag;
  if (banMode) {
    // Choosing whom to blacklist: the follow group stays as it is.
    const picked = banPick.has(key(p.tag));
    box.checked = picked;
    lockBox(box, banned);
    box.setAttribute('aria-label', banned ? `${p.tag} is blacklisted` : `Blacklist ${p.tag}`);
    label.dataset.selected = 'false';
    label.dataset.picked = String(picked);
    box.onchange = () => {
      // A changed choice needs a new confirmation.
      banConfirm = false;
      if (box.checked) banPick.add(key(p.tag));
      else banPick.delete(key(p.tag));
      renderRecent(state);
    };
  } else {
    box.checked = chosen;
    lockBox(box, (!chosen && full) || banned);
    box.setAttribute('aria-label', banned ? `${p.tag} is blacklisted` : `Follow ${p.tag}`);
    label.dataset.selected = String(chosen);
    box.onchange = () => {
      const tags = box.checked
        ? [...state.manualTags, p.tag]
        : state.manualTags.filter(t => key(t) !== key(p.tag));
      changeGroup({tags});
    };
  }
  label.dataset.disabled = box.getAttribute('aria-disabled');
  markPlayer(label, box, p.tag);
  const name = nameCell(p.tag, banned, Boolean(noteOf(s, p.tag)));
  const meta = el('span', 'meta');
  const map = el('span', 'map');
  map.append(icon('map'), document.createTextNode(p.map || 'Unknown map'));
  map.title = p.map;
  const seen = el('time', '', lastSeen(p.lastSeen, st.status === 'lobby'));
  seen.dateTime = p.lastSeen;
  seen.title = new Date(p.lastSeen).toLocaleString();
  meta.append(map, seen);
  label.append(box, avatar(p.tag, {badge: st.status === 'open', banned}), name, presence(st.label, st.lobby), meta);
  li.append(label);
  return li;
}

let recentSignature = '';

function renderRecent(s) {
  const list = $('recent');
  const recent = s.recent ?? [];
  const query = $('friend-search').value.trim().toLowerCase();
  const full = s.manualTags.length >= MAX_TAGS;
  const visible = query ? recent.filter(p => matchesPlayer(s, p.tag, query) || p.map.toLowerCase().includes(query)) : recent;
  $('recent-count').textContent = recent.length ? String(recent.length) : '';

  if (tab === 'recent') {
    // Last seen is rounded to minutes, so this changes at most once a minute.
    const signature = JSON.stringify([visible.map(p => [p.tag, p.map, lastSeen(p.lastSeen, false), recentStatus(s, p.tag)]), s.manualTags, full, query,
      s.blacklist, s.notes, banMode, banConfirm, [...banPick]]);
    if (signature !== recentSignature) {
      recentSignature = signature;
      const scroll = list.scrollTop;
      const focused = document.activeElement?.dataset?.tag;
      list.replaceChildren();
      list.classList.toggle('ban-mode', banMode);
      list.setAttribute('aria-label', banMode ? 'Recent players: choose whom to blacklist' : 'Recent players');
      for (const p of visible) list.append(recentRow(s, p, full));
      if (!recent.length) list.append(el('li', 'empty', 'Players appear here after you’ve been in a lobby with them.'));
      else if (!visible.length) list.append(el('li', 'empty', `No recent player matches “${query}”.`));
      list.scrollTop = scroll;
      if (focused) list.querySelector(`input[data-tag="${CSS.escape(focused)}"]`)?.focus();
    }
  }

  // Ticking someone who isn't a friend: say what that does.
  const strangers = s.connected && s.roster.length
    ? s.manualTags.filter(t => recent.some(p => key(p.tag) === key(t)) && !s.roster.some(f => key(f.battleTag) === key(t)))
    : [];
  const note = $('recent-note');
  note.textContent = s.preferencesError
    || (banMode ? 'Tick the players to keep out of lobbies you host. Who you follow doesn’t change.' : '')
    || (strangers.length
      ? `Auto-join follows only Battle.net friends into lobbies. Auto-host still counts ${strangers.length === 1 ? strangers[0] : 'these players'} for Same team and Start when full.`
      : full ? `You can follow up to ${MAX_TAGS} players. Untick someone to choose another.`
        : 'Everyone who was in a lobby or match with you. Kept on this computer only.');
  note.classList.toggle('error-note', Boolean(s.preferencesError));
  $('recent-clear').hidden = !recent.length || !$('recent-confirm').hidden;
  if (!recent.length) closeClearConfirm();
  $('ban-select').hidden = !recent.length || !$('recent-confirm').hidden;
  if (!recent.length) leaveBanMode();
  renderBanBar();
}

/* Choosing recent players to blacklist ---------------------------------- */

// banMode switches the Recent players checkboxes from the follow group to
// a separate choice of players to blacklist; banPick holds that choice.
let banMode = false;
let banConfirm = false;
const banPick = new Set();

function renderBanBar() {
  $('recent-foot').hidden = banMode;
  $('ban-bar').hidden = !banMode;
  if (!banMode) return;
  const n = banPick.size;
  const who = plural(n, 'player', 'players');
  $('ban-bar-text').textContent = banConfirm
    ? `Blacklist ${who}? They’re banned from lobbies you host${state.manualTags.some(t => banPick.has(key(t))) ? ' and leave who you follow' : ''}.`
    : n ? `${who} chosen.` : 'Choose players to blacklist.';
  $('ban-apply').textContent = banConfirm ? 'Blacklist' : n ? `Blacklist ${who}` : 'Blacklist';
  $('ban-apply').disabled = !n;
  $('ban-cancel').textContent = banConfirm ? 'Back' : 'Cancel';
}

function enterBanMode() {
  closeClearConfirm();
  banMode = true;
  banConfirm = false;
  banPick.clear();
  renderRecent(state);
  ($('recent').querySelector('input:not([aria-disabled="true"])') ?? $('ban-cancel')).focus();
}

function leaveBanMode({focus = false} = {}) {
  if (!banMode) return;
  banMode = false;
  banConfirm = false;
  banPick.clear();
  if (state) renderRecent(state);
  if (focus) ($('ban-select').hidden ? $('tab-recent') : $('ban-select')).focus();
}

$('ban-select').onclick = enterBanMode;
$('ban-cancel').onclick = () => {
  if (banConfirm) {
    banConfirm = false;
    renderRecent(state);
    $('ban-apply').focus();
  } else {
    leaveBanMode({focus: true});
  }
};
$('ban-apply').onclick = async () => {
  if (!banPick.size) return;
  if (!banConfirm) {
    banConfirm = true;
    renderRecent(state);
    $('ban-cancel').focus();
    return;
  }
  const tags = (state.recent ?? []).map(p => p.tag).filter(t => banPick.has(key(t)));
  if (await act(() => bridge.AddToBlacklist(tags))) leaveBanMode({focus: true});
};
// Escape steps back: from the confirmation to choosing, then out.
$('panel-recent').addEventListener('keydown', event => {
  if (event.key !== 'Escape' || !banMode) return;
  event.preventDefault();
  $('ban-cancel').click();
});

/* Blacklist ------------------------------------------------------------- */

const TAG_PATTERN = /^[^\s#]{2,32}#\d{3,12}$/;

// addedOn says when a player was blacklisted: "Added today", "Added
// yesterday" or "Added 3 Oct".
function addedOn(iso, now = new Date()) {
  const then = new Date(iso);
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  if (then >= midnight) return 'Added today';
  if (then >= new Date(midnight - 86400000)) return 'Added yesterday';
  const opts = {day: 'numeric', month: 'short'};
  if (then.getFullYear() !== now.getFullYear()) opts.year = 'numeric';
  return `Added ${then.toLocaleDateString([], opts)}`;
}

function blacklistRow(s, entry) {
  const li = el('li');
  const row = el('div', 'person');
  row.dataset.status = 'banned';
  const added = el('time', 'added', addedOn(entry.added));
  added.dateTime = entry.added;
  added.title = new Date(entry.added).toLocaleString();
  const remove = el('button', 'ghost small remove');
  remove.type = 'button';
  remove.dataset.tag = entry.tag;
  remove.append(icon('x'), el('span', '', 'Remove'));
  remove.setAttribute('aria-label', `Remove ${entry.tag} from the blacklist`);
  remove.onclick = async () => {
    const rows = [...$('blacklist').querySelectorAll('button.remove')];
    const next = rows[rows.indexOf(remove) + 1] ?? rows[rows.indexOf(remove) - 1];
    if (await act(() => bridge.RemoveFromBlacklist(entry.tag))) {
      const target = next && $('blacklist').querySelector(`button.remove[data-tag="${CSS.escape(next.dataset.tag)}"]`);
      (target ?? $('ban-tag')).focus();
    }
  };
  const name = nameCell(entry.tag, false, Boolean(noteOf(s, entry.tag)));
  row.append(avatar(entry.tag, {banned: true}), name, added, remove);
  markPlayer(row, remove, entry.tag);
  li.append(row);
  return li;
}

let blacklistSignature = '';
let banNoteError = '';

function renderBlacklist(s) {
  const entries = s.blacklist ?? [];
  const count = $('blacklist-count');
  count.textContent = entries.length ? String(entries.length) : '';
  if (entries.length) $('tab-blacklist').setAttribute('aria-label', `Blacklist, ${plural(entries.length, 'player', 'players')}`);
  else $('tab-blacklist').removeAttribute('aria-label');
  $('auto-ban').checked = Boolean(s.autoBan);
  $('ban-manual').checked = Boolean(s.blacklistManualBans);
  if (tab !== 'blacklist') return;

  const query = $('friend-search').value.trim().toLowerCase();
  const visible = query ? entries.filter(b => matchesPlayer(s, b.tag, query)) : entries;
  const signature = JSON.stringify([visible.map(b => [b.tag, addedOn(b.added), noteOf(s, b.tag)?.text ?? '']), query, entries.length]);
  if (signature !== blacklistSignature) {
    blacklistSignature = signature;
    const list = $('blacklist');
    const scroll = list.scrollTop;
    const focused = document.activeElement?.dataset?.tag;
    list.replaceChildren();
    for (const b of visible) list.append(blacklistRow(s, b));
    if (!entries.length) list.append(el('li', 'empty', 'Nobody is blacklisted. Add a BattleTag below, or choose players in Recent players.'));
    else if (!visible.length) list.append(el('li', 'empty', `No blacklisted player matches “${query}”.`));
    list.scrollTop = scroll;
    if (focused) list.querySelector(`button.remove[data-tag="${CSS.escape(focused)}"]`)?.focus();
  }
  const note = $('ban-note');
  note.textContent = banNoteError || s.preferencesError
    || (s.autoBan ? 'Kept on this computer. Team Follow bans only in lobbies you host, never in someone else’s.'
      : 'Auto-ban is off. These players can join your lobbies until you turn it on.');
  note.classList.toggle('error-note', Boolean(banNoteError || s.preferencesError));
  $('ban-tag').setAttribute('aria-invalid', String(Boolean(banNoteError)));
}

function banNote(message) {
  banNoteError = message;
  if (state) renderBlacklist(state);
}

$('ban-tag').oninput = () => { if (banNoteError) banNote(''); };
$('ban-form').onsubmit = async event => {
  event.preventDefault();
  const tag = $('ban-tag').value.trim();
  if (!TAG_PATTERN.test(tag)) {
    banNote('Enter a full BattleTag, such as Player#1234.');
    $('ban-tag').focus();
    return;
  }
  if (isBlacklisted(state, tag)) {
    banNote(`${tag} is already on your blacklist.`);
    $('ban-tag').focus();
    return;
  }
  try {
    render(await bridge.AddToBlacklist([tag]));
    $('ban-tag').value = '';
    banNote('');
  } catch (err) {
    banNote(String(err?.message ?? err));
  }
  $('ban-tag').focus();
};
$('auto-ban').onchange = () => act(() => bridge.SetAutoBan($('auto-ban').checked));
$('ban-manual').onchange = () => act(() => bridge.SetBlacklistManualBans($('ban-manual').checked));

function openClearConfirm() {
  const n = state?.recent?.length ?? 0;
  if (!n) return;
  $('recent-confirm-text').textContent = `Clear all ${plural(n, 'recent player', 'recent players')}?`;
  $('recent-clear').hidden = true;
  $('recent-confirm').hidden = false;
  $('recent-clear-no').focus();
}

function closeClearConfirm({focus = false} = {}) {
  if ($('recent-confirm').hidden) return;
  $('recent-confirm').hidden = true;
  $('recent-clear').hidden = !(state?.recent?.length);
  if (focus) ($('recent-clear').hidden ? $('tab-recent') : $('recent-clear')).focus();
}

$('recent-clear').onclick = openClearConfirm;
$('recent-clear-no').onclick = () => closeClearConfirm({focus: true});
$('recent-confirm').onkeydown = event => {
  if (event.key === 'Escape') closeClearConfirm({focus: true});
};
$('recent-clear-yes').onclick = async () => {
  if (await act(() => bridge.ClearRecentPlayers())) {
    closeClearConfirm();
    $('tab-recent').focus();
  }
};

// Activity icons, by what the line is about.
const logKinds = [
  [/by mistake/, 'problem', 'alert'],
  [/^Banned |blacklist/, 'ban', 'ban'],
  [/^Asked to join|^Confirming|^Password sent/, 'join', 'enter'],
  [/^Joined /, 'joined', 'check'],
  [/opened a lobby|open lobby/, 'seen', 'eye'],
  [/paused|^Follow is still on|^Returned/, 'pause', 'pause'],
  [/couldn|could not|failed|didn't|expired|blocked|unrecognized|password/i, 'problem', 'alert'],
  [/^Auto-join is on|resumed/, 'join', 'play'],
  [/^Connected/, 'joined', 'plug'],
];

function renderLog(s) {
  const list = $('log');
  list.replaceChildren();
  const entries = s.log.slice(-14).reverse();
  if (!entries.length) return list.append(el('li', 'empty', 'Nothing yet.'));
  for (const entry of entries) {
    const [, kind, glyph] = logKinds.find(([re]) => re.test(entry.message)) ?? [null, 'info', 'info'];
    const li = el('li');
    li.dataset.kind = kind;
    const badge = el('span', 'kind');
    badge.append(icon(glyph));
    const body = el('div');
    const time = el('time', '', new Date(entry.time).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'}));
    time.dateTime = entry.time;
    body.append(time, el('span', 'text', entry.message));
    li.append(badge, body);
    list.append(li);
  }
}

/* Player menu ------------------------------------------------------------
   One menu for every player row (Friends, Recent players, Blacklist) and
   group chip. Right-click, Shift+F10 or the Menu key opens it; the arrow
   keys, Home and End move, Enter or Space chooses, and Esc or Tab closes
   it and puts focus back on the row. Its items come in groups split by a
   line: the player's note first, then the general actions, then the
   blacklist. */

// playerMenuGroups lists the actions that apply to tag right now. Each item
// is {label, icon, run, danger, disabled, hint}.
function playerMenuGroups(s, tag) {
  const k = key(tag);
  const self = s.team.some(m => m.isSelf && key(m.battleTag) === k);
  const chosen = s.manualTags.some(t => key(t) === k);
  // Members of the game team are followed through it, not through a choice
  // made here, so there is nothing to add or remove for them.
  const viaTeam = s.includeTeam && s.team.some(m => !m.isSelf && key(m.battleTag) === k);
  const banned = isBlacklisted(s, tag);
  const full = s.manualTags.length >= MAX_TAGS;

  const notes = noteOf(s, tag)
    ? [{label: 'Edit note', icon: 'pencil', run: () => openNote(tag, 'edit')}, {label: 'View note', icon: 'note', run: () => openNote(tag, 'view')}]
    : [{label: 'Add note', icon: 'note-add', run: () => openNote(tag, 'edit')}];
  const general = [{label: 'Copy BattleTag', icon: 'copy', run: () => copyTag(tag)}];
  if (chosen) {
    general.push({label: 'Remove from group', icon: 'user-minus', run: () => changeMenuGroup(tag, false)});
  } else if (!self && !viaTeam && !banned) {
    general.push({label: 'Add to group', icon: 'user-plus', run: () => changeMenuGroup(tag, true),
      disabled: full, hint: full ? `Your group is full (${MAX_TAGS})` : ''});
  }
  const groups = [notes, general];
  if (banned) groups.push([{label: 'Remove from blacklist', icon: 'unban', run: () => unblacklist(tag)}]);
  else if (!self) groups.push([{label: 'Add to blacklist', icon: 'ban', danger: true, run: () => blacklist(tag), hint: chosen ? 'Also leaves your group' : ''}]);
  return groups;
}

async function changeMenuGroup(tag, add) {
  const tags = add ? [...state.manualTags, tag] : state.manualTags.filter(t => key(t) !== key(tag));
  if (await changeGroup({tags})) toast(add ? `Added ${tag} to your group` : `Removed ${tag} from your group`);
}

async function blacklist(tag) {
  banPick.delete(key(tag));
  if (await act(() => bridge.AddToBlacklist([tag]))) toast(`Blacklisted ${tag}`);
}

async function unblacklist(tag) {
  if (await act(() => bridge.RemoveFromBlacklist(tag))) toast(`Removed ${tag} from the blacklist`);
}

// copyTag puts tag on the clipboard: the app's own clipboard in the window,
// the browser's in the demo page.
async function copyTag(tag) {
  let ok = false;
  try {
    if (window.runtime?.ClipboardSetText) ok = (await window.runtime.ClipboardSetText(tag)) !== false;
    else if (navigator.clipboard) ok = (await navigator.clipboard.writeText(tag), true);
  } catch { ok = false; }
  if (ok) toast(`Copied ${tag}`);
  else $('error').textContent = `Couldn’t copy ${tag} to the clipboard.`;
}

let toastTimer = 0;

// toast confirms an action in a short line at the bottom of the window;
// the hidden status line beside it is what screen readers read out.
function toast(text) {
  const node = $('toast');
  node.replaceChildren(icon('check'), el('span', '', text));
  $('announce').textContent = '';
  setTimeout(() => { $('announce').textContent = text; }, 50);
  node.dataset.show = 'true';
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { node.dataset.show = 'false'; }, 2400);
}

const menu = {tag: '', origin: null, scope: null, order: [], keyboardAt: 0};

const menuItems = () => [...$('player-menu').querySelectorAll('[role="menuitem"]')];

// openPlayerMenu opens the menu for tag. origin is the element focus goes
// back to; at is the pointer position, or null to place it by origin.
function openPlayerMenu(tag, origin, at = null) {
  if (!state) return;
  closePlayerMenu();
  closeNote({focus: false});
  const banned = isBlacklisted(state, tag);
  const chosen = state.manualTags.some(t => key(t) === key(tag));
  const box = $('player-menu');

  // The head repeats who the menu is for; the menu's label says it too.
  const head = el('div', 'menu-head');
  head.setAttribute('aria-hidden', 'true');
  const [name, number] = tag.split('#');
  const who = el('span', 'menu-who');
  const line = el('span', 'menu-name', name);
  if (number) line.append(el('span', 'menu-number', `#${number}`));
  who.append(line);
  const standing = banned ? 'Blacklisted' : chosen ? 'In your group' : '';
  if (standing) who.append(el('span', `menu-standing ${banned ? 'banned' : 'chosen'}`, standing));
  head.append(avatar(tag, {banned}), who);

  const list = el('ul');
  list.setAttribute('role', 'menu');
  list.setAttribute('aria-label', `Actions for ${tag}`);
  for (const items of playerMenuGroups(state, tag)) {
    if (!items.length) continue;
    if (list.children.length) {
      const sep = el('li', 'menu-sep');
      sep.setAttribute('role', 'separator');
      list.append(sep);
    }
    for (const item of items) {
      const li = el('li', item.danger ? 'menu-item danger' : 'menu-item');
      li.setAttribute('role', 'menuitem');
      li.tabIndex = -1;
      if (item.disabled) li.setAttribute('aria-disabled', 'true');
      const text = el('span', 'menu-text', item.label);
      if (item.hint) text.append(el('small', '', item.hint));
      li.append(icon(item.icon), text);
      li.onclick = () => chooseItem(li, item);
      li.onmousemove = () => { if (document.activeElement !== li) li.focus(); };
      list.append(li);
    }
  }
  list.onkeydown = menuKeys;
  box.replaceChildren(head, list);

  anchorTo(tag, origin);
  if (origin?.hasAttribute('aria-expanded')) origin.setAttribute('aria-expanded', 'true');
  box.hidden = false;
  placeMenu(box, origin, at);
  menuItems()[0]?.focus();
}

// anchorTo remembers the row a menu or popover for tag was opened on, so
// focus can go back to it, or to its neighbour when the lists redraw.
function anchorTo(tag, origin) {
  menu.tag = tag;
  menu.origin = origin;
  menu.scope = origin?.closest('ul') ?? null;
  menu.order = menu.scope ? [...menu.scope.querySelectorAll('[data-tag]')].map(n => n.dataset.tag) : [];
}

// placeMenu keeps the menu inside the window: below and right of the point
// when there is room, flipped above or to the left when not.
function placeMenu(box, origin, at) {
  const margin = 8;
  let x, y, above;
  if (at) {
    ({x, y} = at);
    above = y;
  } else {
    const r = (origin.closest('[data-player]') ?? origin).getBoundingClientRect();
    x = r.left + Math.min(40, r.width / 2);
    y = r.bottom + 4;
    above = r.top - 4;
  }
  const w = box.offsetWidth;
  const h = box.offsetHeight;
  let left = x;
  let top = y;
  let fromX = 'left';
  let fromY = 'top';
  if (left + w > innerWidth - margin) {
    left = Math.max(x - w, margin);
    fromX = 'right';
  }
  if (top + h > innerHeight - margin) {
    top = above - h >= margin ? above - h : innerHeight - margin - h;
    fromY = 'bottom';
  }
  box.style.left = `${Math.max(margin, Math.min(left, innerWidth - margin - w))}px`;
  box.style.top = `${Math.max(margin, Math.min(top, innerHeight - margin - h))}px`;
  box.style.transformOrigin = `${fromY} ${fromX}`;
}

// closePlayerMenu closes the menu; with focus, focus goes back to the row.
function closePlayerMenu({focus = false} = {}) {
  const box = $('player-menu');
  if (box.hidden) return;
  box.hidden = true;
  box.replaceChildren();
  if (menu.origin?.hasAttribute('aria-expanded')) menu.origin.setAttribute('aria-expanded', 'false');
  if (focus) focusPlayer();
}

// focusPlayer focuses the row the menu was opened on. When a list redrew,
// it finds the row again by BattleTag; when the row is gone, its neighbour,
// else the selected tab.
function focusPlayer() {
  const {origin, scope, order, tag} = menu;
  if (origin?.isConnected && origin.getClientRects().length) return origin.focus();
  const find = t => (scope?.isConnected ? scope.querySelector(`[data-tag="${CSS.escape(t)}"]`) : null);
  const i = Math.max(order.indexOf(tag), 0);
  const target = find(tag) ?? order.slice(i + 1).map(find).find(Boolean) ?? order.slice(0, i).reverse().map(find).find(Boolean);
  (target ?? $(`tab-${tab}`)).focus();
}

// chooseItem runs an item: the menu closes and focus returns to the row,
// and again once the lists have redrawn after the action.
async function chooseItem(li, item) {
  if (li.getAttribute('aria-disabled') === 'true') return;
  closePlayerMenu({focus: true});
  await item.run();
  const active = document.activeElement;
  if (!active || active === document.body || !menu.origin?.isConnected) focusPlayer();
}

function menuKeys(event) {
  const items = menuItems();
  const i = items.indexOf(document.activeElement);
  const move = n => {
    event.preventDefault();
    items[(n + items.length) % items.length]?.focus();
  };
  switch (event.key) {
    case 'ArrowDown': return move(i + 1);
    case 'ArrowUp': return move(i - 1);
    case 'Home': return move(0);
    case 'End': return move(items.length - 1);
    case 'Enter':
    case ' ':
      event.preventDefault();
      return items[i]?.click();
    case 'Escape':
    case 'Tab':
      event.preventDefault();
      event.stopPropagation();
      return closePlayerMenu({focus: true});
  }
  // A letter moves to the next item that starts with it.
  if (event.key.length === 1 && /\S/.test(event.key)) {
    const letter = event.key.toLowerCase();
    const next = [...items.slice(i + 1), ...items.slice(0, i + 1)].find(li => li.textContent.trim().toLowerCase().startsWith(letter));
    if (next) {
      event.preventDefault();
      next.focus();
    }
  }
}

document.addEventListener('keydown', event => {
  if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return;
  const row = document.activeElement?.closest?.('[data-player]');
  if (!row) return;
  event.preventDefault();
  menu.keyboardAt = Date.now();
  openPlayerMenu(row.dataset.player, document.activeElement);
});

document.addEventListener('contextmenu', event => {
  if ($('player-menu').contains(event.target)) return event.preventDefault();
  // Shift+F10 and the Menu key send this event too, after the keydown that
  // already opened the menu.
  if (Date.now() - menu.keyboardAt < 600) return event.preventDefault();
  const row = event.target.closest?.('[data-player]');
  if (!row) return;
  event.preventDefault();
  const target = row.matches('[data-tag]') ? row : row.querySelector('[data-tag]') ?? row;
  const fromKeyboard = event.clientX === 0 && event.clientY === 0;
  openPlayerMenu(row.dataset.player, target, fromKeyboard ? null : {x: event.clientX, y: event.clientY});
});

document.addEventListener('pointerdown', event => {
  if (!$('player-menu').contains(event.target)) closePlayerMenu();
}, true);
document.addEventListener('scroll', event => {
  if (!$('player-menu').contains(event.target)) closePlayerMenu();
}, true);
window.addEventListener('resize', () => closePlayerMenu());
window.addEventListener('blur', () => closePlayerMenu());

/* Player notes ------------------------------------------------------------
   One popover for writing, editing and reading the player's private note
   about someone, opened from the player menu. Notes stay on this computer;
   they are shown as plain text only. Esc or Cancel closes it with focus
   back on the row; Ctrl+Enter or Cmd+Enter saves. */

// NOTE_MAX is the longest note, in characters; Go enforces the same limit
// (dto.MaxNoteLength), and the textarea's data-max says it.
const NOTE_MAX = Number($('note-text').dataset.max);
const noteLength = text => [...text.trim()].length;

const notePop = {tag: '', mode: '', saving: false, warned: false};

// openNote opens the popover for tag: mode 'edit' writes or edits the note,
// 'view' reads it. origin is the row to anchor to; it defaults to the row
// the player menu was opened on.
function openNote(tag, mode, origin = menu.tag === tag ? menu.origin : null) {
  if (!state) return;
  const note = noteOf(state, tag);
  if (mode === 'view' && !note) mode = 'edit';
  if (origin) anchorTo(tag, origin);
  Object.assign(notePop, {tag, mode, saving: false, warned: false});

  const box = $('note-popover');
  const [name, number] = tag.split('#');
  const who = el('span', 'menu-who');
  const line = el('span', 'menu-name', name);
  if (number) line.append(el('span', 'menu-number', `#${number}`));
  const sub = mode === 'view' ? `Note, updated ${noteDate(note.updated)}` : note ? 'Editing your note' : 'New note, kept on this computer only';
  who.append(line, el('span', 'menu-standing', sub));
  $('note-head').replaceChildren(avatar(tag, {banned: isBlacklisted(state, tag)}), who);
  box.setAttribute('aria-label', mode === 'view' ? `Your note about ${tag}` : `${note ? 'Edit' : 'Add'} a note about ${tag}`);

  $('note-edit').hidden = mode !== 'edit';
  $('note-view').hidden = mode !== 'view';
  $('note-error').textContent = '';
  if (mode === 'edit') {
    $('note-text').value = note?.text ?? '';
    $('note-delete').hidden = !note;
    noteCount();
  } else {
    $('note-body').textContent = note.text;
  }
  box.hidden = false;
  placeNote();
  if (mode === 'edit') {
    const area = $('note-text');
    area.focus();
    area.setSelectionRange(area.value.length, area.value.length);
  } else {
    $('note-to-edit').focus();
  }
}

function placeNote() {
  const box = $('note-popover');
  if (box.hidden) return;
  const origin = menu.origin?.isConnected ? menu.origin : null;
  if (origin) placeMenu(box, origin, null);
  else {
    box.style.left = `${Math.max(8, (innerWidth - box.offsetWidth) / 2)}px`;
    box.style.top = `${Math.max(8, (innerHeight - box.offsetHeight) / 3)}px`;
  }
}

// noteDate says when a note was last saved: a day this year, else a date.
function noteDate(when) {
  const then = new Date(when);
  if (Number.isNaN(then.getTime()) || then.getFullYear() < 2000) return 'earlier';
  const opts = {day: 'numeric', month: 'short'};
  if (then.getFullYear() !== new Date().getFullYear()) opts.year = 'numeric';
  return then.toLocaleDateString([], opts);
}

// noteCount shows how many characters are left, and blocks saving past
// the limit; screen readers hear it once when the note gets too long.
function noteCount() {
  const n = noteLength($('note-text').value);
  const over = n > NOTE_MAX;
  const count = $('note-count');
  count.textContent = over ? `${n - NOTE_MAX} over the ${NOTE_MAX}-character limit` : `${n} / ${NOTE_MAX}`;
  count.dataset.state = over ? 'over' : n > NOTE_MAX - 50 ? 'near' : '';
  $('note-text').setAttribute('aria-invalid', String(over));
  $('note-save').disabled = over || notePop.saving;
  if (over && !notePop.warned) {
    notePop.warned = true;
    $('announce').textContent = `Notes can be at most ${NOTE_MAX} characters.`;
  }
  if (!over) notePop.warned = false;
}

// noteDirty reports whether the text differs from the saved note.
const noteDirty = () => notePop.mode === 'edit' && $('note-text').value.trim() !== (noteOf(state, notePop.tag)?.text ?? '');

function closeNote({focus = true} = {}) {
  const box = $('note-popover');
  if (box.hidden) return;
  box.hidden = true;
  notePop.mode = '';
  if (focus) focusPlayer();
}

// saveNote saves text as the note about the popover's player; empty text
// deletes it. Errors stay in the popover so nothing typed is lost.
async function saveNote(text) {
  const {tag} = notePop;
  const had = Boolean(noteOf(state, tag));
  text = text.trim();
  if (!text && !had) return closeNote();
  if (noteLength(text) > NOTE_MAX) return noteCount();
  notePop.saving = true;
  noteCount();
  try {
    render(await (text ? bridge.SetNote(tag, text) : bridge.DeleteNote(tag)));
  } catch (err) {
    notePop.saving = false;
    noteCount();
    $('note-error').textContent = String(err?.message ?? err);
    return;
  }
  closeNote();
  toast(text ? `Saved your note about ${tag}` : `Deleted your note about ${tag}`);
}

$('note-text').oninput = noteCount;
$('note-edit').onsubmit = event => {
  event.preventDefault();
  saveNote($('note-text').value);
};
$('note-text').onkeydown = event => {
  if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
    event.preventDefault();
    saveNote($('note-text').value);
  }
};
$('note-delete').onclick = () => saveNote('');
$('note-cancel').onclick = () => closeNote();
$('note-close').onclick = () => closeNote();
$('note-to-edit').onclick = () => openNote(notePop.tag, 'edit');

// Esc closes; Tab stays inside the popover.
$('note-popover').addEventListener('keydown', event => {
  if (event.key === 'Escape') {
    event.preventDefault();
    event.stopPropagation();
    return closeNote();
  }
  if (event.key !== 'Tab') return;
  const stops = [...$('note-popover').querySelectorAll('textarea, button')].filter(n => !n.disabled && n.getClientRects().length);
  const i = stops.indexOf(document.activeElement);
  const next = event.shiftKey ? (i <= 0 ? stops.length - 1 : i - 1) : (i + 1) % stops.length;
  event.preventDefault();
  stops[next]?.focus();
});

// A click outside closes a note being read, or one with no changes; a
// changed note stays open so nothing typed is lost.
document.addEventListener('pointerdown', event => {
  const box = $('note-popover');
  if (box.hidden || box.contains(event.target) || $('player-menu').contains(event.target)) return;
  if (!noteDirty()) closeNote({focus: false});
}, true);
window.addEventListener('resize', placeNote);

/* Events --------------------------------------------------------------- */

// confirmThen runs action once the player confirms they're in the menus,
// or right away when the game has already shown it. feature names what the
// confirm button turns on; only that feature's own state closes the question.
let confirmed = null;
let confirmFor = null;
function confirmThen(toggle, feature, action) {
  if (state.inMenus) return action();
  // The game hasn't said which screen it's on: ask before acting.
  toggle.checked = false;
  confirmed = action;
  confirmFor = feature;
  $('confirm-yes-text').textContent = `Yes, turn on ${feature}`;
  $('confirm').hidden = false;
  $('confirm-yes').focus();
}

$('autojoin').onchange = event => {
  if (!state) return;
  if (!event.target.checked) return configure({enabled: false});
  confirmThen(event.target, 'Auto-join', () => autoJoin(true));
};
function closeConfirm() {
  $('confirm').hidden = true;
  confirmed = confirmFor = null;
}
$('confirm-yes').onclick = () => { const action = confirmed; closeConfirm(); action?.(); };
$('confirm-no').onclick = closeConfirm;

$('friend-search').oninput = () => render(state);
$('show-offline').onchange = () => render(state);
$('ask-password').onchange = event => configure({askPassword: event.target.checked});
$('join-host-team').onchange = event => configure({joinHostTeam: event.target.checked});

$('friend-form').onsubmit = async event => {
  event.preventDefault();
  const tag = $('friend-tag').value.trim();
  if (!tag || !state) return;
  if (await changeGroup({tags: [...state.manualTags, tag]})) $('friend-tag').value = '';
};

$('password-form').onsubmit = async event => {
  event.preventDefault();
  let password = $('lobby-password').value;
  $('lobby-password').value = '';
  await act(() => bridge.SubmitPassword(password));
  password = '';
};
$('cancel-password').onclick = () => configure({enabled: false});

async function copy(text) {
  if (window.runtime?.ClipboardSetText) return window.runtime.ClipboardSetText(text);
  return navigator.clipboard?.writeText(text);
}

$('copy-command').onclick = () => copy($('hint-command-text').textContent);

/* Closing Warcraft III ------------------------------------------------- */

// Both buttons ask first, in the page; the Go side does the closing.
let closingGames = false;

async function closeGames(error) {
  closingGames = true;
  error.textContent = '';
  for (const id of ['close-games', 'close-yes', 'report-close-games', 'report-close-yes']) $(id).disabled = true;
  try {
    render(await bridge.CloseGames());
    return true;
  } catch (err) {
    error.textContent = String(err?.message ?? err);
    return false;
  } finally {
    closingGames = false;
    for (const id of ['close-games', 'close-yes', 'report-close-games', 'report-close-yes']) $(id).disabled = false;
  }
}

$('close-games').onclick = () => {
  $('close-confirm').hidden = false;
  $('close-games-row').hidden = true;
  $('close-no').focus();
};
$('close-no').onclick = () => {
  $('close-confirm').hidden = true;
  render(state);
};
$('close-yes').onclick = async () => {
  $('close-confirm').hidden = true;
  await closeGames($('error'));
  render(state);
};

function resetReportClose() {
  $('report-close-confirm').hidden = true;
  $('report-close-games').hidden = false;
  $('report-games-note').textContent = '';
  $('report-error').textContent = '';
}
$('report-close-games').onclick = () => {
  $('report-close-confirm').hidden = false;
  $('report-close-games').hidden = true;
  $('report-games-note').textContent = '';
  $('report-close-no').focus();
};
$('report-close-no').onclick = resetReportClose;
$('report-close-yes').onclick = async () => {
  $('report-close-confirm').hidden = true;
  $('report-close-games').hidden = false;
  if (await closeGames($('report-error'))) $('report-games-note').textContent = state.status;
};

$('report').onclick = async () => {
  resetReportClose();
  try {
    $('report-text').textContent = await bridge.Diagnostics();
    const folder = await bridge.LogFolder();
    $('log-path').textContent = folder ? `Log folder: ${folder}` : 'Logging to a file is turned off.';
    $('open-logs').hidden = !folder;
    $('report-dialog').showModal();
  } catch (err) {
    $('error').textContent = String(err?.message ?? err);
  }
};
$('report-copy').onclick = async () => {
  await copy($('report-text').textContent);
  const text = $('report-copy').querySelector('span');
  text.textContent = 'Copied';
  setTimeout(() => { text.textContent = 'Copy report'; }, 1500);
};
$('report-close').onclick = () => $('report-dialog').close();
$('open-logs').onclick = async () => {
  try {
    await bridge.OpenLogFolder();
  } catch (err) {
    $('log-path').textContent = String(err?.message ?? err);
  }
};

async function boot() {
  selectTab(tab);
  try { exePath = await bridge.ExecutablePath(); } catch { exePath = ''; }
  window.runtime?.EventsOn?.('state', render);
  try { render(await bridge.State()); } catch (err) { $('status').textContent = String(err); }
  // The Go side pushes every change; this poll only covers a missed event.
  setInterval(async () => { try { render(await bridge.State()); } catch { /* window closing */ } }, 5000);
  if (!window.go) {
    const params = new URLSearchParams(location.search);
    demoOpen(params.get('open') ?? params.get('scene'));
  }
}
boot();

// demoOpen opens a dialog in the demo page, for screenshots:
// ?open=host-setup|maps|maps-search|report|close-games. The recent scenes
// open the Recent players tab (recent-select and recent-select-confirm in
// its blacklist selection), the blacklist scenes the Blacklist tab, the
// menu scenes the player menu on a recent player, a blacklisted player, a
// group chip or a player with a note, and the note scenes the note popover.
async function demoOpen(what) {
  const wait = ms => new Promise(r => setTimeout(r, ms));
  switch (what) {
    case 'host-setup':
      $('host-setup').click();
      break;
    case 'maps':
    case 'maps-search':
      $('host-setup').click();
      await wait(50);
      $('host-choose-map').click();
      await wait(150);
      if (what === 'maps') {
        document.querySelector('#maps-rows tr')?.click();
        await wait(100);
        document.querySelectorAll('#maps-rows tr')[2]?.click();
      } else {
        $('maps-search').value = 'legion';
        $('maps-search').dispatchEvent(new Event('input'));
        await wait(400);
        document.querySelector('#maps-rows tr')?.click();
      }
      break;
    case 'report':
      $('report').click();
      break;
    case 'close-games':
      $('close-games').click();
      break;
    case 'recent':
    case 'recent-empty':
      selectTab('recent');
      break;
    case 'recent-clear':
      selectTab('recent');
      await wait(50);
      openClearConfirm();
      break;
    case 'recent-select':
    case 'recent-select-confirm':
      selectTab('recent');
      await wait(50);
      enterBanMode();
      banPick.add(key('Kelthuzad#6644'));
      banPick.add(key('Anubarak#5102'));
      banConfirm = what === 'recent-select-confirm';
      renderRecent(state);
      if (banConfirm) $('ban-cancel').focus();
      else document.activeElement?.blur();
      break;
    case 'blacklist':
    case 'blacklist-empty':
    case 'blacklist-off':
      selectTab('blacklist');
      break;
    case 'update-open':
    case 'update-notes':
      $('update-notes').open = true;
      break;
    // The Changelog in the Updates section, scrolled into view.
    case 'changelog':
    case 'changelog-dev':
      $('changelog').open = true;
      await wait(100);
      $('changelog').scrollIntoView({block: 'start'});
      break;
    // The player menu, opened from the keyboard on a row or chip.
    case 'menu-recent':
    case 'menu-blacklist':
    case 'menu-group': {
      const [list, tag] = {
        'menu-recent': ['recent', 'Kelthuzad#6644'],
        'menu-blacklist': ['blacklist', 'Deathwing#6666'],
        'menu-group': ['party', 'Thrall#1820'],
      }[what];
      if (list !== 'party') selectTab(list);
      await wait(50);
      const target = $(list).querySelector(`[data-tag="${CSS.escape(tag)}"]`);
      if (!target) break;
      target.closest('[data-player]').scrollIntoView({block: 'center'});
      await wait(50);
      openPlayerMenu(tag, target);
      break;
    }
    // Notes: rows with the note icon, a search that matches note text, and
    // the note popover adding, editing (one too long) and reading a note.
    case 'notes':
    case 'notes-search':
      selectTab('recent');
      if (what === 'notes-search') {
        $('friend-search').value = 'carries';
        $('friend-search').dispatchEvent(new Event('input'));
      }
      await wait(50);
      $('recent').scrollIntoView({block: 'center'});
      break;
    case 'menu-note':
    case 'note-add':
    case 'note-edit':
    case 'note-long':
    case 'note-view': {
      const tag = {'menu-note': 'Arthas#1123', 'note-add': 'Kelthuzad#6644', 'note-edit': 'Arthas#1123', 'note-long': 'Garrosh#7781', 'note-view': 'Jaina#2214'}[what];
      selectTab('recent');
      await wait(50);
      const target = $('recent').querySelector(`[data-tag="${CSS.escape(tag)}"]`);
      if (!target) break;
      target.closest('[data-player]').scrollIntoView({block: 'center'});
      await wait(50);
      if (what === 'menu-note') {
        openPlayerMenu(tag, target);
        break;
      }
      anchorTo(tag, target);
      openNote(tag, what === 'note-view' ? 'view' : 'edit', target);
      if (what === 'note-long') {
        $('note-text').value += ' He also leaves Legion TD when the first boss leaks. '.repeat(10);
        noteCount();
      }
      break;
    }
  }
}

/* Auto-host ------------------------------------------------------------- */

const hostAct = cfg => act(() => bridge.ConfigureHost(cfg));
const listNames = tags => {
  const names = tags.map(nameOf);
  return names.length <= 2 ? names.join(' and ') : `${names.slice(0, -1).join(', ')} and ${names.at(-1)}`;
};

function hostHeadline(s) {
  const h = {...s.host, missing: s.host.missing ?? [], offTeam: s.host.offTeam ?? []};
  const name = `“${h.settings.gameName}”`;
  const map = h.settings.mapName;
  switch (h.phase) {
    case 'waiting': {
      if (!h.createAt) return [`Hosting ${name} next`, `On ${map}.`];
      const secs = Math.max(0, Math.ceil((new Date(h.createAt) - Date.now()) / 1000));
      if (s.busy || s.inMatch || s.queued) return [`Hosting ${name} next`, `The lobby is created on ${map} once you're back in the game's menus.`];
      return [secs > 0 ? `Hosting ${name} in ${secs}s` : `Hosting ${name} now`, `On ${map}. Stay in the game's menus.`];
    }
    case 'creating':
      return ['Creating your lobby…', `${name} on ${map}.`];
    case 'lobby': {
      const slots = h.slots ? `${h.players} of ${h.slots} slots taken.` : '';
      const waiting = h.missing.length ? ` Waiting for ${listNames(h.missing)}.` : group(s).length ? ' Everyone you follow is in.' : '';
      const team = h.offTeam.length ? ` Moving ${listNames(h.offTeam)} to your team.` : '';
      const start = h.settings.startWhenFull ? ' It starts when full.' : ' Press Start in Warcraft III when you’re ready.';
      return [`Your lobby ${name} is open`, `${slots}${waiting}${team}${start}`.trim()];
    }
    case 'match':
      return ['Playing your hosted game', h.settings.delaySeconds ? `The next lobby opens ${h.settings.delaySeconds}s after the match.` : 'The next lobby opens right after the match.'];
  }
  return [`Hosting ${name}`, ''];
}

function hostBlocked(s) {
  const st = s.host.settings;
  if (!st.mapPath) return 'Choose a map in Set up first.';
  if (!st.gameName) return 'Name your lobby in Set up first.';
  if (!s.connected) return 'Waiting for Warcraft III.';
  if (!s.verifiedBuild) return `Auto-host isn’t available on game build ${s.version} until it’s been tested.`;
  if (s.busy || s.queued || s.inMatch) return 'Go back to the game’s menus first.';
  return '';
}

function hostSummary(st) {
  if (!st.mapPath) return 'Create your lobby on a map, then wait for your group.';
  const extras = [st.private ? 'private' : 'public', st.startWhenFull && 'starts when full', st.sameTeam && 'same team', st.inviteGroup && 'invites your group']
    .filter(Boolean).join(', ');
  return `${st.mapName}, “${st.gameName || 'no name yet'}”. ${extras.charAt(0).toUpperCase()}${extras.slice(1)}.${gameModeSummary(st)}`;
}

function gameModeSummary(st) {
  if (!st.gameMode) return '';
  const secs = st.gameModeDelaySeconds ?? 5;
  return secs ? ` Game mode ${st.gameMode} after ${secs} s.` : ` Game mode ${st.gameMode} right after loading.`;
}

// renderKeyboard shows, in Auto-host setup, whether Game mode can type on
// this computer, with a button to allow it when the system asks first.
function renderKeyboard(k) {
  const kb = k ?? {supported: false, permitted: false, reason: ''};
  const line = $('keyboard-line');
  if (kb.supported && kb.permitted) {
    line.dataset.state = 'ok';
    $('keyboard-note').textContent = 'Team Follow may type on this computer.';
  } else if (kb.supported) {
    line.dataset.state = 'ask';
    $('keyboard-note').textContent = kb.reason || 'Team Follow needs your permission to type in Warcraft III.';
  } else {
    line.dataset.state = 'off';
    $('keyboard-note').textContent = kb.reason || 'Typing in the match chat isn’t supported on this computer yet.';
  }
  $('keyboard-allow').hidden = !(kb.supported && !kb.permitted);
}

function renderHost(s) {
  const h = s.host;
  if (!h) return;
  const toggle = $('autohost');
  const reason = h.enabled ? '' : hostBlocked(s);
  toggle.checked = h.enabled;
  toggle.disabled = !h.enabled && Boolean(reason);
  $('host-help').textContent = hostSummary(h.settings);
  $('host-setup').classList.toggle('needs-setup', !h.settings.mapPath || !h.settings.gameName);
  $('host-blocked').textContent = s.connected || !h.settings.mapPath ? reason : '';
  renderKeyboard(s.keyboard);
  if (confirmFor === 'Auto-host' && (h.enabled || reason)) closeConfirm();
}

$('autohost').onchange = event => {
  if (!state) return;
  if (!event.target.checked) return hostAct({enabled: false});
  confirmThen(event.target, 'Auto-host', () => hostAct({enabled: true, fromMenus: true}));
};

/* Auto-host setup dialog */

let draft = null;

function fillHostForm() {
  $('host-map-name').textContent = draft.mapName || 'No map chosen';
  $('host-map-path').textContent = draft.mapPath || '';
  document.querySelector('.map-pick').dataset.empty = String(!draft.mapPath);
  $('host-name').value = draft.gameName || '';
  updateNameCount();
  for (const radio of document.querySelectorAll('input[name="host-private"]')) radio.checked = (radio.value === 'private') === Boolean(draft.private);
  $('host-delay').value = String(draft.delaySeconds ?? 10);
  if (!$('host-delay').value) $('host-delay').value = '10';
  $('host-visibility').value = String(draft.visibility ?? 0);
  $('host-observers').value = String(draft.observers ?? 0);
  $('host-start-full').checked = Boolean(draft.startWhenFull);
  $('host-same-team').checked = Boolean(draft.sameTeam);
  $('host-invite').checked = Boolean(draft.inviteGroup);
  $('game-mode').value = draft.gameMode || '';
  $('game-mode-delay').value = String(draft.gameModeDelaySeconds ?? 5);
  updateGameModeCount();
  $('host-error').textContent = '';
}

function updateNameCount() {
  $('host-name-count').textContent = `${[...$('host-name').value].length}/31`;
}

function updateGameModeCount() {
  const n = [...$('game-mode').value.trim()].length;
  $('game-mode-count').textContent = n ? `${n}/100` : 'Off';
}

// gameModeDelay reads Seconds after loading, kept within 0 and the field's
// max (dto.MaxGameModeDelaySeconds, checked by cmd/gui/frontend_test.go).
function gameModeDelay() {
  const field = $('game-mode-delay');
  const secs = Math.round(Number(field.value));
  return Number.isFinite(secs) ? Math.min(Number(field.max), Math.max(0, secs)) : 5;
}

$('host-setup').onclick = () => {
  if (!state) return;
  draft = {delaySeconds: 10, gameModeDelaySeconds: 5, ...state.host.settings};
  fillHostForm();
  $('host-dialog').showModal();
};
$('host-name').oninput = updateNameCount;
$('game-mode').oninput = updateGameModeCount;
$('keyboard-allow').onclick = async () => {
  $('host-error').textContent = '';
  try {
    render(await bridge.RequestKeyboardPermission());
  } catch (err) {
    $('host-error').textContent = String(err?.message ?? err);
  }
};
$('host-cancel').onclick = () => $('host-dialog').close();
$('host-choose-map').onclick = () => openMapBrowser(entry => {
  draft.mapPath = entry.path;
  draft.mapName = entry.name;
  $('host-map-name').textContent = entry.name;
  $('host-map-path').textContent = entry.path;
  document.querySelector('.map-pick').dataset.empty = 'false';
});

$('host-form').onsubmit = async event => {
  event.preventDefault();
  const settings = {
    ...draft,
    gameName: $('host-name').value.trim(),
    private: document.querySelector('input[name="host-private"]:checked').value === 'private',
    delaySeconds: Number($('host-delay').value),
    visibility: Number($('host-visibility').value),
    observers: Number($('host-observers').value),
    startWhenFull: $('host-start-full').checked,
    sameTeam: $('host-same-team').checked,
    inviteGroup: $('host-invite').checked,
    gameMode: $('game-mode').value.trim(),
    gameModeDelaySeconds: gameModeDelay(),
  };
  try {
    render(await bridge.ConfigureHost({settings}));
    $('host-dialog').close();
  } catch (err) {
    $('host-error').textContent = String(err?.message ?? err);
  }
};

/* Map browser */

const browser = {folders: [], dir: '', query: '', listing: null, sort: {key: 'name', dir: 1}, selected: null, onChoose: null, timer: 0};
const collator = new Intl.Collator(undefined, {numeric: true, sensitivity: 'base'});

async function openMapBrowser(onChoose) {
  browser.onChoose = onChoose;
  browser.selected = null;
  browser.query = '';
  $('maps-search').value = '';
  $('maps-dialog').showModal();
  await loadFolders();
  await browse('');
  $('maps-search').focus();
}

async function loadFolders() {
  try { browser.folders = await bridge.MapsFolders(); } catch { browser.folders = []; }
  const select = $('maps-folder');
  select.replaceChildren();
  if (!browser.folders.length) {
    const option = el('option', '', 'No maps folder found. Use Change folder.');
    option.value = '';
    select.append(option);
    select.disabled = true;
    return;
  }
  select.disabled = browser.folders.length < 2;
  for (const f of browser.folders) {
    const option = el('option', '', `${f.path}  (${f.source})`);
    option.value = f.path;
    select.append(option);
  }
}

async function browse(dir) {
  browser.dir = dir;
  browser.query = '';
  await loadListing(() => bridge.BrowseMaps(dir));
}

async function loadListing(call) {
  $('maps-note').textContent = 'Loading…';
  browser.error = '';
  try {
    browser.listing = await call();
  } catch (err) {
    browser.listing = {entries: []};
    browser.error = String(err?.message ?? err);
  }
  renderMaps();
}

function relPath(entry) {
  return entry.dir ? `${entry.dir}/${entry.name}` : entry.name;
}

function sortedEntries() {
  const {key, dir} = browser.sort;
  const entries = [...(browser.listing?.entries ?? [])];
  return entries.sort((a, b) => {
    if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1;
    const by = key === 'date' ? new Date(a.modified) - new Date(b.modified) : collator.compare(a.name, b.name);
    return (by || collator.compare(a.name, b.name)) * dir;
  });
}

function renderCrumbs() {
  const nav = $('maps-crumbs');
  nav.replaceChildren();
  if (browser.query) {
    nav.append(el('span', 'searching', `Results for “${browser.query}”`));
    return;
  }
  const parts = browser.dir ? browser.dir.split('/') : [];
  const crumb = (label, dir, current) => {
    const b = el('button', '', label);
    b.type = 'button';
    if (current) b.setAttribute('aria-current', 'page');
    b.onclick = () => browse(dir);
    return b;
  };
  nav.append(crumb('Maps', '', !parts.length));
  parts.forEach((part, i) => {
    nav.append(icon('chevron'), crumb(part, parts.slice(0, i + 1).join('/'), i === parts.length - 1));
  });
}

function mapRow(entry) {
  const tr = el('tr');
  tr.tabIndex = 0;
  tr.setAttribute('aria-selected', String(browser.selected?.path === entry.path));
  const name = el('td');
  const cell = el('div', 'cell-name');
  const glyph = icon(entry.isFolder ? 'folder' : 'map');
  glyph.classList.add(entry.isFolder ? 'kind-folder' : 'kind-map');
  const label = el('div');
  label.append(el('span', 'map-name', entry.name));
  if (browser.query && entry.dir) label.append(el('span', 'map-dir', entry.dir));
  cell.append(glyph, label);
  name.append(cell);
  const date = el('td', 'date', entry.modified ? new Date(entry.modified).toLocaleDateString([], {year: 'numeric', month: 'short', day: 'numeric'}) : '');
  tr.append(name, date);
  const open = () => entry.isFolder ? browse(relPath(entry)) : choose(entry);
  tr.onclick = () => {
    if (entry.isFolder) return browse(relPath(entry));
    browser.selected = entry;
    renderMaps();
  };
  tr.ondblclick = () => !entry.isFolder && choose(entry);
  tr.onkeydown = event => {
    if (event.key === 'Enter') open();
    if (event.key === 'ArrowDown') tr.nextElementSibling?.focus();
    if (event.key === 'ArrowUp') tr.previousElementSibling?.focus();
  };
  return tr;
}

function renderMaps() {
  renderCrumbs();
  for (const th of document.querySelectorAll('.maps-table th')) {
    const key = th.querySelector('button').dataset.sort;
    th.setAttribute('aria-sort', key === browser.sort.key ? (browser.sort.dir > 0 ? 'ascending' : 'descending') : 'none');
  }
  const rows = $('maps-rows');
  rows.replaceChildren();
  const entries = sortedEntries();
  if (!entries.length) {
    const tr = el('tr', 'empty-row');
    const td = el('td', '', browser.query ? `No map matches “${browser.query}”.` : 'No maps in this folder.');
    td.colSpan = 2;
    tr.append(td);
    rows.append(tr);
  }
  for (const entry of entries) rows.append(mapRow(entry));
  const maps = entries.filter(e => !e.isFolder).length;
  const folders = entries.length - maps;
  $('maps-note').textContent = browser.error || (browser.listing?.truncated
    ? `Showing the first ${maps} matches. Type more of the name to narrow it down.`
    : [maps && plural(maps, 'map', 'maps'), folders && plural(folders, 'folder', 'folders')].filter(Boolean).join(', '));
  $('maps-selected').textContent = browser.selected ? browser.selected.name : '';
  $('maps-choose').disabled = !browser.selected;
}

function choose(entry) {
  $('maps-dialog').close();
  browser.onChoose?.(entry);
}

for (const button of document.querySelectorAll('.maps-table th button')) {
  button.onclick = () => {
    const key = button.dataset.sort;
    browser.sort = {key, dir: browser.sort.key === key ? -browser.sort.dir : key === 'date' ? -1 : 1};
    renderMaps();
  };
}

$('maps-search').oninput = () => {
  clearTimeout(browser.timer);
  const query = $('maps-search').value.trim();
  browser.timer = setTimeout(() => {
    if (!query) return browse(browser.dir);
    browser.query = query;
    loadListing(() => bridge.SearchMaps(query));
  }, 250);
};

$('maps-folder').onchange = async event => {
  try {
    await bridge.SetMapsFolder(event.target.value);
  } catch (err) {
    browser.error = String(err?.message ?? err);
  }
  await loadFolders();
  await browse('');
};

$('maps-change-folder').onclick = async () => {
  try {
    const dir = await bridge.ChooseMapsFolder(browser.listing?.root ?? '');
    if (!dir) return;
  } catch (err) {
    $('maps-note').textContent = String(err?.message ?? err);
    return;
  }
  await loadFolders();
  await browse('');
};

$('maps-cancel').onclick = () => $('maps-dialog').close();
$('maps-choose').onclick = () => browser.selected && choose(browser.selected);

/* Updates ---------------------------------------------------------------
   The banner and the Updates section (docs/adr/0009). Changelog text comes
   from GitHub, so it is only ever set with textContent. */

const longDate = iso => {
  const d = iso ? new Date(iso.length === 10 ? iso + 'T12:00:00' : iso) : null;
  return d && !Number.isNaN(d.getTime()) ? d.toLocaleDateString([], {day: 'numeric', month: 'long', year: 'numeric'}) : '';
};

// featureNames are the bold feature names the changes introduce, newest
// first, for the one-line summary.
function featureNames(changes) {
  const names = [];
  for (const section of changes) {
    for (const group of section.groups) {
      for (const entry of group.entries) {
        const first = entry.spans[0];
        if (first?.bold) names.push(first.text.replace(/[.:]\s*$/, ''));
      }
    }
  }
  return [...new Set(names)];
}

function teaser(changes) {
  const names = featureNames(changes);
  if (!names.length) return '';
  if (names.length <= 3) return names.join(', ');
  return `${names.slice(0, 3).join(', ')} and ${names.length - 3} more`;
}

function changeEntry(entry) {
  const li = el('li');
  for (const span of entry.spans) {
    li.append(span.code ? el('code', '', span.text) : span.bold ? el('strong', '', span.text) : document.createTextNode(span.text));
  }
  return li;
}

// changeSection is one version's entries, folded or open; current marks
// the version this computer runs.
function changeSection(section, open, current = false) {
  const box = el('details', 'change');
  box.open = open;
  const summary = el('summary');
  summary.append(icon('chevron'), el('span', 'change-version', section.version));
  if (section.date) {
    const time = el('time', 'change-date', longDate(section.date));
    time.dateTime = section.date;
    summary.append(time);
  }
  if (current) {
    box.classList.add('change-current');
    summary.append(el('span', 'change-badge', 'Your version'));
  }
  box.append(summary);
  for (const group of section.groups) {
    const block = el('div', 'change-group');
    block.dataset.group = group.name.toLowerCase();
    if (group.name) block.append(el('h4', '', group.name));
    const list = el('ul');
    for (const entry of group.entries) list.append(changeEntry(entry));
    block.append(list);
    box.append(block);
  }
  return box;
}

let changesSignature = '';

function renderChanges(release) {
  const changes = release.changes ?? [];
  const signature = JSON.stringify([release.version, changes, release.notes]);
  if (signature === changesSignature) return;
  changesSignature = signature;
  const box = $('update-changes');
  box.replaceChildren();
  if (changes.length) {
    // The newest version is open; older ones fold away.
    changes.forEach((section, i) => box.append(changeSection(section, i === 0)));
  } else {
    box.append(el('p', 'release-notes', release.notes?.trim() || 'This release has no notes.'));
  }
  $('update-notes-label').textContent = changes.length > 1 ? `What’s new in ${changes.length} versions` : 'What’s new';
  $('update-teaser').textContent = teaser(changes);
}

// The install's steps, after the player clicked Update (docs/adr/0009).
const installing = ['downloading', 'verifying', 'installing', 'restarting'];

function updateStatus(u) {
  if (!u.supported) return u.message;
  if (u.phase === 'checking') return 'Checking GitHub for a newer version…';
  if (u.release && installing.includes(u.phase)) return `Installing Team Follow ${u.release.version}…`;
  if (u.release && u.phase === 'failed') return `Team Follow ${u.release.version} couldn’t be installed.`;
  if (u.release) {
    const v = `Team Follow ${u.release.version} is available.`;
    return u.skipped ? `${v} You skipped this version.` : v;
  }
  if (u.message) return u.message;
  if (u.lastChecked) return `Last checked at ${new Date(u.lastChecked).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})}.`;
  return u.checkAutomatically ? 'Not checked yet.' : 'Automatic checks are off.';
}

function renderUpdate(s) {
  const u = s.update;
  if (!u) return;
  const banner = $('update-banner');
  const show = Boolean(u.prompt && u.release);
  banner.hidden = !show;
  if (show) renderBanner(u);
  renderAfterUpdate(u);

  $('updates-version').textContent = u.currentVersion ? `Version ${u.currentVersion}` : '';
  const status = $('updates-status');
  status.textContent = updateStatus(u);
  status.dataset.tone = !u.supported ? 'off' : u.phase === 'failed' ? 'problem' : u.release ? 'available' : /couldn’t|couldn't|limiting/.test(u.message) ? 'problem' : '';
  const check = $('updates-check');
  check.disabled = !u.supported || u.phase === 'checking' || installing.includes(u.phase);
  check.querySelector('span').textContent = u.phase === 'checking' ? 'Checking…' : 'Check for updates';
  // While the banner is hidden (Later or Skip), the offer stays reachable here.
  $('updates-go').hidden = !u.release || show;
  if (u.release) $('updates-go').textContent = `Update to ${u.release.version}`;
  const auto = $('updates-auto');
  auto.checked = Boolean(u.supported && u.checkAutomatically);
  auto.disabled = !u.supported;
  renderWhatsNew(u);
}

/* What's new: on the first start after an update, the changelog sections
   since the version the player ran before, from the changelog built into
   the app. The Updates section reopens it. */

let whatsNewReopened = false;
let whatsNewSignature = '';
let runningVersion = '';

function renderWhatsNew(u) {
  runningVersion = u.supported ? u.currentVersion : '';
  const w = u.whatsNew;
  const link = $('whats-new-open');
  link.hidden = !w;
  const card = $('whats-new');
  card.hidden = !w || !(w.show || whatsNewReopened);
  if (!w) return;
  link.querySelector('span').textContent = `What’s new in ${w.version}`;
  $('whats-new-title').textContent = `What’s new in Team Follow ${w.version}`;
  const versions = w.changes.length;
  $('whats-new-sub').textContent = w.since
    ? `You updated from ${w.since}.${versions > 1 ? ` Here’s what changed in ${versions} versions.` : ''}`
    : 'Here’s what changed in this version.';
  const signature = JSON.stringify(w.changes);
  if (signature !== whatsNewSignature) {
    whatsNewSignature = signature;
    const box = $('whats-new-changes');
    box.replaceChildren();
    // The newest version is open; older ones fold away.
    w.changes.forEach((section, i) => box.append(changeSection(section, i === 0)));
  }
}

$('whats-new-close').onclick = async () => {
  whatsNewReopened = false;
  $('whats-new').hidden = true;
  if (state?.update?.whatsNew?.show) await act(() => bridge.DismissWhatsNew());
};
$('whats-new-open').onclick = () => {
  whatsNewReopened = true;
  if (state) renderWhatsNew(state.update);
  const card = $('whats-new');
  const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  card.scrollIntoView({block: 'start', behavior: still ? 'auto' : 'smooth'});
  card.focus({preventScroll: true});
};

/* Changelog: every version built into the app, newest first, read once
   when the player first opens it. */

let changelogLoaded = false;

async function loadChangelog() {
  if (changelogLoaded) return;
  changelogLoaded = true;
  const list = $('changelog-list');
  let sections = [];
  try { sections = (await bridge.Changelog()) ?? []; } catch { sections = []; }
  list.replaceChildren();
  if (!sections.length) {
    list.append(el('p', 'release-notes', 'The changelog isn’t available in this build.'));
    return;
  }
  $('changelog-count').textContent = `${sections.length} versions`;
  // The running version is open; without it, the newest.
  const openVersion = sections.some(s => s.version === runningVersion) ? runningVersion : sections[0].version;
  for (const section of sections) list.append(changeSection(section, section.version === openVersion, section.version === runningVersion));
}

$('changelog').ontoggle = e => { if (e.target.open) loadChangelog(); };

const megabytes = n => `${(n / 1048576).toFixed(1)} MB`;

function downloadLine(p) {
  if (!p || !p.done) return 'Starting the download…';
  return p.total ? `${megabytes(p.done)} of ${megabytes(p.total)}` : `${megabytes(p.done)} downloaded`;
}

// renderBanner fills the banner for the offer, the install's progress or
// its failure. Only the step's own buttons show.
function renderBanner(u) {
  const rel = u.release;
  const phase = u.phase;
  const busy = installing.includes(phase);
  const failed = phase === 'failed';
  $('update-banner').dataset.phase = busy || failed ? phase : 'available';

  const titles = {
    downloading: `Downloading Team Follow ${rel.version}`,
    verifying: `Checking Team Follow ${rel.version}`,
    installing: `Installing Team Follow ${rel.version}`,
    restarting: `Restarting into Team Follow ${rel.version}`,
    failed: `Team Follow ${rel.version} couldn’t be installed`,
  };
  $('update-title').textContent = titles[phase] ?? `Team Follow ${rel.version} is available`;
  const released = longDate(rel.published);
  const subs = {
    downloading: 'Nothing changes until the download is complete and checked.',
    verifying: 'Comparing the download with its published SHA-256 checksum.',
    installing: 'Replacing Team Follow. This takes a moment.',
    restarting: 'Team Follow closes and opens again in the new version.',
    failed: '',
  };
  $('update-sub').textContent = subs[phase] ??
    `You have ${u.currentVersion}.${released ? ` Released ${released}.` : ''} Update downloads it, checks it and restarts Team Follow.`;

  const go = $('update-go');
  go.hidden = busy;
  go.querySelector('span').textContent = failed ? 'Try again' : 'Update';
  $('update-cancel').hidden = !(phase === 'downloading' || phase === 'verifying');
  $('update-releases').hidden = !failed;
  $('update-later').hidden = busy;
  $('update-skip').hidden = busy || failed;

  $('update-progress').hidden = !busy;
  if (busy) {
    const bar = $('update-bar');
    const p = u.progress;
    const pct = phase === 'downloading' && p?.total ? Math.min(100, Math.round((p.done / p.total) * 100)) : null;
    bar.toggleAttribute('data-busy', pct === null);
    if (pct === null) bar.removeAttribute('aria-valuenow'); else bar.setAttribute('aria-valuenow', String(pct));
    bar.style.setProperty('--done', `${pct ?? 0}%`);
    $('update-steps').textContent = phase === 'downloading' ? downloadLine(p) : phase === 'verifying' ? 'Download complete.' : '';
  }

  $('update-problem').hidden = !failed;
  $('update-problem-text').textContent = failed ? u.problem : '';
  const note = !failed && !busy && u.installNote;
  $('update-install-note').hidden = !note;
  $('update-install-note-text').textContent = note || '';
  const warn = !failed && phase !== 'restarting' && u.warning;
  $('update-warning').hidden = !warn;
  $('update-warning-text').textContent = warn || '';
  $('update-notes').hidden = busy || failed;
  renderChanges(rel);
}

// renderAfterUpdate shows the step the new version needs: the Linux setcap
// command with Copy, or the macOS Accessibility permission for Game mode.
function renderAfterUpdate(u) {
  const after = u.after;
  $('update-after').hidden = !after;
  if (!after) return;
  $('update-after-title').textContent = after.setcap ? 'Allow Team Follow to read the game again' : 'Game mode may need Accessibility again';
  $('update-after-sub').textContent = after.setcap
    ? `Team Follow ${after.version} replaced its program, and Linux doesn’t carry the permission over. Run this once in a terminal, then restart Team Follow.`
    : `Team Follow ${after.version} is a new app to macOS. If Game mode stops typing, allow Team Follow again in System Settings, Privacy & Security, Accessibility.`;
  $('update-after-command').hidden = !after.setcap;
  $('update-after-command-text').textContent = after.setcap || '';
}

async function openUpdate() {
  $('error').textContent = '';
  try { render(await bridge.Update()); } catch (err) { $('error').textContent = String(err?.message ?? err); }
}

$('update-go').onclick = openUpdate;
$('updates-go').onclick = openUpdate;
$('update-cancel').onclick = () => act(async () => bridge.CancelUpdate());
$('update-releases').onclick = async () => {
  $('error').textContent = '';
  try { await bridge.OpenReleasesPage(); } catch (err) { $('error').textContent = String(err?.message ?? err); }
};
$('update-after-done').onclick = () => act(async () => bridge.DismissUpdateNotes());
$('update-after-copy').onclick = async () => {
  await copy($('update-after-command-text').textContent);
  const text = $('update-after-copy').querySelector('span');
  text.textContent = 'Copied';
  setTimeout(() => { text.textContent = 'Copy'; }, 1500);
};
$('update-later').onclick = () => act(() => bridge.UpdateLater());
$('update-skip').onclick = () => act(() => bridge.SkipUpdate());
$('updates-check').onclick = () => act(() => bridge.CheckForUpdates());
$('updates-auto').onchange = e => act(() => bridge.SetCheckUpdates(e.target.checked));
$('updates-releases').onclick = async () => {
  $('error').textContent = '';
  try { await bridge.OpenReleasesPage(); } catch (err) { $('error').textContent = String(err?.message ?? err); }
};

/* Demo bridge: used only when the page runs outside the app, to preview
   the design in a browser. Pick a scene with ?scene=watching|first|live|
   paused|password|waiting|many|linux|unverified|host-waiting|host-lobby|
   game-mode|keyboard-ask|keyboard-linux|recent|recent-empty|recent-clear|
   recent-select|recent-select-confirm|blacklist|blacklist-empty|blacklist-off|
   update|update-open|update-warning|update-notes|update-later|update-skipped|
   update-latest|update-offline|update-dev|update-downloading|update-verifying|
   update-installing|update-failed|update-translocated|update-linux|update-after-linux|
   update-after-mac|whats-new|whats-new-older|
   changelog|changelog-dev|
   menu-recent|menu-blacklist|menu-group|notes|notes-search|menu-note|
   note-add|note-edit|note-long|note-view;
   add &open=host-setup to see the Game mode group. */
function demoBridge() {
  const scene = new URLSearchParams(location.search).get('scene') || 'watching';
  const now = Date.now();
  const at = mins => new Date(now - mins * 60000).toISOString();
  const roster = [
    ['Arthas#1123', 'Lobby open', true, true, false, 'Footmen Frenzy 6.2'],
    ['Sylvanas#9031', 'In Warcraft III', true, false, false],
    ['Jaina#2214', 'Online in Battle.net', false, false, false],
    ['Thrall#1820', 'Offline', false, false, false, '', false],
    ['Uther#5521', 'In Warcraft III', true, false, false],
    ['Kaelthas#7710', 'Away', false, false, true],
    ['Tyrande#3348', 'Online in Battle.net', false, false, false],
    ['Illidan#6602', 'Lobby open', true, true, false, 'Legion TD x20'],
    ['Muradin#1290', 'Offline', false, false, false, '', false],
    ['Rexxar#4471', 'Offline', false, false, false, '', false],
    ['Grom#8899', 'Away', false, false, true],
    ['Malfurion#2034', 'Offline', false, false, false, '', false],
  ].map(([battleTag, label, inGame, open, away, lobby = '', online = true]) => ({battleTag, label, inGame, open, away, lobby, online, selected: false}));
  const find = tag => roster.find(r => key(r.battleTag) === key(tag));
  const hostDemo = {mapPath: '/Maps/Download/Legion TD x20.w3x', mapName: 'Legion TD x20', gameName: 'LTD with friends', private: false, visibility: 0, observers: 0, delaySeconds: 10, startWhenFull: false, sameTeam: false, inviteGroup: true, gameMode: '', gameModeDelaySeconds: 5};
  const mapsRoot = '/Users/player/Library/Application Support/Blizzard/Warcraft III/Maps';
  const day = d => new Date(Date.UTC(2026, 0, 1) + d * 86400000).toISOString();
  const tree = {
    '': [['Download', true, 300], ['FrozenThrone', true, 10], ['(2)Lost Temple', false, 5], ['(4)Twisted Meadows', false, 40], ['Legion TD x20', false, 200]],
    'Download': [['Tower Defense', true, 280], ['Green Circle TD 2020', false, 120], ['Footmen Frenzy 6.2', false, 250], ['Island Defense 4.1', false, 90], ['(8)Vanguard Point', false, 23]],
    'Download/Tower Defense': [['Legion TD Mega 4.2', false, 290], ['Element TD 5.5', false, 160], ['Wintermaul Wars', false, 60]],
    'FrozenThrone': [['(2)Echo Isles', false, 1], ['(4)Turtle Rock', false, 1]],
  };
  const entries = dir => (tree[dir] ?? []).map(([name, isFolder, d]) => ({name, isFolder, dir, modified: day(d), path: `${mapsRoot}/${dir ? dir + '/' : ''}${name}${isFolder ? '' : '.w3x'}`}));
  // Recent players: sample BattleTags only. Medivh and Varimathras are in
  // your lobby now; the rest are friends from the roster or strangers.
  const recent = [
    ['Medivh#3310', 'Castle Fight', 0],
    ['Varimathras#2207', 'Castle Fight', 0],
    ['Arthas#1123', 'Castle Fight', 3],
    ['Garrosh#7781', 'Castle Fight', 9],
    ['Kelthuzad#6644', 'Legion TD x20', 26],
    ['Jaina#2214', 'Legion TD x20', 26],
    ['Anubarak#5102', 'Footmen Frenzy 6.2', 190],
    ['Rexxar#4471', 'Island Defense 4.1', 60 * 26],
    ['Alleria#4415', 'Green Circle TD 2020', 60 * 24 * 3],
    ['Zuljin#3087', 'Legion TD Mega 4.2', 60 * 24 * 12],
    ['Vereesa#9920', 'Element TD 5.5', 60 * 24 * 40],
    ['Rokhan#1500', '(4)Twisted Meadows', 60 * 24 * 41],
  ].map(([tag, map, mins]) => ({tag, map, lastSeen: at(mins)}));
  // Blacklist: sample BattleTags only. Garrosh is also a recent player.
  const blacklist = [
    ['Garrosh#7781', 60 * 26],
    ['Deathwing#6666', 60 * 24 * 9],
    ['Mannoroth#1404', 60 * 24 * 33],
  ].map(([tag, mins]) => ({tag, added: at(mins)}));
  // Updates: a sample changelog spanning three versions.
  const t = text => ({text});
  const b = text => ({text, bold: true});
  const c = text => ({text, code: true});
  const changes = [
    {version: '0.9.0', date: '2026-11-12', groups: [
      {name: 'Added', entries: [
        {spans: [b('Update check.'), t(' Team Follow tells you when a new version is out and shows what changed before you update.')]},
        {spans: [b('Player menu.'), t(' Right-click anyone in the People panel to follow, whisper or blacklist them.')]},
      ]},
      {name: 'Changed', entries: [{spans: [t('The Activity list keeps the last 14 lines and shows the time of each.')]}]},
      {name: 'Fixed', entries: [{spans: [t('Auto-host no longer creates a second lobby when the game is slow to answer.')]}]},
    ]},
    {version: '0.8.1', date: '2026-11-05', groups: [
      {name: 'Fixed', entries: [
        {spans: [t('Auto-join finds lobbies whose name contains '), c('[PRIVATE]'), t('.')]},
        {spans: [t('The blacklist keeps BattleTags with accents, such as Player#1234.')]},
      ]},
    ]},
    {version: '0.8.0', date: '2026-11-02', groups: [
      {name: 'Added', entries: [
        {spans: [b('Blacklist.'), t(' A third tab in the People panel keeps players out of lobbies you host.')]},
        {spans: [t('Pick several players at once in Recent players with '), b('Select to blacklist'), t('.')]},
      ]},
    ]},
  ];
  // The changelog built into the app: the sample above, then older ones.
  const history = [...changes,
    {version: '0.7.0', date: '2026-10-28', groups: [
      {name: 'Added', entries: [
        {spans: [b('Recent players.'), t(' The People panel has Friends and Recent players tabs.')]},
        {spans: [t('The Friends tab shows how many friends are online.')]},
      ]},
    ]},
    {version: '0.6.1', date: '2026-10-28', groups: [
      {name: 'Added', entries: [{spans: [t('App icon for the Windows, macOS and Linux builds.')]}]},
    ]},
    {version: '0.6.0', date: '2026-10-28', groups: [
      {name: 'Added', entries: [
        {spans: [b('Game mode.'), t(' Auto-host can type a command such as '), c('-ar'), t(' in the match chat a few seconds after the match starts.')]},
        {spans: [b('Join the host’s team'), t(' (Auto-join): moves you into an open slot on the host’s team.')]},
      ]},
      {name: 'Changed', entries: [{spans: [t('The window is simpler: the Your team card is gone.')]}]},
    ]},
  ];
  const page = 'https://github.com/mihaiflorentin/wc3-team-follow/releases';
  const offer = {version: '0.9.0', url: `${page}/tag/v0.9.0`, published: '2026-11-12T10:00:00Z', changes, notes: ''};
  const updateIdle = {supported: true, phase: 'idle', currentVersion: '0.7.0', checkAutomatically: true, release: null, prompt: false,
    skipped: false, warning: '', message: '', lastChecked: at(40), releasesUrl: page,
    whatsNew: {version: '0.7.0', since: '', changes: history.slice(3, 4), show: false}};
  // After updating from 0.7.0 to 0.9.0.
  const updated = {...updateIdle, currentVersion: '0.9.0', whatsNew: {version: '0.9.0', since: '0.7.0', changes, show: true}};
  const updateOffer = {...updateIdle, phase: 'available', release: offer, prompt: true, progress: null, problem: '', installNote: '', after: null};
  const macNote = 'After updating, macOS may ask you again to allow Team Follow under Accessibility for Game mode.';
  // Notes: sample BattleTags only, keyed lower-case as the Go side keeps them.
  const notes = Object.fromEntries([
    ['Arthas#1123', 'Carries in Legion TD. Calls the waves early and builds Elves.', 60 * 5],
    ['Jaina#2214', 'Friend of Thrall’s from the Sunday group.\nPlays every race; asks before picking random.\n\nGood at Footmen Frenzy, weaker at tower defense. Prefers voice chat on Discord and is usually on after 8 pm. Happy to host when the lobby fills slowly.', 60 * 24 * 2],
    ['Garrosh#7781', 'Left three Castle Fight games in a row when losing.', 60 * 26],
    ['Kaelthas#7710', 'Met in a Legion TD lobby. Plays well, quiet in chat.', 60 * 24 * 20],
  ].map(([tag, text, mins]) => [key(tag), {text, updated: at(mins)}]));
  const s = {
    update: scene.startsWith('update') ? updateOffer : updateIdle,
    notes,
    recent, lobbyTags: ['Medivh#3310', 'Varimathras#2207'],
    blacklist, autoBan: true, blacklistManualBans: true,
    connected: true, version: '3.0.1.24371', verifiedBuild: true, enabled: false, diagnostic: false,
    manualTags: ['Arthas#1123', 'Jaina#2214', 'Thrall#1820', 'Uther#5521'], includeTeam: true, joinHostTeam: true,
    status: '', screen: 'CUSTOM_LOBBIES', busy: false, queued: false, inMatch: false, paused: false, inMenus: true,
    joinPending: false, passwordRequest: null, preferencesError: '', connectionHint: '', gameCount: 0,
    team: [
      {battleTag: 'Player#1234', isSelf: true, status: 'You'},
      {battleTag: 'Sylvanas#9031', isFriend: true, status: 'In Warcraft III'},
    ],
    watched: [], friends: [], roster, lastJoin: null, mapsFolder: '',
    keyboard: {supported: true, permitted: true, reason: ''},
    host: {enabled: false, phase: 'off', settings: hostDemo, players: 0, slots: 0, missing: [], offTeam: []},
    log: [
      {time: at(12), message: 'Connected to Warcraft III. Form a team in the game, or add friends here.'},
      {time: at(9), message: 'Illidan#6602 opened a lobby: Legion TD x20.'},
      {time: at(5), message: 'Auto-join is on. It pauses in lobbies, matches and queues.'},
      {time: at(4), message: 'Illidan#6602 opened a lobby: Legion TD x20.'},
      {time: at(4), message: 'Asked to join Illidan#6602’s lobby. Follow stays on.'},
      {time: at(4), message: 'Joined Illidan#6602’s lobby. Follow stays on and is paused here.'},
      {time: at(1), message: 'Back in the game’s menus. Follow has resumed.'},
    ],
  };
  const watch = () => {
    const tags = [...(s.includeTeam ? s.team.filter(m => !m.isSelf).map(m => m.battleTag) : []), ...s.manualTags];
    s.watched = tags.map(tag => {
      const r = find(tag);
      return {battleTag: tag, label: r?.label ?? 'Waiting for activity', open: Boolean(r?.open), lobby: r?.lobby ?? ''};
    });
  };
  const scenes = {
    'host-waiting': {host: {enabled: true, phase: 'waiting', createAt: new Date(now + 8000).toISOString(), settings: hostDemo, players: 0, slots: 0, missing: [], offTeam: []}},
    'host-lobby': {host: {enabled: true, phase: 'lobby', settings: {...hostDemo, startWhenFull: true, sameTeam: true}, players: 3, slots: 6, full: false, missing: ['Jaina#2214', 'Thrall#1820'], offTeam: ['Uther#5521']},
      log: [{time: at(1), message: 'Your lobby “LTD with friends” is open. It starts once it’s full and everyone you follow is in.'}, {time: at(0), message: 'Moving Uther#5521 to your team.'}]},
    'host-swap': {host: {enabled: true, phase: 'lobby', settings: {...hostDemo, startWhenFull: true, sameTeam: true}, players: 6, slots: 6, full: true, missing: [], offTeam: ['Uther#5521']},
      log: [{time: at(1), message: 'The lobby is full, but it won’t start until Uther#5521 is on your team. You can still press Start yourself.'}, {time: at(0), message: 'Asked the game to swap Uther#5521 with Player#5678.'}]},
    'join-team': {enabled: true, paused: true, busy: true, inMenus: false, status: 'Moving you to Illidan#6602’s team.',
      log: [{time: at(1), message: 'Joined Illidan#6602’s lobby. Follow stays on and is paused here.'}, {time: at(0), message: 'Moving you to Illidan#6602’s team.'}]},
    'game-mode': {host: {enabled: true, phase: 'match', settings: {...hostDemo, startWhenFull: true, gameMode: '-ar'}, players: 6, slots: 6, full: true, missing: [], offTeam: []},
      busy: true, inMatch: true, inMenus: false, screen: 'DISABLED_SCREEN', status: 'Typed -ar in the game chat.',
      log: [{time: at(2), message: 'The lobby is full and everyone’s in. Starting the game.'}, {time: at(1), message: 'The match started. Game mode types -ar in 5 seconds; keep Warcraft III in front.'}, {time: at(1), message: 'Typed -ar in the game chat.'}]},
    'keyboard-ask': {host: {enabled: false, phase: 'off', settings: {...hostDemo, gameMode: '-ar'}, players: 0, slots: 0, missing: [], offTeam: []},
      keyboard: {supported: true, permitted: false, reason: 'Team Follow needs permission to send keystrokes. Allow it in System Settings, Privacy & Security, Accessibility, then try again.'}},
    'keyboard-linux': {host: {enabled: false, phase: 'off', settings: {...hostDemo, gameMode: '-ar', gameModeDelaySeconds: 0}, players: 0, slots: 0, missing: [], offTeam: []},
      keyboard: {supported: false, permitted: false, reason: 'Typing in the match chat isn’t supported on Linux yet.'}},
    watching: {log: [{time: at(3), message: 'Connected to Warcraft III. Form a team in the game, or add friends here.'}, {time: at(1), message: 'Arthas#1123 opened a lobby: Footmen Frenzy 6.2.'}]},
    first: {manualTags: [], team: [{battleTag: 'Player#1234', isSelf: true, status: 'You'}], log: s.log.slice(0, 1)},
    live: {enabled: true},
    paused: {enabled: true, paused: true, busy: true, inMatch: true, inMenus: false, status: 'Follow is on but paused during the match.'},
    password: {enabled: true, passwordRequest: {lobby: 'Pros only', friend: 'Arthas#1123'}},
    waiting: {connected: false, team: [], roster: [], connectionHint: 'Warcraft III isn’t running. Start it through Battle.net.'},
    many: {connected: false, gameCount: 2, team: [], roster: [], connectionHint: 'Warcraft III is running 2 times. Close the ones you aren’t playing, or use Close all Warcraft III; a copy you restarted may still run in the background. Team Follow connects by itself once one is left.'},
    linux: {connected: false, team: [], roster: [], connectionHint: 'This computer blocked reading Warcraft III’s menu session. To allow it, run: sudo setcap cap_sys_ptrace=ep <path to wc3-team-follow>'},
    unverified: {version: '3.0.2.25010', verifiedBuild: false},
    recent: {manualTags: ['Arthas#1123', 'Jaina#2214', 'Kelthuzad#6644'], busy: true, inMenus: false, screen: 'GAME_LOBBY'},
    'recent-empty': {recent: [], lobbyTags: []},
    'recent-clear': {},
    'recent-select': {manualTags: ['Arthas#1123', 'Jaina#2214']},
    'recent-select-confirm': {manualTags: ['Arthas#1123', 'Jaina#2214']},
    blacklist: {log: [...s.log.slice(0, 2), {time: at(2), message: 'Your lobby “LTD with friends” is open. Press Start in Warcraft III when everyone’s in.'},
      {time: at(1), message: 'Banned Garrosh#7781 (on your blacklist).'}, {time: at(0), message: 'Added Mannoroth#1404 to your blacklist (you banned them).'}]},
    'blacklist-empty': {blacklist: []},
    'blacklist-off': {autoBan: false},
    update: {},
    'update-warning': {enabled: true, paused: true, busy: true, inMatch: true, inMenus: false, status: 'Follow is on but paused during the match.',
      update: {...updateOffer, warning: 'You’re in a lobby or a match. Updating restarts Team Follow and turns Follow off.'}},
    'update-notes': {update: {...updateOffer, release: {...offer, version: '0.7.1', changes: [],
      notes: 'What’s Changed\n• Fix Auto-join after signing in again\n• Faster map search\n\nFull Changelog: v0.7.0...v0.7.1'}}},
    'update-later': {update: {...updateOffer, prompt: false}},
    'update-skipped': {update: {...updateOffer, prompt: false, skipped: true}},
    'update-latest': {update: {...updateIdle, message: 'Team Follow is up to date.'}},
    'update-offline': {update: {...updateIdle, checkAutomatically: false, lastChecked: null,
      message: 'Team Follow couldn’t check for updates. Check your internet connection and try again.'}},
    'update-dev': {update: {...updateIdle, supported: false, currentVersion: 'v0.7.0-3-gabc1234-dirty', lastChecked: null,
      message: 'Updates are off in development builds. Download releases from GitHub.', whatsNew: null}},
    'update-downloading': {update: {...updateOffer, phase: 'downloading', progress: {done: 5662310, total: 13212057}}},
    'update-verifying': {update: {...updateOffer, phase: 'verifying', progress: {done: 13212057, total: 13212057}}},
    'update-installing': {update: {...updateOffer, phase: 'installing'}},
    'update-failed': {update: {...updateOffer, phase: 'failed',
      problem: 'The download didn’t match its published checksum, so nothing was installed. Team Follow 0.7.0 keeps running. You can also download the new version from the releases page.'}},
    'update-translocated': {update: {...updateOffer, phase: 'failed', installNote: macNote,
      problem: 'macOS is running Team Follow from a temporary copy, so it can’t update itself. Move Warcraft Team Follow to the Applications folder, open it from there and try again. Team Follow 0.7.0 keeps running. You can also download the new version from the releases page.'}},
    'update-linux': {update: {...updateOffer, installNote: 'After updating, run the setcap step again so Team Follow can read the game.'}},
    'update-after-linux': {update: {...updateIdle, currentVersion: '0.9.0', whatsNew: null,
      after: {version: '0.9.0', setcap: "sudo setcap cap_sys_ptrace=ep '/home/player/Apps/wc3-team-follow/wc3-team-follow'", accessibility: false}}},
    'update-after-mac': {update: {...updateIdle, currentVersion: '0.9.0', whatsNew: null, after: {version: '0.9.0', setcap: '', accessibility: true}}},
    'whats-new': {update: updated},
    'whats-new-older': {update: {...updated, whatsNew: {version: '0.9.0', since: '', changes: changes.slice(0, 1), show: true}}},
    changelog: {update: updated},
    'changelog-dev': {update: {...updateIdle, supported: false, currentVersion: 'v0.7.0-3-gabc1234-dirty', lastChecked: null,
      message: 'Updates are off in development builds. Download releases from GitHub.', whatsNew: null}},
    'menu-recent': {manualTags: ['Arthas#1123', 'Jaina#2214', 'Kelthuzad#6644']},
    'menu-blacklist': {},
    'menu-group': {},
    notes: {},
    'notes-search': {},
    'menu-note': {},
    'note-add': {},
    'note-edit': {},
    'note-long': {},
    'note-view': {},
  };
  Object.assign(s, scenes[scene] || {});
  if (s.enabled) watch();
  const clone = () => structuredClone(s);
  let demoInstall = 0;
  return {
    State: async () => clone(),
    ExecutablePath: async () => '/home/me/Apps/wc3-team-follow',
    Diagnostics: async () => JSON.stringify({version: s.version, connected: s.connected, platform: 'demo', logFolder: '/home/me/.config/WarcraftTeamFollow/logs'}, null, 2),
    LogFolder: async () => '/home/me/.config/WarcraftTeamFollow/logs',
    OpenLogFolder: async () => {},
    CloseGames: async () => {
      const n = s.connected ? 1 : s.gameCount;
      const status = n ? `Closed ${plural(n, 'copy', 'copies')} of Warcraft III.` : 'No copy of Warcraft III was running.';
      if (n) Object.assign(s, {connected: false, gameCount: 0, enabled: false, team: [], roster: [], watched: [], connectionHint: '', host: {...s.host, enabled: false, phase: 'off'}});
      s.status = status;
      s.log.push({time: new Date().toISOString(), message: status});
      return clone();
    },
    SubmitPassword: async () => { s.passwordRequest = null; return clone(); },
    CheckForUpdates: async () => {
      await new Promise(r => setTimeout(r, 700));
      s.update = {...s.update, ...updateOffer, checkAutomatically: s.update.checkAutomatically, skipped: false, lastChecked: new Date().toISOString()};
      return clone();
    },
    UpdateLater: async () => { s.update = {...s.update, prompt: false}; return clone(); },
    DismissWhatsNew: async () => {
      if (s.update.whatsNew) s.update = {...s.update, whatsNew: {...s.update.whatsNew, show: false}};
      return clone();
    },
    Changelog: async () => structuredClone(history),
    SkipUpdate: async () => { s.update = {...s.update, prompt: false, skipped: true}; return clone(); },
    SetCheckUpdates: async on => { s.update = {...s.update, checkAutomatically: on}; return clone(); },
    // The demo walks through the install's steps, then restarts; Cancel
    // stops it.
    Update: async () => {
      const total = 13212057;
      let done = 0;
      s.update = {...s.update, phase: 'downloading', prompt: true, problem: '', progress: {done, total}};
      clearInterval(demoInstall);
      demoInstall = setInterval(() => {
        done = Math.min(total, done + 1400000);
        const u = s.update;
        if (u.phase === 'downloading') s.update = done < total ? {...u, progress: {done, total}} : {...u, phase: 'verifying', progress: {done, total}};
        else if (u.phase === 'verifying') s.update = {...u, phase: 'installing', progress: null};
        else if (u.phase === 'installing') s.update = {...u, phase: 'restarting'};
        else clearInterval(demoInstall);
        render(clone());
      }, 450);
      return clone();
    },
    CancelUpdate: async () => { clearInterval(demoInstall); s.update = {...s.update, phase: 'available', progress: null}; return clone(); },
    DismissUpdateNotes: async () => { s.update = {...s.update, after: null}; return clone(); },
    OpenReleasesPage: async () => {},
    ClearRecentPlayers: async () => { s.recent = []; s.lobbyTags = []; return clone(); },
    SetNote: async (tag, text) => {
      text = text.trim();
      if ([...text].length > NOTE_MAX) throw new Error(`A note can be at most ${NOTE_MAX} characters.`);
      if (text) s.notes[key(tag)] = {text, updated: new Date().toISOString()};
      else delete s.notes[key(tag)];
      return clone();
    },
    DeleteNote: async tag => { delete s.notes[key(tag)]; return clone(); },
    AddToBlacklist: async tags => {
      for (const tag of tags) {
        if (!TAG_PATTERN.test(tag.trim())) throw new Error('Enter a full BattleTag, such as Player#1234.');
        if (s.team.some(m => m.isSelf && key(m.battleTag) === key(tag))) throw new Error('You can’t blacklist yourself.');
      }
      const added = tags.filter(t => !isBlacklisted(s, t));
      s.blacklist = [...added.map(tag => ({tag: tag.trim(), added: new Date().toISOString()})), ...s.blacklist];
      s.manualTags = s.manualTags.filter(t => !isBlacklisted(s, t));
      if (added.length) s.log.push({time: new Date().toISOString(), message: added.length === 1 ? `Added ${added[0]} to your blacklist.` : `Added ${added.length} players to your blacklist.`});
      return clone();
    },
    RemoveFromBlacklist: async tag => { s.blacklist = s.blacklist.filter(b => key(b.tag) !== key(tag)); return clone(); },
    SetAutoBan: async on => { s.autoBan = on; return clone(); },
    SetBlacklistManualBans: async on => { s.blacklistManualBans = on; return clone(); },
    RequestKeyboardPermission: async () => { s.keyboard = {supported: true, permitted: true, reason: ''}; return clone(); },
    ConfigureHost: async cfg => {
      if (cfg.settings) s.host.settings = cfg.settings;
      if (cfg.enabled === false) s.host = {...s.host, enabled: false, phase: 'off'};
      if (cfg.enabled === true) { s.enabled = false; s.host = {...s.host, enabled: true, phase: 'waiting', createAt: new Date(Date.now() + s.host.settings.delaySeconds * 1000).toISOString()}; }
      return clone();
    },
    MapsFolders: async () => [{path: mapsRoot, source: 'Warcraft III’s maps folder'}],
    SetMapsFolder: async () => clone(),
    ChooseMapsFolder: async () => '',
    BrowseMaps: async dir => ({root: mapsRoot, dir, entries: entries(dir)}),
    SearchMaps: async query => {
      const words = query.toLowerCase().split(/\s+/).filter(Boolean);
      const all = Object.keys(tree).flatMap(entries).filter(e => !e.isFolder && words.every(w => e.name.toLowerCase().includes(w)));
      return {root: mapsRoot, query, entries: all};
    },
    Configure: async cfg => {
      if (cfg.tags) { s.manualTags = cfg.tags; s.enabled = false; }
      if ('includeTeam' in cfg) { s.includeTeam = cfg.includeTeam; s.enabled = false; }
      if ('askPassword' in cfg) s.askPassword = cfg.askPassword;
      if ('joinHostTeam' in cfg) s.joinHostTeam = cfg.joinHostTeam;
      if ('diagnostic' in cfg && cfg.diagnostic !== s.diagnostic) { s.diagnostic = cfg.diagnostic; s.enabled = false; }
      if (cfg.enabled === false) s.enabled = false;
      if (cfg.enabled === true) { s.enabled = true; s.diagnostic = false; s.host = {...s.host, enabled: false, phase: 'off'}; watch(); }
      return clone();
    },
  };
}
