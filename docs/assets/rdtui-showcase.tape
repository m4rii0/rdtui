Output bin/showcase/rdtui-showcase.mp4

# Render with `make showcase-video` to burn # Caption: annotations into a
# separate band below the terminal. Plain VHS still records the same tour.
# Reading pauses keep the tour near 75-80 seconds without slowing animations.

Require go
Require vhs

Set Shell "bash"
Set FontFamily "Ubuntu Sans Mono"
Set FontSize 27
Set Width 1920
Set Height 960
Set Padding 18
Set Framerate 24
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set Margin 12
Set MarginFill "#11111b"
Set BorderRadius 10
Set TypingSpeed 35ms

Env RDTUI_DEMO "1"
Env RDTUI_DEMO_DOWNLOAD_DURATION "4s"

Hide
# The browser resolves its starting directory and edit field to absolute paths.
# Build here, then launch from a neutral fixture directory before recording.
Type@1ms "export PS1='$ ' && go build -ldflags '-X github.com/m4rii0/rdtui/internal/version.Version=v0.0.0-preview' -o bin/rdtui-showcase ./cmd/rdtui && export PATH=$PWD/bin:$PATH && mkdir -p /tmp/opencode/rdtui-showcase-demo/torrents && printf '%s' 'd4:infod6:lengthi0e4:name15:Debian.Demo.iso12:piece lengthi16384e6:pieces0:ee' > /tmp/opencode/rdtui-showcase-demo/torrents/Debian.Demo.torrent && printf '%s' 'd4:infod6:lengthi0e4:name13:Arch.Demo.iso12:piece lengthi16384e6:pieces0:ee' > /tmp/opencode/rdtui-showcase-demo/torrents/Arch.Demo.torrent && cd /tmp/opencode/rdtui-showcase-demo && clear"
Enter
Wait+Line@60s /\$/
Show

# Caption: Manage your Real-Debrid torrents without leaving the terminal.
Type "rdtui-showcase"
Enter
Wait+Screen /Torrents/
Sleep 2500ms

# Caption: [?] Discover the shortcuts available on each screen.
Type "?"
Wait+Screen /Shortcuts/
Sleep 4s
Escape
Sleep 250ms

# Caption: [o] Sort torrents by status, progress, size, date or name.
Type "o"
Wait+Screen /Sort torrents/
Up 5
Down 2
Sleep 2s
Enter
Wait+Screen /Sorted by Size/
Sleep 1s

# Caption: [m] Add a torrent by pasting its magnet link.
Type "m"
Wait+Screen /Paste a magnet link/
Copy "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=KDE.Neon.Install.Media"
Paste
Sleep 2s
Enter
Wait+Screen /Added magnet/
Sleep 500ms
Type "/"
Type "KDE.Neon.Install.Media"
Enter
Sleep 500ms

# Caption: [s, space] Choose which source files Real-Debrid should fetch.
Type "s"
Wait+Screen /Select Files/
Sleep 1s
Down
Space
Sleep 2s
Enter
Wait+Screen /Updated torrent file selection/
Sleep 750ms
Escape
Sleep 250ms

# Caption: [i] Browse local files and import several .torrent files at once.
Type "i"
Wait+Screen /Import: .*rdtui-showcase-demo/
Sleep 500ms
Type "/"
Ctrl+A
Ctrl+K
Type "torrents/"
Enter
Wait+Screen /Debian.Demo.torrent/
Ctrl+A
Sleep 2s
Ctrl+S
Wait+Screen /Imported 2 torrent/
Sleep 1s

# Caption: [/] Find torrents quickly with fuzzy search and a persistent filter.
Type "/"
Type "Ubuntu.24.04.2.Desktop"
Enter
Wait+Screen /Ubuntu.24.04.2.Desktop/
Sleep 3s

# Caption: [enter] Inspect a torrent's status, size and selected files.
Enter
Wait+Screen /Torrent Details/
Sleep 4s

# Caption: [y] Resolve and copy a direct URL without starting a download.
Type "y"
Wait+Screen /Direct URL/
Sleep 4s
Escape
Sleep 250ms

# Caption: [d] Download locally with live progress, speed and ETA.
Type "d"
Wait+Screen /Managed Download/
Wait+Screen@10s /100\.0%/
Sleep 1500ms
Escape
Wait+Screen /Torrent Details/
Escape
Sleep 250ms
Wait+Screen /Torrents/
Escape
Sleep 250ms

# Caption: [b, space] Mark multiple completed torrents for a bulk download.
Type "/"
Type "Debian.12.9.0"
Enter
Type "N"
Up 10
Type "b"
Wait+Screen /BATCH · 0 selected/
Space
Wait+Screen /BATCH · 1 selected/
Down 10
Space
Wait+Screen /BATCH · 2 selected/
Sleep 2500ms

# Caption: [d, J/K] Build a download queue and choose the torrent order.
Type "d"
Wait+Screen /Bulk Download Order/
Sleep 2s
Type "J"
Sleep 2s

# Caption: [space, enter] Pick the files to include from a multi-file torrent.
Enter
Wait+Screen /Select Download Files/
Ctrl+D
Space
Down
Space
Sleep 4s
Enter
Wait+Screen /Start Bulk Download/

# Caption: [enter] Review the selected torrents and files before starting.
Sleep 3s
Enter
Wait+Screen /Bulk Download/
Sleep 1500ms

# Caption: [esc] Keep browsing while the queue downloads in the background.
Escape
Wait+Screen /Torrents/
Escape
Sleep 250ms
Escape
Sleep 250ms
Sleep 1500ms
Down 4
Sleep 1500ms

# Caption: [a] Return to the active queue without starting another transfer.
Type "a"
Wait+Screen /Bulk Download/
Wait+Screen@20s /Bulk Download Summary/

# Caption: Review completed, failed and skipped files in the final summary.
Sleep 4s

# Caption: [x] Clean up source torrents after their downloads finish.
Type "x"
Wait+Screen /Delete Source Torrents/
Sleep 3s
Enter
Wait+Screen /Deleted 2 source torrent/
Sleep 2s

Hide
Type "q"
Sleep 2s
