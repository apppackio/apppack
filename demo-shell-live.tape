Output demo-shell-live.gif

# Requires: an AppPack app with 2+ running tasks
# Note: VHS resets HOME; re-export it so apppack can find auth tokens
# Usage: vhs demo-shell-live.tape

Set Shell "bash"
Set Width 900
Set Height 400
Set FontSize 16
Set Padding 20
Set TypingSpeed 10ms
Set LoopOffset 50%

Hide
Type `export HOME=${REAL_HOME:-$HOME}`
Enter
Sleep 500ms
Type "clear"
Enter
Sleep 500ms
Show

Type "./apppack-dev -a huh-test shell --live"
Enter
Sleep 6s
Down
Sleep 600ms
Enter
Sleep 3s
Ctrl+C
Sleep 1s
