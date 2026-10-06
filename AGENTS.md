# Instructions for AI agents

- In Makefiles, put each `.PHONY` declaration next to its target, for example, `.PHONY: build` immediately above `build:`.
- Use `github.com/stretchr/testify/require` in all Go tests.
- Use descriptive variable names, especially in `main` and other initialization code where multiple repositories, metrics, or services may be created. For example, use `userRepository`, `userMetrics`, and `userService` instead of `r`, `m`, and `s`. Short names are acceptable in short functions where their meaning is clear and for struct receivers.
