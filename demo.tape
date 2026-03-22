# nvueschema demo
Output demo.gif
Set Width 1200
Set Height 500
Set FontSize 16
Set Theme "Catppuccin Mocha"
Set Padding 20
Set TypingSpeed 40ms

# Show the tree view
Type "nvueschema show 5.16 --path interface.[*].ipv4.vrrp"
Enter
Sleep 4s

# Diff between versions
Ctrl+L
Type "nvueschema diff 5.15 5.16 --path interface"
Enter
Sleep 4s

# Flat output for grepping
Ctrl+L
Type "nvueschema diff 5.15 5.16 -O flat --path interface"
Enter
Sleep 4s

# Filter diff to only what affects your config
Ctrl+L
Type "cat testdata/config.yaml"
Enter
Sleep 3s
Type "nvueschema diff 5.15 5.16 --affected testdata/config.yaml"
Enter
Sleep 4s

# Generate YANG
Ctrl+L
Type "nvueschema gen -f yang 5.16 | head -20"
Enter
Sleep 3s
