# Gate B command evidence

Environment observed on the primary development machine:

```text
go version go1.26.7 darwin/arm64
git version 2.50.1 (Apple Git-155)
```

The spike invokes Git with these argument-array shapes (repository root is passed
after `-C`; `LC_ALL=C`, `GIT_CONFIG_NOSYSTEM=1`, and
`GIT_CONFIG_GLOBAL=/dev/null` make parsing deterministic and isolate contributor
configuration):

```text
git -C <temp-repo> rev-parse --verify HEAD^{commit}
git -C <temp-repo> cat-file -e <validated-full-object-id>^{commit}
git -C <temp-repo> merge-base --is-ancestor <validated-full-object-id> HEAD
git -C <temp-repo> diff --name-status -z -M <validated-full-object-id> HEAD --
git -C <temp-repo> diff --name-status -z -M --
git -C <temp-repo> diff --cached --name-status -z -M --
git -C <temp-repo> ls-files --others --exclude-standard -z --
git -C <temp-repo> rev-parse --path-format=absolute --git-common-dir
git -C <temp-repo> remote
git -C <temp-repo> remote get-url <remote-name>
```

`-z` avoids line parsing and preserves newlines and unusual bytes in valid Git
paths. Baselines are captured full object IDs and revalidated as 40/64 ASCII hex
before being placed in an argument position. Paths are output data only and are
never interpolated into a command.

Fixture setup commands (`init`, `config`, `add`, `commit`, `switch`, `checkout`,
`rebase`, `worktree add`, `remote add`) run only inside temporary synthetic
repositories. Observation itself uses only the read-only commands above.
