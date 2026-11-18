# Releasing

Each SDK is released by itself, by pushing a tag. The tag names the SDK and the version:

| SDK        | Tag                          | Workflow                                               |
| ---------- | ---------------------------- | ------------------------------------------------------ |
| Go         | `go/webhooks/vX.Y.Z`         | [`release-go.yml`](.github/workflows/release-go.yml)   |
| TypeScript | `typescript/webhooks/vX.Y.Z` | [`release-npm.yml`](.github/workflows/release-npm.yml) |

Both workflows check the tagged commit as CI does, then wait for an approval in the `release` environment, then release. Nothing is published until someone approves the run, so a tag pushed by mistake can still be stopped there. After that it cannot: see [Undoing a release](#undoing-a-release).

A release is refused when the tagged commit is not on `main`.

## Releasing the TypeScript package

1. Open a pull request that sets `version` in `typescript/webhooks/package.json`, and merge it.
2. Tag the merge commit and push the tag:

   ```sh
   git switch main && git pull
   git tag typescript/webhooks/v0.1.0
   git push origin typescript/webhooks/v0.1.0
   ```

3. The run checks and packs the package, and lists the files in it. Read the list, then approve the **Publish to npm** job.
4. Check the version on [npmjs.com](https://www.npmjs.com/package/@cimento/webhooks). It should show a provenance statement naming this repository and the run.

The run fails before the approval if the tag does not name the version in `package.json`.

The workflow chooses the npm distribution tag from the version, and says which in the run's summary:

| Version                                            | Tag           |
| -------------------------------------------------- | ------------- |
| Newer than everything published                    | `latest`      |
| A pre-release, such as `0.2.0-rc.1`                | `next`        |
| Older than `latest`, such as `0.1.1` after `0.2.0` | `release-0.1` |

So `npm install @cimento/webhooks` only ever gives the newest full release.

The workflow has no npm token. npm trusts it by [trusted publishing](https://docs.npmjs.com/trusted-publishers): the run proves to npm which repository, workflow and environment it is.

## Releasing the Go module

1. Tag the commit on `main` to release, and push the tag:

   ```sh
   git switch main && git pull
   git tag go/webhooks/v0.1.0
   git push origin go/webhooks/v0.1.0
   ```

2. Approve the **Release** job.
3. Check the version on [pkg.go.dev](https://pkg.go.dev/github.com/cimento-ai/cimento-sdk/go/webhooks).

A Go module is not uploaded anywhere. The tag is the release, and anyone can fetch it from the moment it is pushed. The workflow is there to refuse a tag that should not be one, and to confirm the Go module proxy can serve the version.

The module's path has no major version in it, so only `v0` and `v1` tags are accepted. A `v2` needs `/v2` added to the module's path first.

## Dry runs

A dry run does everything except release: it runs the checks, and for npm it packs the package and runs `npm publish --dry-run`. It needs no approval.

- Start either workflow by hand from the Actions tab. That is always a dry run.
- While the repository is private, a tag is a dry run too. Nothing is ever released from a private repository: npm does not record provenance for one, and the Go proxy cannot read one.

Delete a tag that was pushed only for a dry run:

```sh
git push origin :refs/tags/typescript/webhooks/v0.1.0
```

## Undoing a release

Neither kind of release can be taken back cleanly, so the approval is the last point to stop.

- **Go.** Once the proxy has a version, it keeps it. Deleting or moving the tag does not help, and a moved tag breaks everyone whose checksum no longer matches. Never reuse a version. Release a fixed version, and add a [`retract`](https://go.dev/ref/mod#go-mod-file-retract) line for the bad one to `go.mod` in that release.
- **npm.** `npm deprecate @cimento/webhooks@X.Y.Z "reason"` warns everyone who installs the version; prefer it. `npm unpublish` works only within 72 hours and only under [npm's conditions](https://docs.npmjs.com/policies/unpublish), and the version number can never be used again.

## Setting up npm, once

npm only lets a trusted publisher be added to a package that exists. So the first version of a new package is published by hand, as a placeholder, and every real version comes from the workflow.

1. Make the repository public. npm does not record provenance for a private one.
2. Publish a placeholder to claim the name. In an empty directory:

   ```sh
   npm init --scope=@cimento -y
   npm pkg set name=@cimento/webhooks version=0.0.0 description="Placeholder. Use 0.1.0 or later."
   npm publish --access public
   npm deprecate @cimento/webhooks@0.0.0 "Placeholder. Use 0.1.0 or later."
   ```

3. On npmjs.com, in the package's settings, add a trusted publisher for GitHub Actions. Every field is matched exactly, and npm does not check them until a publish:

   | Field                | Value             |
   | -------------------- | ----------------- |
   | Organization or user | `Cimento-AI`      |
   | Repository           | `cimento-sdk`     |
   | Workflow filename    | `release-npm.yml` |
   | Environment name     | `release`         |

4. Release the first real version within 2 days. A trusted publisher that has not published by then expires.
5. After that release, set the package's publishing access to require two-factor authentication and disallow tokens, so the workflow is the only way to publish.

A new package in this repository needs the same steps, and its own workflow file.
