# Records vigil's scripted demo to assets/demo.mp4 (portfolio site).
# Run from the repo root: vhs demo.tape
# Needs VHS (which pulls in ttyd) and ffmpeg. Builds vigil with -tags demo; see cmd/vigil/demo.go.
#
# On a machine low on memory, limit the run's memory and keep its frames out of a RAM-backed /tmp.
# Chromium's socket lives under TMPDIR, so keep that path short, and create it first:
#   systemd-run --user --scope -p MemoryMax=2G -p MemorySwapMax=0 env TMPDIR=$HOME/.cache/vhs-tmp vhs demo.tape
#
# The README's GIF comes from the MP4 in two passes. Don't add an Output for it here:
# VHS's GIF filter holds every raw frame in RAM while it builds the palette, which is
# gigabytes for this recording.
#   ffmpeg -y -i assets/demo.mp4 -vf "fps=6,palettegen=stats_mode=diff" /tmp/demo-palette.png
#   ffmpeg -y -i assets/demo.mp4 -i /tmp/demo-palette.png -lavfi "fps=6[v];[v][1:v]paletteuse=dither=none:diff_mode=rectangle" assets/demo.gif

Output assets/demo.mp4

Require go

Set Shell bash
Set FontSize 14
# Keeps vigil at 160+ columns, the layout that fits every panel without truncating rows
Set Width 1540
Set Height 690
Set Padding 16
Set Framerate 10
Set WaitTimeout 120s

Hide
Type "go build -tags demo -o /tmp/vigil-demo ./cmd/vigil && clear && /tmp/vigil-demo --demo"
Enter
Wait+Screen /PWR: OK/
Show

# 0s: no alerts
Sleep 7s
Type "?"
Sleep 4s
Escape
# 11s: back to the dashboard; CPU alert at 13s, NAS missing at 19s, USB unstable at 26s, mqtt down at 28s, power at 30s
Sleep 22s
Type "m"
# 33s: muted. Alerts are cpu, nas, usb unstable, mqtt, throttle; step to usb unstable (index 2) and dismiss it
Sleep 3s
Type "j"
Sleep 0.7s
Type "j"
Sleep 0.7s
Type "j"
Sleep 0.7s
Type "k"
Sleep 0.9s
Type "d"
# 39s: recovery at 44-48s
Sleep 11s
Type "m"
# 50s: unmuted. Dismiss the four resolved alerts from the top
Sleep 2s
Type "k"
Sleep 0.4s
Type "k"
Sleep 0.6s
Type "d"
Sleep 0.6s
Type "d"
Sleep 0.6s
Type "d"
Sleep 0.6s
Type "d"
# ~56s: all clear
Sleep 4s

# Hidden from the recording: quit so the demo deletes its temporary database
Hide
Type "q"
Sleep 1s
