//go:build headless

package cmd

// Headless builds (server containers) exclude the Wails desktop shell
// entirely so no CGO/GTK/webkit toolchain is needed. runDesktop stays nil and
// the root command directs users to `klados serve`.
