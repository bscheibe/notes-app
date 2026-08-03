# Repository rulesets

JSON exports of this repo's GitHub [repository rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets), kept here so the branch protection policy is versioned and reusable rather than living only in repo Settings.

**These files are not automatically enforced or synced.** GitHub does not read this directory - a ruleset only takes effect once it's created via the UI ("New ruleset" -> "Import a ruleset") or the API (`POST /repos/{owner}/{repo}/rulesets` with the file's contents as the body). If you edit repo Settings by hand, re-export and update the file here to keep it in sync; if you edit the file here, re-import it to actually apply the change.

## Files

- `main-branch-protection.json` - applies to the default branch (`~DEFAULT_BRANCH`). Blocks force-pushes and branch deletion, requires the `build-and-test` and `lint` status checks to pass and be up to date with the base branch before merging. No bypass actors - the rules apply to everyone, including the repo owner.

## Re-applying to this repo (or a new one)

```bash
gh api repos/<owner>/<repo>/rulesets -X POST --input .github/rulesets/main-branch-protection.json
```

The required status check `context` values (`build-and-test`, `lint`) must match the job names in that repo's workflows - see [.github/workflows/ci.yml](../workflows/ci.yml) and [.github/workflows/pr-title-lint.yml](../workflows/pr-title-lint.yml).

## Exporting after a manual change

```bash
gh api repos/<owner>/<repo>/rulesets/<id> --jq '{name, target, enforcement, bypass_actors, conditions, rules}' > .github/rulesets/main-branch-protection.json
```

(`<id>` is visible in the ruleset's URL under repo Settings -> Rules -> Rulesets, or via `gh api repos/<owner>/<repo>/rulesets`.)
