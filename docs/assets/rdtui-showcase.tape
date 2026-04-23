Output docs/assets/rdtui-showcase.gif

Require go
Require vhs

Set Shell "bash"
Set FontFamily "Ubuntu Sans Mono"
Set FontSize 18
Set Width 1280
Set Height 720
Set Padding 18
Set Framerate 24
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set Margin 12
Set MarginFill "#11111b"
Set BorderRadius 10
Set TypingSpeed 35ms

Env RDTUI_DEMO "1"

Hide
Type "export PS1='$ ' && go build -o bin/rdtui ./cmd/rdtui && export PATH=$PWD/bin:$PATH && clear"
Enter
Wait+Line /\$/
Show

Type "rdtui"
Enter
Wait+Screen /Torrents/
Sleep 1s

# Add a magnet through the centered popup.
Type "m"
Sleep 50ms
Copy "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=KDE+Neon+User+Edition.iso"
Paste
Sleep 2s
Enter
Wait+Screen /Added magnet/
Sleep 1s

# Filter to a torrent that needs file selection.
Type "/"
Sleep 200ms
Type "Linux.Mint.22.1.Cinnamon"
Enter
Sleep 500ms

# Select files for a waiting torrent.
Type "s"
Wait+Screen /Select Files/
Sleep 700ms
Space
Sleep 350ms
Space
Sleep 350ms
Enter
Wait+Screen /Updated torrent file selection/
Sleep 1s

# Clear the active filter.
Type "q"
Sleep 500ms

# Inspect a ready torrent.
Type "/"
Sleep 200ms
Type "Ubuntu.24.04.2.Desktop"
Enter
Sleep 500ms
Enter
Wait+Screen /Torrent Details/
Sleep 1s

# Start a managed download from the same ready torrent.
Type "d"
Wait+Screen /Download/
Sleep 700ms
Enter
Wait+Screen /Managed Download/
Sleep 11s
Escape
Sleep 500ms
Type "q"
Sleep 500ms

# Show batch selection without destructive actions.
Type "b"
Sleep 500ms
Space
Down
Space
Sleep 1s
Escape
Sleep 500ms

Hide
Type "q"
Sleep 200ms
