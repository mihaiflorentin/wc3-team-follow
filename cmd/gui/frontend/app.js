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

// nameCell is a row's BattleTag, with the Blacklisted badge when it is on
// the blacklist.
function nameCell(tag, banned) {
  const wrap = el('span', 'name-wrap');
  const name = el('span', 'name', tag);
  name.title = tag;
  wrap.append(name);
  if (banned) {
    const badge = el('span', 'ban-badge');
    badge.title = 'Blacklisted';
    badge.append(icon('ban'), el('span', 'ban-badge-text', 'Blacklisted'));
    wrap.append(badge);
  }
  return wrap;
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

function renderParty(s) {
  const list = $('party');
  list.replaceChildren();
  const members = group(s);
  for (const m of members) {
    const li = el('li');
    li.dataset.status = m.status;
    li.title = m.lobby ? `${m.tag}: lobby “${m.lobby}”` : m.tag;
    li.append(avatar(m.tag, {badge: m.status === 'open'}), el('span', 'tag', nameOf(m.tag)));
    list.append(li);
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
  box.disabled = (!f.selected && full) || banned;
  box.setAttribute('aria-label', banned ? `${f.battleTag} is blacklisted` : `Follow ${f.battleTag}`);
  label.dataset.disabled = String(box.disabled);
  box.onchange = () => {
    const tags = box.checked
      ? [...state.manualTags, f.battleTag]
      : state.manualTags.filter(t => key(t) !== key(f.battleTag));
    changeGroup({tags});
  };
  label.append(box, avatar(f.battleTag, {badge: Boolean(f.open), banned}), nameCell(f.battleTag, banned), presence(f.open ? 'Lobby open' : f.label, f.open ? f.lobby : ''));
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
  const visible = others.filter(f => (query ? key(f.battleTag).includes(query) : showOffline || f.online));
  const hiddenOffline = others.filter(f => !f.online).length;

  // Re-render only when something visible changed, so scrolling and focus
  // survive the once-a-second state updates.
  const signature = JSON.stringify([s.connected, full, chosen, visible, query, showOffline, s.roster.length, s.blacklist]);
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

const placeholders = {friends: 'Search your Battle.net friends', recent: 'Search recent players', blacklist: 'Search the blacklist'};

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
    box.disabled = banned;
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
    box.disabled = (!chosen && full) || banned;
    box.setAttribute('aria-label', banned ? `${p.tag} is blacklisted` : `Follow ${p.tag}`);
    label.dataset.selected = String(chosen);
    box.onchange = () => {
      const tags = box.checked
        ? [...state.manualTags, p.tag]
        : state.manualTags.filter(t => key(t) !== key(p.tag));
      changeGroup({tags});
    };
  }
  label.dataset.disabled = String(box.disabled);
  const name = nameCell(p.tag, banned);
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
  const visible = query ? recent.filter(p => key(p.tag).includes(query) || p.map.toLowerCase().includes(query)) : recent;
  $('recent-count').textContent = recent.length ? String(recent.length) : '';

  if (tab === 'recent') {
    // Last seen is rounded to minutes, so this changes at most once a minute.
    const signature = JSON.stringify([visible.map(p => [p.tag, p.map, lastSeen(p.lastSeen, false), recentStatus(s, p.tag)]), s.manualTags, full, query,
      s.blacklist, banMode, banConfirm, [...banPick]]);
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
  ($('recent').querySelector('input:not(:disabled)') ?? $('ban-cancel')).focus();
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

function blacklistRow(entry) {
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
  const name = el('span', 'name', entry.tag);
  name.title = entry.tag;
  row.append(avatar(entry.tag, {banned: true}), name, added, remove);
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
  const visible = query ? entries.filter(b => key(b.tag).includes(query)) : entries;
  const signature = JSON.stringify([visible.map(b => [b.tag, addedOn(b.added)]), query, entries.length]);
  if (signature !== blacklistSignature) {
    blacklistSignature = signature;
    const list = $('blacklist');
    const scroll = list.scrollTop;
    const focused = document.activeElement?.dataset?.tag;
    list.replaceChildren();
    for (const b of visible) list.append(blacklistRow(b));
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
// its blacklist selection), the blacklist scenes the Blacklist tab.
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

/* Demo bridge: used only when the page runs outside the app, to preview
   the design in a browser. Pick a scene with ?scene=watching|first|live|
   paused|password|waiting|many|linux|unverified|host-waiting|host-lobby|
   game-mode|keyboard-ask|keyboard-linux|recent|recent-empty|recent-clear|
   recent-select|recent-select-confirm|blacklist|blacklist-empty|blacklist-off;
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
  const s = {
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
  };
  Object.assign(s, scenes[scene] || {});
  if (s.enabled) watch();
  const clone = () => structuredClone(s);
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
    ClearRecentPlayers: async () => { s.recent = []; s.lobbyTags = []; return clone(); },
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
