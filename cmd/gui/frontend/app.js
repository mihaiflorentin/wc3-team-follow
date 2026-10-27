'use strict';

const $ = id => document.getElementById(id);
const bridge = window.go?.gui?.Bridge ?? demoBridge();
const MAX_TAGS = 12;

let state = null;
let lastJoinSeen;
let exePath = '';

const key = tag => tag.trim().toLowerCase();
const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

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

/* Derived state -------------------------------------------------------- */

function hasTargets(s) {
  return (s.includeTeam && s.team.some(m => !m.isSelf)) || s.manualTags.length > 0;
}

function tone(s) {
  if (!s.connected) return 'off';
  if (s.passwordRequest) return 'alert';
  if (!s.enabled) return 'off';
  if (s.paused) return 'paused';
  return s.diagnostic ? 'preview' : 'live';
}

function headline(s) {
  const n = s.watched.length;
  if (!s.connected) return ['Waiting for Warcraft III', s.connectionHint || 'Start the game through Battle.net. Team Follow finds it by itself.'];
  if (s.passwordRequest) {
    const r = s.passwordRequest;
    return ['This lobby needs a password', `${r.friend}'s lobby “${r.lobby}” asked for one.`];
  }
  if (s.enabled && s.paused) return ['Following, paused for now', s.status];
  if (s.enabled && s.joinPending && s.lastJoin) return [`Joining ${s.lastJoin.friend}…`, s.status];
  if (s.enabled && s.diagnostic) return [`Previewing ${plural(n, 'person', 'people')}`, s.status];
  if (s.enabled) return [`Following ${plural(n, 'person', 'people')}`, s.status];
  if (!hasTargets(s)) return ['Choose who to follow', 'Add a friend below, or form a team in Warcraft III.'];
  return ['Follow is off', 'Pick a mode, then start. Team Follow keeps watching while you play.'];
}

function blockedReason(s) {
  if (!s.connected) return 'Waiting for Warcraft III.';
  if (!hasTargets(s)) return 'Add a friend or form a team first.';
  if (s.joinPending) return 'A join is still in progress.';
  if (!s.diagnostic && !s.verifiedBuild) return 'Auto-join isn’t available on this game build.';
  if (s.busy || s.queued || s.inMatch) return 'Go back to Custom Games first.';
  return '';
}

/* Rendering ------------------------------------------------------------ */

