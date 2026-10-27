# Installing Team Follow

Team Follow is one program per system. Each system needs one setup step so it is allowed to find the running game. Run Team Follow as the same user that runs Warcraft III.

To check the setup without opening the window, run the program with `discover` from a terminal while the game is on its menus:

```bash
wc3-team-follow discover
```

It prints the game's process, version and menu port, or says what is missing. The menu session itself is never printed.

## Windows 10/11 (x64)

1. Unzip `wc3-team-follow-windows-amd64.zip` anywhere and run `wc3-team-follow-windows-amd64.exe`.
2. The program is unsigned, so SmartScreen may warn the first time: choose **More info → Run anyway**.
3. The window uses Microsoft Edge WebView2, which Windows 10/11 already includes.

Friends are saved in `%LOCALAPPDATA%\WarcraftTeamFollow\settings\friends.json`. Files from the 0.4 release are picked up as they are.

## macOS 13+ (Apple silicon)

1. Unzip `wc3-team-follow-macos-arm64.zip` and move **Warcraft Team Follow.app** to Applications.
2. The app is not notarized. The first time, right-click it and choose **Open**, then **Open** again. If macOS says the app is damaged, run `xattr -dr com.apple.quarantine "/Applications/Warcraft Team Follow.app"` once.
3. To read the game's menu session, macOS needs developer tools access to be allowed. If Team Follow says *"This computer blocked reading Warcraft III's menu session"*, run this once in Terminal, enter your password, then reopen the app:

   ```bash
   sudo DevToolsSecurity -enable
   ```

   macOS may also ask for your password the first time Team Follow connects. Team Follow inspects only the Warcraft III process.

Warcraft III for Mac is an Intel app that runs under Rosetta; Team Follow works with it as is. Friends are saved in `~/Library/Application Support/WarcraftTeamFollow/settings/friends.json`.

## Linux x64 (Wine or Proton)

Team Follow runs natively on Linux and works with Warcraft III: Reforged started through Wine, Proton, Lutris, Bottles or Steam (as a non-Steam game). It finds the game by its `Warcraft III.exe` process, so it doesn't matter which launcher started it.

1. Install the window's runtime libraries (most desktops already have them):

   ```bash
   # Debian, Ubuntu, Mint, Pop!_OS
   sudo apt install libwebkit2gtk-4.1-0 libgtk-3-0
   # Fedora
   sudo dnf install webkit2gtk4.1 gtk3
   # Arch, SteamOS desktop mode
   sudo pacman -S webkit2gtk-4.1 gtk3
   ```

2. Unpack and run:

   ```bash
   tar -xzf wc3-team-follow-linux-amd64.tar.gz
   ./wc3-team-follow-linux-amd64
   ```

3. Allow it to read the game's menu session. Most distributions (Ubuntu, Fedora, Arch with Yama) only let a program read another program's memory if it started that program. Give Team Follow that one permission:

   ```bash
   sudo setcap cap_sys_ptrace=ep ./wc3-team-follow-linux-amd64
   ```

   Team Follow shows this command, with the right path, when it needs it. Run it again after replacing the file with a new version. Alternatively, `sudo sysctl kernel.yama.ptrace_scope=0` relaxes the rule for every program until the next reboot.

Notes for Wine and Proton:

- Run Team Follow outside Wine; it is a Linux program.
- Flatpak installs of Steam, Lutris or Bottles work too, but Team Follow must run outside the Flatpak sandbox as the same user.
- The game's version is read from `Warcraft III.exe` inside the Wine prefix (`WINEPREFIX`, Proton's `STEAM_COMPAT_DATA_PATH/pfx`, or `~/.wine`). If it cannot be read, Team Follow connects in Preview only.

Friends are saved in `~/.config/WarcraftTeamFollow/settings/friends.json` (or under `$XDG_CONFIG_HOME`).

## Troubleshooting

| Message | What to do |
| --- | --- |
| Warcraft III isn't running | Start the game through Battle.net and wait for the main menu. |
| More than one Warcraft III is running | Close the extra copies; Team Follow won't guess which to follow. |
| Waiting for its menus to open / finish loading | Wait on the main menu; Team Follow retries every 10 seconds. |
| This computer blocked reading Warcraft III's menu session | Do the setup step for your system above. |
| Auto-join is off for game build … | The game updated. Preview still works; joining waits for that build to be checked. |
