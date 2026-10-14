# Homebrew tap

Homebrew formulae maintained by [nemith](https://github.com/nemith).

After the first stable nvueschema release is published:

```sh
brew install nemith/tap/nvueschema
```

The formula builds the CLI from source on macOS and Linux. It requires Go only
during the build and does not install Python packages.

Stable releases from [nvueschema](https://github.com/nemith/nvueschema) update
`Formula/nvueschema.rb` automatically. Prereleases do not replace the stable formula.