function render(s) {
  if (!s) return;
  state = s;

  const connection = $('connection');
  connection.textContent = s.connected ? `Connected to Warcraft III ${s.version || ''}`.trim() : 'Not connected';
  connection.dataset.state = s.connected ? 'on' : 'off';

  const beacon = $('beacon');
  beacon.dataset.tone = tone(s);
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

  renderPassword(s);
  renderActions(s);
  renderMode(s);
  renderTeam(s);
  renderFriends(s);
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

function renderPassword(s) {
  const form = $('password-form');
  form.hidden = !s.passwordRequest;
  if (!s.passwordRequest) $('lobby-password').value = '';
}

function renderActions(s) {
  const start = $('start');
  const reason = s.enabled ? '' : blockedReason(s);
  if (s.enabled) {
    start.textContent = 'Stop';
    start.classList.add('stop');
    start.disabled = false;
  } else {
    start.textContent = s.diagnostic ? 'Start preview' : 'Start auto-join';
    start.classList.remove('stop');
    start.disabled = Boolean(reason);
  }
  $('refresh').hidden = !s.enabled || s.paused || s.joinPending || Boolean(s.passwordRequest);
  $('blocked').textContent = reason;
  if (s.enabled || reason) $('confirm').hidden = true;
}

function renderMode(s) {
  const liveBlocked = s.connected && !s.verifiedBuild;
  for (const button of document.querySelectorAll('.segmented button')) {
    const live = button.dataset.mode === 'live';
    button.setAttribute('aria-checked', String(live !== s.diagnostic));
    button.disabled = live && liveBlocked && s.diagnostic;
  }
  let help = s.diagnostic
    ? 'Shows when a friend’s lobby opens. Never leaves your team or joins anything.'
    : 'Leaves your team and joins a friend’s open lobby. Pauses in lobbies, matches and queues, then resumes in Custom Games.';
  if (liveBlocked) help = `Auto-join is off for game build ${s.version} until it’s been tested with it. Preview works.`;
  $('mode-help').textContent = help;
}

function watchedFor(s, tag) {
  return s.watched.find(w => key(w.battleTag) === key(tag));
}

function followedMark(s, tag) {
  if (!s.enabled || !watchedFor(s, tag)) return 'false';
  return s.diagnostic ? 'preview' : 'true';
}

function personRow({tag, note, presence, open, followed, removable}) {
  const li = document.createElement('li');
  li.className = 'person';
  li.dataset.followed = followed;
  const marker = document.createElement('span');
  marker.className = 'marker';
  marker.setAttribute('aria-hidden', 'true');
  const name = document.createElement('span');
  name.className = 'name';
  name.textContent = tag;
  name.title = tag;
  if (note) {
    const small = document.createElement('small');
    small.textContent = note;
    name.append(small);
  }
  const status = document.createElement('span');
  status.className = 'presence';
  status.dataset.open = String(Boolean(open));
  status.textContent = presence;
  li.append(marker, name, status);
  if (removable) {
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'remove';
    remove.textContent = '×';
    remove.setAttribute('aria-label', `Remove ${tag}`);
    remove.onclick = () => configure({tags: state.manualTags.filter(t => t !== tag)});
    li.append(remove);
  } else {
    li.append(document.createElement('span'));
  }
  return li;
}

function emptyRow(text) {
  const li = document.createElement('li');
  li.className = 'empty';
  li.textContent = text;
  return li;
}

function renderTeam(s) {
  $('include-team').checked = s.includeTeam;
  const list = $('team');
  list.replaceChildren();
  if (!s.connected) return list.append(emptyRow('Start Warcraft III to see your team.'));
  if (!s.team.some(m => !m.isSelf)) return list.append(emptyRow('You’re not in a team. Invite friends in Warcraft III, or add them below.'));
  for (const m of s.team) {
    const w = watchedFor(s, m.battleTag);
    list.append(personRow({
      tag: m.battleTag,
      note: m.isSelf ? 'you' : '',
      presence: m.isSelf ? '' : (w ? w.label : m.status),
      open: w?.open,
      followed: m.isSelf || !s.includeTeam ? 'false' : followedMark(s, m.battleTag),
    }));
  }
  list.classList.toggle('muted', !s.includeTeam);
}

function renderFriends(s) {
  const list = $('friends');
  list.replaceChildren();
  if (!s.manualTags.length) list.append(emptyRow('No friends added yet.'));
  for (const tag of s.manualTags) {
    const live = watchedFor(s, tag) ?? s.friends.find(f => key(f.battleTag) === key(tag));
    list.append(personRow({
      tag,
      presence: s.connected ? (live?.label ?? 'Waiting for activity') : '',
      open: live?.open,
      followed: followedMark(s, tag),
      removable: true,
    }));
  }
  $('friend-count').textContent = s.manualTags.length ? `${s.manualTags.length} of ${MAX_TAGS}` : '';
  const full = s.manualTags.length >= MAX_TAGS;
  $('friend-tag').disabled = full;
  $('friend-form').querySelector('button').disabled = full;
  const note = $('friend-note');
  note.textContent = s.preferencesError || 'They need to be on your Battle.net friends list. Saved on this computer.';
  note.classList.toggle('error-note', Boolean(s.preferencesError));
}

function renderLog(s) {
  const list = $('log');
  list.replaceChildren();
  const entries = s.log.slice(-12).reverse();
  if (!entries.length) {
    const li = document.createElement('li');
    li.className = 'empty';
    li.textContent = 'Nothing yet.';
    return list.append(li);
  }
  for (const entry of entries) {
    const li = document.createElement('li');
    const time = document.createElement('time');
    const at = new Date(entry.time);
    time.dateTime = entry.time;
    time.textContent = at.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    const text = document.createElement('span');
    text.textContent = entry.message;
    li.append(time, text);
    list.append(li);
  }
}

/* Events --------------------------------------------------------------- */

function start(fromMenus) {
  $('confirm').hidden = true;
  return configure({enabled: true, diagnostic: state.diagnostic, fromMenus});
}

$('start').onclick = () => {
  if (!state) return;
  if (state.enabled) return configure({enabled: false});
  if (state.inMenus) return start(true);
  $('confirm').hidden = false;
  $('confirm-yes').focus();
};
$('confirm-yes').onclick = () => start(true);
$('confirm-no').onclick = () => { $('confirm').hidden = true; };
$('refresh').onclick = () => configure({enabled: true, diagnostic: state.diagnostic, fromMenus: true});

for (const button of document.querySelectorAll('.segmented button')) {
  button.onclick = () => {
    const diagnostic = button.dataset.mode === 'preview';
    if (state && diagnostic !== state.diagnostic) configure({diagnostic});
  };
}

$('include-team').onchange = event => configure({includeTeam: event.target.checked});

$('friend-form').onsubmit = async event => {
  event.preventDefault();
  const tag = $('friend-tag').value.trim();
  if (!tag || !state) return;
  if (await configure({tags: [...state.manualTags, tag]})) $('friend-tag').value = '';
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

$('report').onclick = async () => {
  try {
    $('report-text').textContent = await bridge.Diagnostics();
    $('report-dialog').showModal();
  } catch (err) {
    $('error').textContent = String(err?.message ?? err);
  }
};
$('report-copy').onclick = async () => {
  await copy($('report-text').textContent);
  $('report-copy').textContent = 'Copied';
  setTimeout(() => { $('report-copy').textContent = 'Copy report'; }, 1500);
};
$('report-close').onclick = () => $('report-dialog').close();

async function boot() {
  try { exePath = await bridge.ExecutablePath(); } catch { exePath = ''; }
  window.runtime?.EventsOn?.('state', render);
  try { render(await bridge.State()); } catch (err) { $('status').textContent = String(err); }
  // The Go side pushes every change; this poll only covers a missed event.
  setInterval(async () => { try { render(await bridge.State()); } catch { /* window closing */ } }, 5000);
}
boot();

/* Demo bridge: used only when the page runs outside the app, to preview
   the design in a browser. Pick a scene with ?scene=off|live|paused|
   password|waiting|linux|unverified. */
function demoBridge() {
  const scene = new URLSearchParams(location.search).get('scene') || 'live';
  const now = Date.now();
  const at = mins => new Date(now - mins * 60000).toISOString();
  const s = {
    connected: true, version: '3.0.1.24371', verifiedBuild: true, enabled: true, diagnostic: false,
    manualTags: ['Thrall#1820', 'Jaina#2214'], includeTeam: true,
    status: 'Auto-join is on. It pauses in lobbies, matches and queues.',
    screen: 'CUSTOM_LOBBIES', busy: false, queued: false, inMatch: false, paused: false, inMenus: true,
    joinPending: false, passwordRequest: null, preferencesError: '', connectionHint: '',
    team: [
      {battleTag: 'Mihai#2811', isSelf: true, status: 'You'},
      {battleTag: 'Arthas#1123', isFriend: true, status: 'Lobby open'},
      {battleTag: 'Sylvanas#9031', isFriend: true, status: 'In Warcraft III'},
    ],
    watched: [
      {battleTag: 'Arthas#1123', label: 'Lobby open', open: true, lobby: 'Footmen Frenzy 6.2'},
      {battleTag: 'Sylvanas#9031', label: 'In Warcraft III', open: false},
      {battleTag: 'Thrall#1820', label: 'Offline', open: false},
      {battleTag: 'Jaina#2214', label: 'Online in Battle.net', open: false},
    ],
    friends: [
      {battleTag: 'Thrall#1820', label: 'Offline', open: false},
      {battleTag: 'Jaina#2214', label: 'Online in Battle.net', open: false},
    ],
    lastJoin: null,
    log: [
      {time: at(9), message: 'Connected to Warcraft III. Form a team in the game, or add friends here.'},
      {time: at(6), message: 'Preview is on. You’ll see when a friend’s lobby opens; nothing will be joined.'},
      {time: at(4), message: 'Arthas#1123 has an open lobby. Preview mode didn’t join it.'},
      {time: at(1), message: 'Auto-join is on. It pauses in lobbies, matches and queues.'},
    ],
  };
  const scenes = {
    off: {enabled: false, diagnostic: true, status: 'Follow is off.'},
    paused: {paused: true, busy: true, inMatch: true, inMenus: false, status: 'Follow is on but paused during the match.'},
    password: {passwordRequest: {lobby: 'Pros only', friend: 'Arthas#1123'}, status: 'The game is asking for a lobby password.'},
    waiting: {connected: false, enabled: false, team: [], watched: [], friends: [], connectionHint: 'Warcraft III isn’t running. Start it through Battle.net.'},
    linux: {connected: false, enabled: false, team: [], watched: [], friends: [], connectionHint: 'This computer blocked reading Warcraft III’s menu session. To allow it, run: sudo setcap cap_sys_ptrace=ep <path to wc3-team-follow>'},
    unverified: {enabled: false, diagnostic: true, version: '3.0.2.25010', verifiedBuild: false, status: 'Connected to Warcraft III.'},
  };
  Object.assign(s, scenes[scene] || {});
  const clone = () => structuredClone(s);
  return {
    State: async () => clone(),
    ExecutablePath: async () => '/home/me/Apps/wc3-team-follow',
    Diagnostics: async () => JSON.stringify({version: s.version, connected: s.connected, platform: 'demo'}, null, 2),
    SubmitPassword: async () => { s.passwordRequest = null; return clone(); },
    Configure: async cfg => {
      if (cfg.tags) s.manualTags = cfg.tags;
      if ('includeTeam' in cfg) { s.includeTeam = cfg.includeTeam; s.enabled = false; }
      if ('diagnostic' in cfg && cfg.diagnostic !== s.diagnostic) { s.diagnostic = cfg.diagnostic; s.enabled = false; }
      if (cfg.enabled === false) s.enabled = false;
      if (cfg.enabled === true) s.enabled = true;
      return clone();
    },
  };
}
