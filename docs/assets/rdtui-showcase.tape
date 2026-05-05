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
Env RDTUI_DEMO_DOWNLOAD_DURATION "4s"

Hide
Type "export PS1='$ ' && go build -ldflags '-X github.com/m4rii0/rdtui/internal/version.Version=v0.0.0-preview' -o bin/rdtui-showcase ./cmd/rdtui && export PATH=$PWD/bin:$PATH && clear"
Enter
Wait+Line /\$/
Show

Type "rdtui-showcase"
Enter
Wait+Screen /Torrents/
Sleep 1s

# Add a magnet through the centered popup.
Type "m"
Sleep 50ms
Copy "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=KDE+Neon+User+Edition.iso"
Paste
Sleep 1500ms
Enter
Wait+Screen /Added magnet/
Sleep 1s

# Filter to a known downloaded torrent before opening details.
Type "/"
Sleep 200ms
Type "Ubuntu.24.04.2.Desktop"
Enter
Wait+Screen /Ubuntu.24.04.2.Desktop/
Sleep 500ms
Enter
Wait+Screen /Torrent Details/
Sleep 1s

# Start a managed download from the same ready torrent (single file, skips picker).
Type "d"
Wait+Screen /Managed Download/
Sleep 5s
Escape
Sleep 500ms
Escape
Sleep 500ms
Type "q"
Sleep 500ms

# Select three torrents in batch mode and delete them.
Type "b"
Sleep 500ms
Space
Sleep 250ms
Down
Space
Sleep 250ms
Down
Space
Sleep 1s
Type "x"
Wait+Screen /Delete Torrents/
Sleep 1s
Enter
Wait+Screen /Deleted 3 torrent/
Sleep 1s

Hide
Type "q"
Sleep 2s
